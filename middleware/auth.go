package middleware

import (
	"encoding/base64"
	"magnet-webdav/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware WebDAV 认证中间件
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否启用认证
		if !cfg.Auth.Enabled {
			c.Next()
			return
		}

		// 检查认证头
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Header("WWW-Authenticate", `Basic realm="Magnet WebDAV"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 解析 Basic Auth
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Basic" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 解码认证信息
		payload, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		pair := strings.SplitN(string(payload), ":", 2)
		if len(pair) != 2 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 验证用户名和密码
		username := pair[0]
		password := pair[1]

		if username != cfg.Auth.Username || password != cfg.Auth.Password {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 认证通过
		c.Next()
	}
}
