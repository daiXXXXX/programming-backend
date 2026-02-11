package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token has expired")
	ErrInvalidClaims = errors.New("invalid token claims")
)

// UserRole 用户角色
type UserRole string

const (
	RoleStudent    UserRole = "student"
	RoleInstructor UserRole = "instructor"
	RoleAdmin      UserRole = "admin"
)

// TokenType Token类型
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims JWT claims 结构
type Claims struct {
	UserID   int64     `json:"userId"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
	Role     UserRole  `json:"role"`
	Type     TokenType `json:"type"`
	jwt.RegisteredClaims
}

// JWTManager JWT管理器
type JWTManager struct {
	secretKey          []byte
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
}

// NewJWTManager 创建JWT管理器
func NewJWTManager(secretKey string) *JWTManager {
	return &JWTManager{
		secretKey:          []byte(secretKey),
		accessTokenExpiry:  15 * time.Minute,   // Access Token 15分钟过期
		refreshTokenExpiry: 7 * 24 * time.Hour, // Refresh Token 7天过期
	}
}

// GenerateTokenPair 生成Access Token和Refresh Token对
func (m *JWTManager) GenerateTokenPair(userID int64, username, email string, role UserRole) (*TokenPair, error) {
	accessToken, accessExp, err := m.generateToken(userID, username, email, role, AccessToken, m.accessTokenExpiry)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExp, err := m.generateToken(userID, username, email, role, RefreshToken, m.refreshTokenExpiry)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessExp,
		RefreshTokenExpiresAt: refreshExp,
	}, nil
}

// generateToken 生成Token
func (m *JWTManager) generateToken(userID int64, username, email string, role UserRole, tokenType TokenType, expiry time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(expiry)

	claims := &Claims{
		UserID:   userID,
		Username: username,
		Email:    email,
		Role:     role,
		Type:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "programming-platform",
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// ValidateToken 验证Token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	return claims, nil
}

// RefreshAccessToken 使用Refresh Token刷新Access Token
func (m *JWTManager) RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	if claims.Type != RefreshToken {
		return nil, ErrInvalidToken
	}

	return m.GenerateTokenPair(claims.UserID, claims.Username, claims.Email, claims.Role)
}

// TokenPair Token对
type TokenPair struct {
	AccessToken           string    `json:"accessToken"`
	RefreshToken          string    `json:"refreshToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

// HashPassword 使用bcrypt哈希密码
func HashPassword(password string) (string, error) {
	// bcrypt cost 12 提供良好的安全性和性能平衡
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateRandomToken 生成随机Token（用于密码重置等）
func GenerateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// IsValidRole 检查角色是否有效
func IsValidRole(role string) bool {
	switch UserRole(role) {
	case RoleStudent, RoleInstructor, RoleAdmin:
		return true
	default:
		return false
	}
}

// HasPermission 检查用户是否有权限
func HasPermission(userRole UserRole, requiredRole UserRole) bool {
	roleHierarchy := map[UserRole]int{
		RoleStudent:    1,
		RoleInstructor: 2,
		RoleAdmin:      3,
	}

	return roleHierarchy[userRole] >= roleHierarchy[requiredRole]
}
