package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 中间件（已由 gin-contrib/cors 提供，这里保留作为参考）

// Logger 自定义日志中间件
func Logger() gin.HandlerFunc {
	return gin.Logger()
}

// Recovery 恢复中间件
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}

// TODO: 实现 JWT 认证中间件
// func AuthMiddleware() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		// JWT 验证逻辑
// 		c.Next()
// 	}
// }
