package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"magnet-webdav/config"
	"magnet-webdav/models"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"gorm.io/gorm"
)

type TorrentService struct {
	cfg            *config.Config
	db             *gorm.DB
	client         *torrent.Client
	activeTorrents map[string]*torrent.Torrent
	mutex          sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewTorrentService(cfg *config.Config, db *gorm.DB) *TorrentService {
	ctx, cancel := context.WithCancel(context.Background())
	return &TorrentService{
		cfg:            cfg,
		db:             db,
		activeTorrents: make(map[string]*torrent.Torrent),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *TorrentService) Start() error {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = s.cfg.Torrent.DownloadDir
	clientConfig.DefaultStorage = storage.NewFile(s.cfg.Torrent.DownloadDir)
	clientConfig.ListenPort = s.cfg.Torrent.ListenPort
	clientConfig.DisableIPv6 = true
	clientConfig.HTTPUserAgent = s.cfg.Torrent.UserAgent

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create torrent client: %w", err)
	}

	s.client = client

	// 恢复之前活跃的种子
	if err := s.restoreActiveTorrents(); err != nil {
		log.Printf("Failed to restore active torrents: %v", err)
	}

	log.Println("Torrent service started")
	return nil
}




func (s *TorrentService) Stop() {
	s.cancel()

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for infoHash, torr := range s.activeTorrents {
		torr.Drop()
		delete(s.activeTorrents, infoHash)
	}

	if s.client != nil {
		s.client.Close()
	}

	log.Println("Torrent service stopped")
}

func (s *TorrentService) AddMagnet(magnetURI string) (*models.Magnet, error) {
	infoHash := s.extractInfoHash(magnetURI)

	// 检查是否已存在
	var existingMagnet models.Magnet
	if err := s.db.Where("id = ?", infoHash).First(&existingMagnet).Error; err == nil {
		return &existingMagnet, nil
	}

	// 创建新的磁力记录
	magnet := &models.Magnet{
		ID:        infoHash,
		MagnetURI: magnetURI,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.db.Create(magnet).Error; err != nil {
		return nil, fmt.Errorf("failed to create magnet record: %w", err)
	}

	// 添加到 torrent 客户端
	go s.addTorrentToClient(magnetURI, infoHash)

	return magnet, nil
}

func (s *TorrentService) addTorrentToClient(magnetURI, infoHash string) {
	torr, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		log.Printf("Failed to add magnet: %v", err)
		s.updateMagnetStatus(infoHash, "error", err.Error())
		return
	}

	s.mutex.Lock()
	s.activeTorrents[infoHash] = torr
	s.mutex.Unlock()

	// 等待元数据
	select {
	case <-torr.GotInfo():
		s.handleTorrentReady(torr, infoHash)
	case <-time.After(30 * time.Second):
		log.Printf("Timeout waiting for metadata: %s", infoHash)
		s.updateMagnetStatus(infoHash, "error", "metadata timeout")
	case <-s.ctx.Done():
		return
	}
}

func (s *TorrentService) handleTorrentReady(torr *torrent.Torrent, infoHash string) {
	// 更新磁力记录
	updates := map[string]interface{}{
		"name":       torr.Name(),
		"total_size": torr.Length(),
		"file_count": len(torr.Files()),
		"status":     "ready",
		"updated_at": time.Now(),
	}

	if err := s.db.Model(&models.Magnet{}).Where("id = ?", infoHash).Updates(updates).Error; err != nil {
		log.Printf("Failed to update magnet: %v", err)
		return
	}

	// 获取现有的文件记录
	var existingFiles []models.File
	if err := s.db.Where("magnet_id = ?", infoHash).Find(&existingFiles).Error; err != nil {
		log.Printf("Failed to get existing files: %v", err)
		return
	}

	// 创建现有文件的映射，用于快速查找
	existingFileMap := make(map[string]models.File)
	for _, file := range existingFiles {
		existingFileMap[file.FilePath] = file
	}

	// 处理新文件
	var filesToCreate []models.File
	var filesToUpdate []models.File
	seenFiles := make(map[string]bool)

	for i, file := range torr.Files() {
		filePath := file.Path()

		// 检查是否已经处理过这个文件
		if seenFiles[filePath] {
			log.Printf("Skipping duplicate file: %s", filePath)
			continue
		}
		seenFiles[filePath] = true

		mimeType := s.getMimeType(filePath)
		newFile := models.File{
			MagnetID:  infoHash,
			FilePath:  filePath,
			FileName:  path.Base(filePath),
			FileSize:  file.Length(),
			FileIndex: i,
			MimeType:  mimeType,
			CreatedAt: time.Now(),
		}

		// 检查文件是否已存在
		if existingFile, exists := existingFileMap[filePath]; exists {
			// 文件已存在，检查是否需要更新
			needsUpdate := false
			updatedFile := existingFile

			if existingFile.FileSize != file.Length() {
				updatedFile.FileSize = file.Length()
				needsUpdate = true
				log.Printf("File size changed: %s (%d -> %d)", filePath, existingFile.FileSize, file.Length())
			}

			if existingFile.FileIndex != i {
				updatedFile.FileIndex = i
				needsUpdate = true
				log.Printf("File index changed: %s (%d -> %d)", filePath, existingFile.FileIndex, i)
			}

			if existingFile.MimeType != mimeType {
				updatedFile.MimeType = mimeType
				needsUpdate = true
				log.Printf("Mime type changed: %s (%s -> %s)", filePath, existingFile.MimeType, mimeType)
			}

			if needsUpdate {
				updatedFile.UpdatedAt = time.Now()
				filesToUpdate = append(filesToUpdate, updatedFile)
			}

			// 从 existingFileMap 中移除，剩下的就是需要删除的文件
			delete(existingFileMap, filePath)
		} else {
			// 新文件，需要创建
			filesToCreate = append(filesToCreate, newFile)
		}
	}

	// 执行数据库操作
	if len(filesToCreate) > 0 {
		if err := s.db.Create(&filesToCreate).Error; err != nil {
			log.Printf("Failed to create new file records: %v", err)
		} else {
			log.Printf("Created %d new files", len(filesToCreate))
		}
	}

	if len(filesToUpdate) > 0 {
		for _, file := range filesToUpdate {
			if err := s.db.Save(&file).Error; err != nil {
				log.Printf("Failed to update file %s: %v", file.FilePath, err)
			}
		}
		log.Printf("Updated %d files", len(filesToUpdate))
	}

	// 删除不再存在的文件
	if len(existingFileMap) > 0 {
		var filesToDelete []int
		for _, file := range existingFileMap {
			filesToDelete = append(filesToDelete, file.ID)
		}

		if err := s.db.Where("id IN ?", filesToDelete).Delete(&models.File{}).Error; err != nil {
			log.Printf("Failed to delete old files: %v", err)
		} else {
			log.Printf("Deleted %d old files", len(filesToDelete))
			for filePath := range existingFileMap {
				log.Printf("  - %s", filePath)
			}
		}
	}

	log.Printf("Torrent synchronization completed: %s", torr.Name())
	log.Printf("  Total files: %d", len(torr.Files()))
	log.Printf("  Created: %d, Updated: %d, Deleted: %d",
		len(filesToCreate), len(filesToUpdate), len(existingFileMap))
}

func (s *TorrentService) GetTorrent(infoHash string) *torrent.Torrent {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.activeTorrents[infoHash]
}



// 修改 GetFileStream 方法中的条件判断
func (s *TorrentService) GetFileStream(infoHash, filePath string, start, end int64) (*torrent.File, torrent.Reader, error) {
	torr := s.GetTorrent(infoHash)
	if torr == nil {
		return nil, nil, fmt.Errorf("torrent not found: %s", infoHash)
	}

	// 检查 torrent 是否已准备好 - 修复语法错误
	if torr.Info() == nil {
		return nil, nil, fmt.Errorf("torrent not ready: %s", infoHash)
	}

	// 检查文件列表是否为空
	files := torr.Files()
	if files == nil || len(files) == 0 {
		return nil, nil, fmt.Errorf("no files in torrent: %s", infoHash)
	}

	var targetFile *torrent.File
	for _, file := range files {
		if file.Path() == filePath {
			targetFile = file
			break
		}
	}

	if targetFile == nil {
		return nil, nil, fmt.Errorf("file not found: %s in torrent %s", filePath, infoHash)
	}

	// 更新访问统计
	go s.updateAccessStats(infoHash)

	reader := targetFile.NewReader()
	if start > 0 {
		reader.Seek(start, 0)
	}

	return targetFile, reader, nil
}





func (s *TorrentService) extractInfoHash(magnetURI string) string {
	re := regexp.MustCompile(`btih:([^&]+)`)
	matches := re.FindStringSubmatch(magnetURI)
	if len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	hash := sha1.Sum([]byte(magnetURI))
	return hex.EncodeToString(hash[:])[:20]
}

func (s *TorrentService) getMimeType(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	mimeTypes := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".webm": "video/webm",
		".jpg":  "image/jpeg",
		".png":  "image/png",
		".srt":  "text/plain",
		".ass":  "text/plain",
	}

	if mime, exists := mimeTypes[ext]; exists {
		return mime
	}
	return "application/octet-stream"
}

func (s *TorrentService) updateMagnetStatus(infoHash, status, errorMsg string) {
	updateData := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if errorMsg != "" {
		updateData["name"] = errorMsg
	}

	s.db.Model(&models.Magnet{}).Where("id = ?", infoHash).Updates(updateData)
}

func (s *TorrentService) updateAccessStats(infoHash string) {
	s.db.Model(&models.Magnet{}).Where("id = ?", infoHash).
		Updates(map[string]interface{}{
			"access_count":  gorm.Expr("access_count + 1"),
			"last_accessed": time.Now(),
		})
}

func (s *TorrentService) restoreActiveTorrents() error {
	var magnets []models.Magnet
	if err := s.db.Where("status = ?", "ready").Find(&magnets).Error; err != nil {
		return err
	}

	for _, magnet := range magnets {
		go s.addTorrentToClient(magnet.MagnetURI, magnet.ID)
	}

	log.Printf("Restored %d active torrents", len(magnets))
	return nil
}

func (s *TorrentService) GetActiveTorrentCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.activeTorrents)
}

func (s *TorrentService) DB() *gorm.DB {
	return s.db
}
