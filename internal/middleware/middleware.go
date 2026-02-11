package middleware

import (
	"net/http"
	"strings"

	"github.com/daiXXXXX/programming-backend/internal/auth"
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

// AuthMiddleware JWT认证中间件
func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "missing_token",
				"message": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Bearer token 格式
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_token_format",
				"message": "Authorization header format must be 'Bearer {token}'",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil {
			switch err {
			case auth.ErrExpiredToken:
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "token_expired",
					"message": "Token has expired",
				})
			default:
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "invalid_token",
					"message": "Invalid token",
				})
			}
			c.Abort()
			return
		}

		// 验证是Access Token而非Refresh Token
		if claims.Type != auth.AccessToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_token_type",
				"message": "Invalid token type",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// OptionalAuthMiddleware 可选的认证中间件，不强制要求登录
func OptionalAuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil || claims.Type != auth.AccessToken {
			c.Next()
			return
		}

		// 将用户信息存入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// RoleMiddleware 角色权限中间件
func RoleMiddleware(requiredRole auth.UserRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Not authenticated",
			})
			c.Abort()
			return
		}

		userRole := role.(auth.UserRole)
		if !auth.HasPermission(userRole, requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "Insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// InstructorOnly 仅教师和管理员可访问
func InstructorOnly() gin.HandlerFunc {
	return RoleMiddleware(auth.RoleInstructor)
}

// AdminOnly 仅管理员可访问
func AdminOnly() gin.HandlerFunc {
	return RoleMiddleware(auth.RoleAdmin)
}
