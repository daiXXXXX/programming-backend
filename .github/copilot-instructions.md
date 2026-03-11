# Programming Backend - Copilot Instructions

## 项目概述

这是一个在线编程练习平台的后端服务，提供题目管理、代码提交评测、用户认证、排行榜、班级管理等 API。

## 技术栈

- **语言**: Go 1.24
- **HTTP 框架**: Gin
- **数据库**: MySQL (InnoDB, utf8mb4)
- **数据库驱动**: database/sql + go-sql-driver/mysql（原生 SQL，不使用 ORM）
- **认证**: JWT (golang-jwt/jwt/v5) + bcrypt (golang.org/x/crypto)
- **代码评测**: goja (JavaScript 沙箱) + gcc (C 语言编译执行)
- **配置**: godotenv (.env 文件)
- **CORS**: gin-contrib/cors

## 项目结构

```
cmd/server/main.go           # 应用入口
internal/
├── config/config.go          # 配置加载（从 .env）
├── models/models.go          # 数据模型和 DTO
├── database/                 # Repository 层（数据访问）
│   ├── database.go           # DB 连接和连接池配置
│   ├── problem.go            # 题目 CRUD
│   ├── submission.go         # 提交记录 + 统计 + 每日活动
│   ├── user.go               # 用户 CRUD + 排行榜查询
│   └── class.go              # 班级管理
├── handlers/                 # Handler 层（HTTP 处理）
│   ├── auth.go               # 注册/登录/Token 刷新/个人资料
│   ├── problem.go            # 题目 CRUD API
│   ├── submission.go         # 代码提交/查询/统计
│   ├── ranking.go            # 排行榜
│   └── manager.go            # 后台管理
├── middleware/middleware.go   # 中间件（认证/权限/日志）
├── auth/jwt.go               # JWT 生成/验证/角色体系
└── evaluator/evaluator.go    # 代码评测引擎
database/                     # SQL 文件（手动执行）
├── schema.sql                # 建表脚本
├── seed.sql                  # 种子数据
└── migrations/               # 增量迁移
uploads/avatars/              # 用户头像存储
```

## 架构模式

采用三层架构，通过构造函数依赖注入：

```
Router (main.go) → Middleware → Handler → Repository → MySQL
```

- **Handler 层**: 解析请求参数、调用 Repository、组装 HTTP 响应
- **Repository 层**: 手写 SQL，执行数据库操作
- **不使用 Service 层**，业务逻辑直接在 Handler 或 Repository 中处理

## 代码规范

### 命名约定

- Go 标准: PascalCase 导出, camelCase 私有
- JSON tag: camelCase (`json:"userId"`)
- 敏感字段: `json:"-"` 隐藏（如 PasswordHash）
- 可选字段: `json:"field,omitempty"`
- 注释使用中文

### Repository 模式

```go
type XxxRepository struct {
    db *DB
}

func NewXxxRepository(db *DB) *XxxRepository {
    return &XxxRepository{db: db}
}

func (r *XxxRepository) GetByID(id int64) (*models.Xxx, error) {
    query := `SELECT ... FROM xxx WHERE id = ?`
    // 使用 ? 占位符防注入
    // NULL 处理用 sql.NullString / sql.NullInt64 / COALESCE()
    // 未找到返回 fmt.Errorf("xxx not found")
}
```

### 事务模式

```go
tx, err := r.db.Begin()
if err != nil {
    return err
}
defer tx.Rollback()

// ... 多个操作 ...

return tx.Commit()
```

### Handler 模式

```go
func (h *XxxHandler) DoSomething(c *gin.Context) {
    // 1. 解析参数: c.Param(), c.Query(), c.ShouldBindJSON()
    // 2. 参数校验
    // 3. 从中间件获取用户: c.Get("userID")
    // 4. 调用 Repository
    // 5. 返回响应: c.JSON(http.StatusOK, data)
}
```

### 错误响应格式

```go
c.JSON(http.StatusXxx, gin.H{
    "error":   "error_code",      // 机器可读的错误码
    "message": "Human readable",  // 人类可读的描述
})
```

### 自定义错误

```go
var (
    ErrUserNotFound   = fmt.Errorf("user not found")
    ErrDuplicateEmail = fmt.Errorf("email already exists")
)
```

## 认证与权限

- **JWT**: Access Token (1h) + Refresh Token (7d)，HS256 签名
- **密码**: bcrypt cost 12
- **角色层级**: `student` (1) < `instructor` (2) < `admin` (3)
- **中间件链**:
  - `AuthMiddleware` — 必须登录
  - `OptionalAuthMiddleware` — 可选登录（公开接口带上用户信息）
  - `InstructorOnly` — 需要教师/管理员权限
- **Context 传递**: 中间件通过 `c.Set("userID", ...)` 存入，Handler 通过 `c.Get("userID")` 读取

## 数据库规范

- 主键: `BIGINT AUTO_INCREMENT`
- 字符集: `utf8mb4`
- 时间字段: `TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`
- 外键: 使用 `FOREIGN KEY` 约束，子表 `ON DELETE CASCADE`
- 统计表: `user_stats` 作为缓存表，提交时原子更新 (`INSERT ... ON DUPLICATE KEY UPDATE`)
- SQL 文件不会被程序自动执行，需手动导入

## API 路由总览

| 方法   | 路径                                | 权限     | 说明                    |
| ------ | ----------------------------------- | -------- | ----------------------- |
| POST   | /api/auth/register                  | 公开     | 注册                    |
| POST   | /api/auth/login                     | 公开     | 登录                    |
| POST   | /api/auth/refresh                   | 公开     | 刷新 Token              |
| GET    | /api/auth/me                        | 登录     | 获取当前用户            |
| PUT    | /api/auth/password                  | 登录     | 修改密码                |
| PUT    | /api/auth/profile                   | 登录     | 更新个人资料            |
| POST   | /api/auth/avatar                    | 登录     | 上传头像                |
| GET    | /api/problems                       | 可选登录 | 题目列表（?name= 搜索） |
| GET    | /api/problems/:id                   | 可选登录 | 题目详情                |
| POST   | /api/problems                       | 教师     | 创建题目                |
| PUT    | /api/problems/:id                   | 教师     | 更新题目                |
| DELETE | /api/problems/:id                   | 教师     | 删除题目                |
| POST   | /api/submissions                    | 登录     | 提交代码                |
| GET    | /api/submissions/:id                | 登录     | 提交详情                |
| GET    | /api/submissions/user/:userId       | 登录     | 用户提交记录            |
| GET    | /api/submissions/problem/:problemId | 登录     | 题目提交记录            |
| GET    | /api/stats/user/:userId             | 登录     | 用户统计                |
| GET    | /api/stats/user/:userId/activity    | 登录     | 每日活动                |
| GET    | /api/ranking/total                  | 公开     | 总排行榜                |
| GET    | /api/ranking/today                  | 公开     | 今日排行榜              |
| GET    | /api/manager/my-classes             | 教师     | 我的班级                |
| GET    | /api/manager/classes                | 教师     | 所有班级                |
| GET    | /api/manager/classes/:id            | 教师     | 班级详情                |

## 代码评测

- **JavaScript**: 使用 goja 沙箱执行，入口函数 `processInput(input)`
- **C**: gcc 编译 → 运行二进制，stdin/stdout 交互
- **超时控制**: `context.WithTimeout`
- **评分**: (通过用例数 × 100) / 总用例数

## 注意事项

- `gin.Default()` 已自带 Logger 和 Recovery，`middleware.Logger()` 和 `middleware.Recovery()` 是重复注册（当前无害但冗余）
- 分页参数: `page` (默认 1) + `limit` (默认 20)，使用 `LIMIT ? OFFSET ?`
- 文件上传: 头像限制 2MB，支持 jpg/jpeg/png/gif/webp，存储在 `uploads/avatars/`
- 配置通过 `.env` 文件加载，数据库名默认 `xfy_bs`，端口默认 `8080`
