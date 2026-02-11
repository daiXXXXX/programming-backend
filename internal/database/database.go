package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/daiXXXXX/programming-backend/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	*sql.DB
}

// Connect 连接到数据库
func Connect(cfg *config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	log.Println("Database connection established")

	return &DB{db}, nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.DB.Close()
}
