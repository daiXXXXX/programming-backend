# Programming Online Judge - Backend API

这是编程在线判题系统的后端 API 服务，使用 Go 语言和 Gin 框架开发。

## 技术栈

- **语言**: Go 1.21+
- **框架**: Gin Web Framework
- **数据库**: PostgreSQL
- **ORM**: 原生 SQL (database/sql)
- **代码评测**: Otto (JavaScript 引擎)

## 项目结构

```
programming-backend/
├── cmd/
│   └── server/          # 主程序入口
│       └── main.go
├── internal/
│   ├── config/          # 配置管理
│   │   └── config.go
│   ├── models/          # 数据模型
│   │   └── models.go
│   ├── database/        # 数据库连接和操作
│   │   ├── database.go
│   │   ├── problem.go
│   │   └── submission.go
│   ├── handlers/        # HTTP 处理器
│   │   ├── problem.go
│   │   └── submission.go
│   ├── middleware/      # 中间件
│   │   └── middleware.go
│   └── evaluator/       # 代码评测引擎
│       └── evaluator.go
├── database/
│   ├── schema.sql       # 数据库表结构
│   └── seed.sql         # 初始数据
├── Makefile             # 构建和管理脚本
├── go.mod               # Go 模块依赖
├── .env.example         # 环境变量示例
└── .env                 # 环境变量（不要提交到版本控制）
```

## 快速开始

### 前置要求

- Go 1.21 或更高版本
- PostgreSQL 14 或更高版本

### 1. 安装依赖

```bash
cd backend
go mod download
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，配置数据库等信息
```

### 3. 初始化数据库

```bash
# 创建数据库
createdb programming_oj

# 导入表结构
psql -d programming_oj -f database/schema.sql

# 导入初始数据（可选）
psql -d programming_oj -f database/seed.sql
```

### 4. 运行服务

```bash
go run cmd/server/main.go
```

服务将在 `http://localhost:8080` 启动。

## API 文档

### 健康检查

- `GET /health` - 健康检查

### 题目相关

| 方法   | 路径                | 描述         |
| ------ | ------------------- | ------------ |
| GET    | `/api/problems`     | 获取题目列表 |
| GET    | `/api/problems/:id` | 获取题目详情 |
| POST   | `/api/problems`     | 创建新题目   |
| PUT    | `/api/problems/:id` | 更新题目     |
| DELETE | `/api/problems/:id` | 删除题目     |

### 提交相关

| 方法 | 路径                                  | 描述               |
| ---- | ------------------------------------- | ------------------ |
| POST | `/api/submissions`                    | 提交代码           |
| GET  | `/api/submissions/:id`                | 获取提交详情       |
| GET  | `/api/submissions/user/:userId`       | 获取用户的提交历史 |
| GET  | `/api/submissions/problem/:problemId` | 获取题目的所有提交 |

### 统计相关

| 方法 | 路径                      | 描述             |
| ---- | ------------------------- | ---------------- |
| GET  | `/api/stats/user/:userId` | 获取用户统计信息 |

## API 示例

### 提交代码

```bash
curl -X POST http://localhost:8080/api/submissions \
  -H "Content-Type: application/json" \
  -d '{
    "problemId": 1,
    "code": "function processInput(input) { const lines = input.split(\"\\n\"); return \"0 1\"; }",
    "language": "JavaScript"
  }'
```

### 获取题目列表

```bash
curl http://localhost:8080/api/problems
```

## 开发

### 运行测试

```bash
go test ./...
```

### 构建

```bash
go build -o bin/server cmd/server/main.go
```

### 代码结构说明

- `internal/config`: 配置管理，从环境变量加载配置
- `internal/models`: 数据模型定义，与前端类型保持一致
- `internal/database`: 数据库连接和 Repository 层
- `internal/handlers`: HTTP 处理器，处理请求和响应
- `internal/evaluator`: 代码评测引擎，使用 Otto JavaScript 引擎执行用户代码

## 安全说明

1. 代码评测在沙箱环境中执行（Otto JavaScript 引擎）
2. 执行超时限制为 5 秒
3. 代码长度限制为 10000 字符

## 待实现功能

- [ ] JWT 用户认证
- [ ] 更多编程语言支持（Python, Java 等）
- [ ] 代码运行日志
- [ ] 更安全的沙箱环境
- [ ] WebSocket 实时反馈

## License

MIT
