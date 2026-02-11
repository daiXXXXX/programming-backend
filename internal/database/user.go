package database

import (
	"database/sql"
	"errors"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// UserRepository 用户仓库
type UserRepository struct {
	db *DB
}

// NewUserRepository 创建用户仓库
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create 创建用户
func (r *UserRepository) Create(user *models.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := r.db.Exec(query, user.Username, user.Email, user.PasswordHash, user.Role, now, now)
	if err != nil {
		// 检查是否为重复键错误
		if isDuplicateKeyError(err) {
			return ErrUserAlreadyExists
		}
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	user.ID = id
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

// GetByID 根据ID获取用户
func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE id = ?
	`
	user := &models.User{}
	err := r.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetByUsername 根据用户名获取用户
func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE username = ?
	`
	user := &models.User{}
	err := r.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetByEmail 根据邮箱获取用户
func (r *UserRepository) GetByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE email = ?
	`
	user := &models.User{}
	err := r.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdatePassword 更新用户密码
func (r *UserRepository) UpdatePassword(userID int64, passwordHash string) error {
	query := `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, passwordHash, time.Now(), userID)
	return err
}

// UpdateRole 更新用户角色
func (r *UserRepository) UpdateRole(userID int64, role string) error {
	query := `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, role, time.Now(), userID)
	return err
}

// ExistsByUsername 检查用户名是否存在
func (r *UserRepository) ExistsByUsername(username string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE username = ?`
	var count int
	err := r.db.QueryRow(query, username).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(email string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE email = ?`
	var count int
	err := r.db.QueryRow(query, email).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// isDuplicateKeyError 检查是否为重复键错误
func isDuplicateKeyError(err error) bool {
	return err != nil && (
	// MySQL duplicate key error
	contains(err.Error(), "Duplicate entry") ||
		contains(err.Error(), "UNIQUE constraint failed"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
