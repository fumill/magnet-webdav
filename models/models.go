package models

import (
	"time"
)

type Magnet struct {
	ID           string    `json:"id" gorm:"primaryKey;size:64"` // infoHash
	MagnetURI    string    `json:"magnet_uri" gorm:"type:text;not null"`
	Name         string    `json:"name" gorm:"size:512"`
	TotalSize    int64     `json:"total_size" gorm:"default:0"`
	FileCount    int       `json:"file_count" gorm:"default:0"`
	Status       string    `json:"status" gorm:"size:32;default:'pending';index"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	LastAccessed time.Time `json:"last_accessed" gorm:"autoCreateTime;index"`
	AccessCount  int64     `json:"access_count" gorm:"default:0"`
}

type File struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	MagnetID  string    `json:"magnet_id" gorm:"size:64;not null;index"`
	FilePath  string    `json:"file_path" gorm:"type:text;not null"`
	FileName  string    `json:"file_name" gorm:"size:512;not null"`
	FileSize  int64     `json:"file_size" gorm:"default:0"`
	FileIndex int       `json:"file_index" gorm:"default:0"`
	MimeType  string    `json:"mime_type" gorm:"size:128"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"` // 添加更新时间字段
}

type Stats struct {
	TotalMagnets   int64 `json:"total_magnets"`
	TotalFiles     int64 `json:"total_files"`
	ActiveTorrents int   `json:"active_torrents"`
}
