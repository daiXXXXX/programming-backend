.PHONY: build run clean dev test setup-db seed-db

# Go 相关
BINARY_NAME=server
BUILD_DIR=bin

# 数据库相关
DB_NAME=programming_oj

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/server/main.go

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

dev:
	go run cmd/server/main.go

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

# 数据库操作
setup-db:
	createdb $(DB_NAME) || true
	psql -d $(DB_NAME) -f database/schema.sql

seed-db:
	psql -d $(DB_NAME) -f database/seed.sql

reset-db:
	dropdb $(DB_NAME) || true
	createdb $(DB_NAME)
	psql -d $(DB_NAME) -f database/schema.sql
	psql -d $(DB_NAME) -f database/seed.sql

# 安装依赖
install:
	go mod download

# 整理依赖
tidy:
	go mod tidy

# 帮助
help:
	@echo "Available commands:"
	@echo "  make build      - Build the backend binary"
	@echo "  make run        - Build and run the backend"
	@echo "  make dev        - Run backend in development mode"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make test       - Run tests"
	@echo "  make setup-db   - Create database and run migrations"
	@echo "  make seed-db    - Seed the database with initial data"
	@echo "  make reset-db   - Reset database (drop, create, migrate, seed)"
	@echo "  make install    - Install Go dependencies"
	@echo "  make tidy       - Tidy Go modules"
