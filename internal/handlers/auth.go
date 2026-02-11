package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/auth"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/gin-gonic/gin"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	userRepo   *database.UserRepository
	jwtManager *auth.JWTManager
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(userRepo *database.UserRepository, jwtManager *auth.JWTManager) *AuthHandler {
	return &AuthHandler{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	Role     string `json:"role"` // 可选，默认为 student
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest 刷新Token请求
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	User         UserDTO   `json:"user"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// UserDTO 用户数据传输对象
type UserDTO struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Register 用户注册
// @Summary 用户注册
// @Description 创建新用户账号
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "注册请求"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// 验证用户名格式（只允许字母、数字、下划线）
	if !isValidUsername(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_username",
			"message": "Username can only contain letters, numbers, and underscores",
		})
		return
	}

	// 验证密码强度
	if !isStrongPassword(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "weak_password",
			"message": "Password must contain at least one uppercase letter, one lowercase letter, and one number",
		})
		return
	}

	// 检查用户名是否已存在
	exists, err := h.userRepo.ExistsByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to check username availability",
		})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "username_exists",
			"message": "Username already taken",
		})
		return
	}

	// 检查邮箱是否已存在
	exists, err = h.userRepo.ExistsByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to check email availability",
		})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "email_exists",
			"message": "Email already registered",
		})
		return
	}

	// 哈希密码
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to process password",
		})
		return
	}

	// 设置默认角色
	role := req.Role
	if role == "" {
		role = string(auth.RoleStudent)
	}
	if !auth.IsValidRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_role",
			"message": "Invalid role specified",
		})
		return
	}

	// 创建用户
	user := &models.User{
		Username:     req.Username,
		Email:        strings.ToLower(req.Email),
		PasswordHash: passwordHash,
		Role:         role,
	}

	if err := h.userRepo.Create(user); err != nil {
		if err == database.ErrUserAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "user_exists",
				"message": "User already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to create user",
		})
		return
	}

	// 生成Token对
	tokenPair, err := h.jwtManager.GenerateTokenPair(
		user.ID,
		user.Username,
		user.Email,
		auth.UserRole(user.Role),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to generate token",
		})
		return
	}

	c.JSON(http.StatusCreated, AuthResponse{
		User: UserDTO{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.AccessTokenExpiresAt,
	})
}

// Login 用户登录
// @Summary 用户登录
// @Description 用户登录获取Token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "登录请求"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "Invalid request data",
		})
		return
	}

	// 支持用户名或邮箱登录
	var user *models.User
	var err error

	if strings.Contains(req.Username, "@") {
		user, err = h.userRepo.GetByEmail(strings.ToLower(req.Username))
	} else {
		user, err = h.userRepo.GetByUsername(req.Username)
	}

	if err != nil {
		if err == database.ErrUserNotFound {
			// 为了安全，不透露用户是否存在
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_credentials",
				"message": "Invalid username or password",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to process login",
		})
		return
	}

	// 验证密码
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "invalid_credentials",
			"message": "Invalid username or password",
		})
		return
	}

	// 生成Token对
	tokenPair, err := h.jwtManager.GenerateTokenPair(
		user.ID,
		user.Username,
		user.Email,
		auth.UserRole(user.Role),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to generate token",
		})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		User: UserDTO{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.AccessTokenExpiresAt,
	})
}

// RefreshToken 刷新Token
// @Summary 刷新Token
// @Description 使用Refresh Token获取新的Access Token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshRequest true "刷新请求"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "Invalid request data",
		})
		return
	}

	tokenPair, err := h.jwtManager.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "invalid_token",
			"message": "Invalid or expired refresh token",
		})
		return
	}

	// 获取用户信息
	claims, _ := h.jwtManager.ValidateToken(tokenPair.AccessToken)
	user, err := h.userRepo.GetByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "user_not_found",
			"message": "User no longer exists",
		})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		User: UserDTO{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.AccessTokenExpiresAt,
	})
}

// GetCurrentUser 获取当前用户信息
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的详细信息
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} UserDTO
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/me [get]
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	// 从上下文获取用户ID（由认证中间件设置）
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Not authenticated",
		})
		return
	}

	user, err := h.userRepo.GetByID(userID.(int64))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "user_not_found",
			"message": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, UserDTO{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	})
}

// ChangePassword 修改密码
// @Summary 修改密码
// @Description 修改当前用户密码
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6,max=128"`
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "Invalid request data",
		})
		return
	}

	userID, _ := c.Get("userID")
	user, err := h.userRepo.GetByID(userID.(int64))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "user_not_found",
			"message": "User not found",
		})
		return
	}

	// 验证旧密码
	if !auth.CheckPassword(req.OldPassword, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "invalid_password",
			"message": "Current password is incorrect",
		})
		return
	}

	// 验证新密码强度
	if !isStrongPassword(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "weak_password",
			"message": "Password must contain at least one uppercase letter, one lowercase letter, and one number",
		})
		return
	}

	// 哈希新密码
	newPasswordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to process password",
		})
		return
	}

	// 更新密码
	if err := h.userRepo.UpdatePassword(user.ID, newPasswordHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "server_error",
			"message": "Failed to update password",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password updated successfully",
	})
}

// 验证用户名格式
func isValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 50 {
		return false
	}
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// 验证密码强度
func isStrongPassword(password string) bool {
	if len(password) < 6 {
		return false
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
