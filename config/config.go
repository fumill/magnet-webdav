package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Torrent  TorrentConfig  `yaml:"torrent"`
	Auth     AuthConfig     `yaml:"auth"`
}

type ServerConfig struct {
	Port         string        `yaml:"port"`
	Env          string        `yaml:"env"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	Domain       string        `yaml:"domain"`
}

type DatabaseConfig struct {
	Driver          string        `yaml:"driver"`
	Host            string        `yaml:"host"`
	Port            string        `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	Name            string        `yaml:"name"`
	SSLMode         string        `yaml:"ssl_mode"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type TorrentConfig struct {
	DownloadDir    string `yaml:"download_dir"`
	CacheSize      int64  `yaml:"cache_size"`
	MaxConnections int    `yaml:"max_connections"`
	UserAgent      string `yaml:"user_agent"`
	ListenPort     int    `yaml:"listen_port"`
}

type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LoadConfig 从指定路径加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	// 如果未指定配置文件路径，使用默认路径
	if configPath == "" {
		defaultPaths := []string{
			"config.yaml",
			"config/config.yaml",
			"/etc/magnet-webdav/config.yaml",
			"./config.yaml",
		}

		for _, path := range defaultPaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}

		if configPath == "" {
			return nil, fmt.Errorf("no configuration file found in default paths")
		}
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析 YAML
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 设置默认值
	cfg.setDefaults()

	// 覆盖环境变量
	cfg.overrideWithEnv()

	// 创建必要的目录
	if err := cfg.createDirectories(); err != nil {
		return nil, err
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	fmt.Printf("Configuration loaded from: %s\n", configPath)
	fmt.Printf("Database driver: %s\n", cfg.Database.Driver)
	return cfg, nil
}

// 设置配置默认值
func (c *Config) setDefaults() {
	if c.Server.Port == "" {
		c.Server.Port = "3000"
	}
	if c.Server.Env == "" {
		c.Server.Env = "development"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.Domain == "" {
		c.Server.Domain = "localhost"
	}

	// 数据库默认配置
	if c.Database.Driver == "" {
		c.Database.Driver = "sqlite"
	}
	if c.Database.Name == "" {
		c.Database.Name = "magnet_webdav.db"
	}
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 10
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 100
	}
	if c.Database.ConnMaxLifetime == 0 {
		c.Database.ConnMaxLifetime = time.Hour
	}

	if c.Torrent.DownloadDir == "" {
		c.Torrent.DownloadDir = "./data/torrents"
	}
	if c.Torrent.CacheSize == 0 {
		c.Torrent.CacheSize = 1024 * 1024 * 1024
	}
	if c.Torrent.MaxConnections == 0 {
		c.Torrent.MaxConnections = 100
	}
	if c.Torrent.UserAgent == "" {
		c.Torrent.UserAgent = "Magnet-WebDAV/1.0"
	}

	// 认证默认配置
	if c.Auth.Username == "" {
		c.Auth.Username = "admin"
	}
	if c.Auth.Password == "" {
		c.Auth.Password = "password"
	}
}

// 使用环境变量覆盖配置
func (c *Config) overrideWithEnv() {
	if port := os.Getenv("PORT"); port != "" {
		c.Server.Port = port
	}
	if env := os.Getenv("ENV"); env != "" {
		c.Server.Env = env
	}
	if domain := os.Getenv("DOMAIN"); domain != "" {
		c.Server.Domain = domain
	}

	if driver := os.Getenv("DB_DRIVER"); driver != "" {
		c.Database.Driver = driver
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		c.Database.Name = dbName
	}
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		c.Database.Host = dbHost
	}
	if dbPort := os.Getenv("DB_PORT"); dbPort != "" {
		c.Database.Port = dbPort
	}
	if dbUser := os.Getenv("DB_USER"); dbUser != "" {
		c.Database.User = dbUser
	}
	if dbPass := os.Getenv("DB_PASSWORD"); dbPass != "" {
		c.Database.Password = dbPass
	}

	if torrentDir := os.Getenv("TORRENT_DIR"); torrentDir != "" {
		c.Torrent.DownloadDir = torrentDir
	}
	if userAgent := os.Getenv("TORRENT_USER_AGENT"); userAgent != "" {
		c.Torrent.UserAgent = userAgent
	}

	if authEnabled := os.Getenv("AUTH_ENABLED"); authEnabled != "" {
		if enabled, err := strconv.ParseBool(authEnabled); err == nil {
			c.Auth.Enabled = enabled
		}
	}
	if username := os.Getenv("WEBDAV_USERNAME"); username != "" {
		c.Auth.Username = username
	}
	if password := os.Getenv("WEBDAV_PASSWORD"); password != "" {
		c.Auth.Password = password
	}
}

// 创建必要的目录
func (c *Config) createDirectories() error {
	dirs := []string{
		c.Torrent.DownloadDir,
		"./data/db",
		"./data/logs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// 验证配置
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	// 验证数据库配置
	supportedDrivers := map[string]bool{
		"sqlite":    true,
		"mysql":     true,
		"postgres":  true,
		"sqlserver": true,
	}

	if !supportedDrivers[c.Database.Driver] {
		return fmt.Errorf("unsupported database driver: %s", c.Database.Driver)
	}

	// 验证数据库特定配置
	switch c.Database.Driver {
	case "mysql", "postgres", "sqlserver":
		if c.Database.Host == "" {
			return fmt.Errorf("database host is required for %s", c.Database.Driver)
		}
		if c.Database.User == "" {
			return fmt.Errorf("database user is required for %s", c.Database.Driver)
		}
	}

	return nil
}

// 获取 Gin 模式
func (c *Config) GetGinMode() string {
	if c.Server.Env == "production" {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}

// 获取数据库连接字符串
func (c *DatabaseConfig) GetConnectionString() string {
	switch c.Driver {
	case "sqlite":
		return fmt.Sprintf("./data/db/%s", c.Name)
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			c.User, c.Password, c.Host, c.Port, c.Name)
	case "postgres":
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
			c.Host, c.User, c.Password, c.Name, c.Port, c.SSLMode)
	case "sqlserver":
		return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s",
			c.User, c.Password, c.Host, c.Port, c.Name)
	default:
		return ""
	}
}
