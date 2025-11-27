package database

import (
	"fmt"
	"log"
	"magnet-webdav/config"
	"magnet-webdav/models"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库实例
var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	var err error

	// 根据配置选择数据库驱动
	switch cfg.Database.Driver {
	case "sqlite":
		dialector = sqlite.Open(cfg.Database.GetConnectionString())
	case "mysql":
		// 确保 MySQL 使用 UTF8MB4 编码
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)
		dialector = mysql.Open(dsn)
	case "postgres":
		dialector = postgres.Open(cfg.Database.GetConnectionString())
	case "sqlserver":
		dialector = sqlserver.Open(cfg.Database.GetConnectionString())
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Database.Driver)
	}
	// GORM 配置
	gormConfig := &gorm.Config{}
	if cfg.Server.Env == "development" {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	} else {
		gormConfig.Logger = logger.Default.LogMode(logger.Warn)
	}

	// 连接数据库
	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 自动迁移
	if err := autoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// 设置全局实例
	DB = db

	log.Printf("Database connected successfully: %s", cfg.Database.Driver)
	return db, nil
}

// autoMigrate 自动迁移数据库表
func autoMigrate(db *gorm.DB) error {
	// 注册所有模型
	models := []interface{}{
		&models.Magnet{},
		&models.File{},
	}

	// 执行迁移
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate model %T: %w", model, err)
		}
	}

	log.Printf("Database migration completed successfully")
	return nil
}

// Close 关闭数据库连接
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return DB
}

// HealthCheck 数据库健康检查
func HealthCheck() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Ping()
}

// GetStats 获取数据库统计信息
func GetStats() map[string]interface{} {
	if DB == nil {
		return nil
	}

	stats := make(map[string]interface{})

	// 获取表统计
	var magnetCount int64
	var fileCount int64

	DB.Model(&models.Magnet{}).Count(&magnetCount)
	DB.Model(&models.File{}).Count(&fileCount)

	stats["magnets_count"] = magnetCount
	stats["files_count"] = fileCount

	// 获取数据库连接池统计
	sqlDB, err := DB.DB()
	if err == nil {
		stats["max_open_connections"] = sqlDB.Stats().MaxOpenConnections
		stats["open_connections"] = sqlDB.Stats().OpenConnections
		stats["in_use"] = sqlDB.Stats().InUse
		stats["idle"] = sqlDB.Stats().Idle
	}

	return stats
}
