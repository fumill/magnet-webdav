package handlers

import (
	"fmt"
	"io"
	"log"
	"magnet-webdav/config"
	"magnet-webdav/models"
	"magnet-webdav/services"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type WebDAVHandler struct {
	torrentService *services.TorrentService
	config         *config.Config
}

func NewWebDAVHandler(torrentService *services.TorrentService, config *config.Config) *WebDAVHandler {
	return &WebDAVHandler{
		torrentService: torrentService,
		config:         config,
	}
}

func (h *WebDAVHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET", "HEAD":
		h.handleGet(w, r)
	case "PROPFIND":
		h.handlePropfind(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *WebDAVHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/webdav/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		h.serveDirectoryListing(parts[0], w, r)
		return
	}

	magnetID := parts[0]
	encodedFilePath := strings.Join(parts[1:], "/")

	// Properly unescape filename
	filePath, err := url.PathUnescape(encodedFilePath)
	if err != nil {
		http.Error(w, "Invalid file path encoding", http.StatusBadRequest)
		return
	}

	// Parse Range
	var start, end int64
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		parsed, err := parseRangeHeader(rangeHeader)
		if err == nil {
			start, end = parsed.start, parsed.end
		}
	}

	// Try get torrent reader
	file, reader, err := h.torrentService.GetFileStream(magnetID, filePath, start, end)
	if err != nil {
		log.Printf("Error getting file stream: %v", err)
		http.Error(w, "File not found or not ready", http.StatusNotFound)
		return
	}

	// IMPORTANT: if special caching conditions match, return 304 without reading
	if h.handleConditionalRequest(w, r, filePath, start, end) {
		reader.Close()
		return
	}

	defer reader.Close()

	mimeType := getMimeType(filePath)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")

	fileSize := file.Length()

	// Fix end boundaries
	if end == 0 || end >= fileSize {
		end = fileSize - 1
	}

	// Partial Content
	if start > 0 || r.Header.Get("Range") != "" {
		contentLength := end - start + 1
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
		w.Header().Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	// Optimize torrent streaming
	reader.SetReadahead(2 * 1024 * 1024) // 2MB max prefetch

	if r.Method == "GET" {
		_, copyErr := io.CopyN(w, reader, end-start+1)
		if copyErr != nil && copyErr != io.EOF {
			log.Printf("Copy error: %v", copyErr)
		}
	}
}

func (h *WebDAVHandler) handleConditionalRequest(w http.ResponseWriter, r *http.Request, filePath string, start, end int64) bool {
	etag := generateETag(filePath, start, end)
	w.Header().Set("ETag", etag)
	h.setCacheHeaders(w, r, filePath, start, end)

	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch == etag {
		w.WriteHeader(http.StatusNotModified)
		return true // <== STOP HERE
	}
	return false
}

// setCacheHeaders 设置缓存头
func (h *WebDAVHandler) setCacheHeaders(w http.ResponseWriter, r *http.Request, filePath string, start, end int64) {
	// 设置缓存控制头
	cacheControl := "public, max-age=3600" // 1小时缓存

	// 视频文件可以缓存更长时间
	if isVideoFile(filePath) {
		if start == 0 && end == 0 {
			// 完整视频文件缓存更长时间
			cacheControl = "public, max-age=86400" // 24小时
		} else {
			// 视频范围请求缓存较短时间
			cacheControl = "public, max-age=1800" // 30分钟
		}
	}

	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type")

	// 设置过期头
	expires := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	w.Header().Set("Expires", expires)

	// 设置 ETag 用于缓存验证
	etag := generateETag(filePath, start, end)
	w.Header().Set("ETag", etag)

	// 处理 If-None-Match 请求（缓存验证）
	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" {
		if ifNoneMatch == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
}

// isVideoFile 检查是否为视频文件
func isVideoFile(filePath string) bool {
	videoExtensions := map[string]bool{
		".mp4":  true,
		".mkv":  true,
		".avi":  true,
		".mov":  true,
		".webm": true,
		".flv":  true,
		".wmv":  true,
		".m4v":  true,
		".3gp":  true,
	}

	ext := strings.ToLower(filePath[strings.LastIndex(filePath, "."):])
	return videoExtensions[ext]
}

// generateETag 生成 ETag 用于缓存验证
func generateETag(filePath string, start, end int64) string {
	key := fmt.Sprintf("%s-%d-%d", filePath, start, end)
	// 简化版的 ETag 生成
	return fmt.Sprintf(`"%x"`, len(key))
}

func (h *WebDAVHandler) handlePropfind(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/webdav/")
	xml := h.generatePropfindResponse(path)

	// 设置正确的 XML 编码
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("DAV", "1, 2")
	w.Write([]byte(xml))
}

func (h *WebDAVHandler) serveDirectoryListing(magnetID string, w http.ResponseWriter, r *http.Request) {
	var files []struct {
		FileName string
		FileSize int64
		FilePath string
	}

	// 检查磁力链接状态
	var magnet models.Magnet
	db := h.torrentService.DB()
	if err := db.Where("id = ?", magnetID).First(&magnet).Error; err != nil {
		http.Error(w, "Magnet not found", http.StatusNotFound)
		return
	}

	// 设置正确的 HTML 编码
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>` + magnetID + `</title>
    <style>
        body { font-family: Arial, "Microsoft YaHei", sans-serif; margin: 20px; }
        ul { list-style: none; padding: 0; }
        li { padding: 8px; border-bottom: 1px solid #eee; }
        a { text-decoration: none; color: #0366d6; }
        .size { color: #666; font-size: 0.9em; }
        .status { padding: 4px 8px; border-radius: 4px; font-size: 0.8em; }
        .status-ready { background: #d4edda; color: #155724; }
        .status-pending { background: #fff3cd; color: #856404; }
        .status-error { background: #f8d7da; color: #721c24; }
        .warning { background: #fff3cd; border: 1px solid #ffeaa7; padding: 10px; border-radius: 4px; margin: 10px 0; }
    </style>
</head>
<body>
    <h1>文件列表 - ` + magnet.Name + `</h1>
    <div class="status status-` + magnet.Status + `">状态: ` + getStatusText(magnet.Status) + `</div>`

	if magnet.Status != "ready" {
		html += `<div class="warning">
            <strong>注意:</strong> 磁力链接正在准备中，文件暂时不可访问。请稍后刷新页面。
        </div>`
	}

	html += `<ul>`

	db.Raw(`
        SELECT file_name, file_size, file_path 
        FROM files 
        WHERE magnet_id = ? 
        ORDER BY file_index`, magnetID).Scan(&files)

	for _, file := range files {
		// 正确编码文件名
		fileName := file.FileName
		fileURL := "/webdav/" + magnetID + "/" + url.PathEscape(file.FilePath)
		size := formatFileSize(file.FileSize)

		// 如果磁力链接未就绪，禁用文件链接
		if magnet.Status != "ready" {
			html += fmt.Sprintf(`<li><span style="color: #999;">%s</span> <span class="size">(%s)</span></li>`,
				fileName, size)
		} else {
			html += fmt.Sprintf(`<li><a href="%s">%s</a> <span class="size">(%s)</span></li>`,
				fileURL, fileName, size)
		}
	}

	html += `</ul>
    <div style="margin-top: 20px;">
        <a href="/admin">返回管理界面</a> | 
        <a href="javascript:location.reload()">刷新页面</a>
    </div>
</body>
</html>`

	w.Write([]byte(html))
}

// 添加状态文本转换函数
func getStatusText(status string) string {
	statusMap := map[string]string{
		"pending":  "准备中",
		"metadata": "获取元数据",
		"ready":    "就绪",
		"error":    "错误",
	}
	if text, exists := statusMap[status]; exists {
		return text
	}
	return status
}

func (h *WebDAVHandler) generatePropfindResponse(path string) string {
	// 确保 PROPFIND 响应也使用 UTF-8
	return `<?xml version="1.0" encoding="UTF-8"?>
<D:multistatus xmlns:D="DAV:">
	<D:response>
		<D:href>/webdav/` + path + `</D:href>
		<D:propstat>
			<D:prop>
				<D:resourcetype><D:collection/></D:resourcetype>
			</D:prop>
			<D:status>HTTP/1.1 200 OK</D:status>
		</D:propstat>
	</D:response>
</D:multistatus>`
}

type rangeInfo struct {
	start int64
	end   int64
}

func parseRangeHeader(rangeHeader string) (*rangeInfo, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, fmt.Errorf("invalid range header")
	}

	rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range format")
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}

	var end int64
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, err
		}
	}

	return &rangeInfo{start: start, end: end}, nil
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getMimeType(filename string) string {
	ext := strings.ToLower(filename[strings.LastIndex(filename, "."):])
	mimeTypes := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".webm": "video/webm",
		".jpg":  "image/jpeg",
		".png":  "image/png",
		".srt":  "text/plain; charset=utf-8", // 字幕文件也设置编码
		".ass":  "text/plain; charset=utf-8",
		".ssa":  "text/plain; charset=utf-8",
	}

	if mime, exists := mimeTypes[ext]; exists {
		return mime
	}
	return "application/octet-stream"
}
