package handlers

import (
	"magnet-webdav/models"
	"magnet-webdav/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	torrentService *services.TorrentService
}

func NewAPIHandler(torrentService *services.TorrentService) *APIHandler {
	return &APIHandler{
		torrentService: torrentService,
	}
}

type AddMagnetRequest struct {
	MagnetURI string `json:"magnet_uri" binding:"required"`
}

func (h *APIHandler) AddMagnet(c *gin.Context) {
	var req AddMagnetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	magnet, err := h.torrentService.AddMagnet(req.MagnetURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, magnet)
}

func (h *APIHandler) ListMagnets(c *gin.Context) {
	var magnets []models.Magnet

	db := h.torrentService.DB()
	if err := db.Order("last_accessed DESC").Find(&magnets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, magnets)
}

func (h *APIHandler) ListFiles(c *gin.Context) {
	magnetID := c.Param("id")

	var files []models.File
	db := h.torrentService.DB()

	if err := db.Where("magnet_id = ?", magnetID).Order("file_index").Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

func (h *APIHandler) RemoveMagnet(c *gin.Context) {
	magnetID := c.Param("id")

	// 从数据库中删除相关记录
	db := h.torrentService.DB()

	// 删除文件记录
	db.Where("magnet_id = ?", magnetID).Delete(&models.File{})

	// 删除磁力记录
	if err := db.Where("id = ?", magnetID).Delete(&models.Magnet{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Magnet removed successfully"})
}

func (h *APIHandler) GetStats(c *gin.Context) {
	var stats models.Stats

	db := h.torrentService.DB()

	// 获取统计信息
	db.Model(&models.Magnet{}).Count(&stats.TotalMagnets)
	db.Model(&models.File{}).Count(&stats.TotalFiles)

	// 获取活跃种子数量
	stats.ActiveTorrents = h.torrentService.GetActiveTorrentCount()

	c.JSON(http.StatusOK, stats)
}
