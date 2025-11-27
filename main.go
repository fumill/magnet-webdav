package main

import (
	"flag"
	"fmt"
	"log"
	"magnet-webdav/config"
	"magnet-webdav/database"
	"magnet-webdav/handlers"
	"magnet-webdav/middleware"
	"magnet-webdav/services"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
)

var (
	configPath = flag.String("c", "", "Path to configuration file")
	version    = flag.Bool("v", false, "Show version information")
)

const (
	AppVersion = "1.0.0"
	AppName    = "Magnet WebDAV Server"
)

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("%s v%s\n", AppName, AppVersion)
		fmt.Println("A high-performance magnet link WebDAV server")
		os.Exit(0)
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// 初始化数据库
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// 初始化服务
	torrentService := services.NewTorrentService(cfg, db)

	// 启动服务
	if err := torrentService.Start(); err != nil {
		log.Fatal("Failed to start torrent service:", err)
	}

	// 初始化处理器
	apiHandler := handlers.NewAPIHandler(torrentService)
	webdavHandler := handlers.NewWebDAVHandler(torrentService, cfg)

	// 设置路由
	router := setupRouter(apiHandler, webdavHandler, cfg)

	// 启动 HTTP 服务器
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	log.Printf("Starting %s v%s", AppName, AppVersion)
	log.Printf("Server running on :%s", cfg.Server.Port)
	log.Printf("Database: %s", cfg.Database.Driver)
	log.Printf("WebDAV URL: http://localhost:%s/webdav/", cfg.Server.Port)
	log.Printf("Admin interface: http://localhost:%s/admin", cfg.Server.Port)

	if cfg.Auth.Enabled {
		log.Printf("WebDAV Authentication: Enabled (username: %s)", cfg.Auth.Username)
	} else {
		log.Printf("WebDAV Authentication: Disabled")
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// 清理资源
	torrentService.Stop()

	log.Println("Server shutdown complete")
}

func setupRouter(apiHandler *handlers.APIHandler, webdavHandler *handlers.WebDAVHandler, cfg *config.Config) http.Handler {
	gin.SetMode(cfg.GetGinMode())

	// 配置自定义恢复中间件
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.CustomRecovery(func(c *gin.Context, err interface{}) {
		log.Printf("Panic recovered: %v", err)
		c.JSON(500, gin.H{
			"error": "Internal server error",
		})
	}))

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"version":   AppVersion,
			"database":  cfg.Database.Driver,
			"auth":      cfg.Auth.Enabled,
		})
	})

	// API 路由（不需要认证）
	api := router.Group("/api")
	{
		api.POST("/magnets", apiHandler.AddMagnet)
		api.GET("/magnets", apiHandler.ListMagnets)
		api.GET("/magnets/:id/files", apiHandler.ListFiles)
		api.DELETE("/magnets/:id", apiHandler.RemoveMagnet)
		api.GET("/stats", apiHandler.GetStats)
	}

	// WebDAV 路由（需要认证）
	webdavGroup := router.Group("/webdav")
	if cfg.Auth.Enabled {
		webdavGroup.Use(middleware.AuthMiddleware(cfg))
	}
	{
		webdavGroup.Any("/*path", gin.WrapH(webdavHandler))
	}

	// 管理界面（不需要认证）
	router.Static("/admin", "./web/admin")
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/admin")
	})

	return router
}
