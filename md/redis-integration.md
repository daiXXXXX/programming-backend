# Redis 缓存接入方案

> 日期: 2026-03-20  
> 类型: 最小接入方案（仅缓存层，不改变业务逻辑）

## 一、为什么引入 Redis

| 场景                         | 现状                                      | 问题                   |
| ---------------------------- | ----------------------------------------- | ---------------------- |
| 排行榜 `/api/ranking/*`      | 每次请求都执行 `JOIN` 查询                | 高频访问时数据库压力大 |
| 题目列表 `/api/problems`     | N+1 查询（主表 + 标签 + 示例 + 测试用例） | 响应慢，重复查询多     |
| 题目详情 `/api/problems/:id` | 同上                                      | 热门题目被反复查询     |

引入 Redis 后，上述接口在缓存命中时直接返回内存数据，**响应时间从 10-50ms 降至 <1ms**。

## 二、方案设计原则

1. **优雅降级** — Redis 不可用时自动回退到直接查库，不影响服务可用性
2. **最小侵入** — 只在 Handler 层加缓存读写，不修改 Repository 层
3. **不引入缓存不一致风险** — 写操作（增/改/删）后立即清除相关缓存
4. **短 TTL** — 缓存自然过期兜底，避免长期脏数据

## 三、改动文件清单

```
programming-backend/
├─ internal/
│  ├─ cache/
│  │  └─ redis.go          ← 新增：Redis 缓存封装
│  ├─ config/
│  │  └─ config.go         ← 修改：增加 RedisConfig
│  └─ handlers/
│     ├─ problem.go        ← 修改：题目读取加缓存、写入清缓存
│     └─ ranking.go        ← 修改：排行榜加缓存
├─ cmd/server/
│  └─ main.go              ← 修改：初始化 Redis，注入到 Handler
├─ .env.example            ← 修改：增加 Redis 环境变量
└─ go.mod / go.sum         ← 修改：增加 go-redis/v9 依赖
```

## 四、改动内容详述

### 4.1 新增 `internal/cache/redis.go`

新建 Redis 缓存封装层，核心结构体 `Cache` 持有一个 `redis.Client`。提供 4 个通用方法：

| 方法            | 签名                                | 说明                                   |
| --------------- | ----------------------------------- | -------------------------------------- |
| `Get`           | `Get(ctx, dest, keyParts...) bool`  | 读取缓存并 JSON 反序列化，返回是否命中 |
| `Set`           | `Set(ctx, value, ttl, keyParts...)` | JSON 序列化后写入缓存                  |
| `Delete`        | `Delete(ctx, keyParts...)`          | 精确删除某个 key                       |
| `DeletePattern` | `DeletePattern(ctx, pattern)`       | 按通配符批量删除                       |

**关键设计**：所有方法在 `Cache` 为 `nil`（Redis 不可用）时安全返回，不 panic，这是优雅降级的基础。

### 4.2 修改 `internal/config/config.go`

- 新增 `RedisConfig` 结构体（Host / Port / Password / DB / Prefix）
- 在 `Config` 中添加 `Redis RedisConfig` 字段
- 在 `Load()` 中从环境变量读取 Redis 配置，均有默认值

### 4.3 修改 `internal/handlers/ranking.go`

- `RankingHandler` 新增 `cache *cache.Cache` 字段
- `NewRankingHandler` 签名变为接收 `(userRepo, cache)` 两个参数
- `GetTotalSolvedRanking`：先尝试从 Redis 读取 `ranking:total:{limit}`，命中直接返回；未命中则查库后写入缓存，TTL 30s
- `GetTodaySolvedRanking`：同上，TTL 15s（今日榜变化更频繁）

### 4.4 修改 `internal/handlers/problem.go`

- `ProblemHandler` 新增 `cache *cache.Cache` 字段
- `NewProblemHandler` 签名变为接收 `(repo, cache)` 两个参数
- `GetProblems`：无搜索关键词时缓存全量列表 `problems:list`，TTL 60s
- `GetProblem`：缓存单题详情 `problems:detail:{id}`，TTL 120s
- `CreateProblem / UpdateProblem / DeleteProblem`：操作完成后调用 `invalidateProblemCache()` 清除列表缓存和对应详情缓存

### 4.5 修改 `cmd/server/main.go`

- 新增 `cache` 包 import
- 在数据库连接后、仓库初始化前创建 Redis 缓存实例
- `NewProblemHandler(problemRepo, redisCache)` — 注入缓存
- `NewRankingHandler(userRepo, redisCache)` — 注入缓存
- Redis 连接失败时 `redisCache` 为 `nil`，Handler 自动降级

### 4.6 修改 `.env.example`

新增 Redis 相关环境变量模板：

```dotenv
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_PREFIX=oj
```

### 4.7 修改 `go.mod` / `go.sum`

新增依赖 `github.com/redis/go-redis/v9 v9.18.0` 及其传递依赖。

## 五、缓存 Key 设计

| Key 模式                   | 说明                     | TTL  |
| -------------------------- | ------------------------ | ---- |
| `oj:ranking:total:{limit}` | 总刷题数排行榜           | 30s  |
| `oj:ranking:today:{limit}` | 今日刷题数排行榜         | 15s  |
| `oj:problems:list`         | 题目列表（无搜索条件时） | 60s  |
| `oj:problems:detail:{id}`  | 单个题目详情             | 120s |

前缀 `oj` 可通过环境变量 `REDIS_PREFIX` 自定义。

## 六、缓存策略

### 读取流程

```
请求 → 检查 Redis 中是否有缓存
         ├─ 命中 → 直接返回（不查数据库）
         └─ 未命中 → 查数据库 → 写入 Redis → 返回
```

### 写入/删除时的缓存失效

| 操作     | 清除的缓存                                     |
| -------- | ---------------------------------------------- |
| 创建题目 | `oj:problems:list`                             |
| 更新题目 | `oj:problems:list` + `oj:problems:detail:{id}` |
| 删除题目 | `oj:problems:list` + `oj:problems:detail:{id}` |
| 排行榜   | 不主动清除，依赖 TTL 自然过期（15-30s）        |

## 七、环境变量配置

在 `.env` 中添加（均有默认值，不配置也能启动）：

```dotenv
# Redis Configuration（可选，不配置则以无缓存模式运行）
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_PREFIX=oj
```

## 八、如何启用

### 1. 安装 Redis

**Windows（开发环境）**：

```bash
# 使用 Scoop
scoop install redis

# 或使用 WSL
sudo apt install redis-server
sudo service redis-server start
```

**Linux/Mac**：

```bash
# Ubuntu/Debian
sudo apt install redis-server

# macOS
brew install redis && brew services start redis
```

### 2. 验证 Redis 连接

```bash
redis-cli ping
# 应返回: PONG
```

### 3. 启动后端服务

```bash
go run cmd/server/main.go
```

启动日志中会看到：

- 连接成功：`[Cache] Redis 连接成功: localhost:6379`
- 连接失败：`[Cache] Redis 连接失败，将以无缓存模式运行: ...`（服务照常运行）

## 九、改动原因

1. **排行榜查询开销大** — 每次请求都执行 `users LEFT JOIN user_stats` 并排序，排行榜是公开接口且访问频率高，适合短 TTL 缓存
2. **题目列表 N+1 问题** — `GetAll` 查主表后对每条结果再查标签、示例、测试用例，共 4 次 SQL/题目；缓存后只需在首次查询时执行
3. **题目详情重复查询** — 热门题目被大量用户反复打开，内容几乎不变，适合较长 TTL 缓存
4. **为后续功能铺路** — 缓存层建好后，后续可快速接入评测队列、Token 黑名单、接口限流等 Redis 高级功能

## 十、后续扩展方向

| 方向                 | 说明                                 | 优先级 |
| -------------------- | ------------------------------------ | ------ |
| 提交评测异步队列     | 用 Redis List 做任务队列，异步评测   | ⭐⭐⭐ |
| WebSocket 跨实例广播 | 用 Redis Pub/Sub 替代内存 Hub        | ⭐⭐   |
| JWT Token 黑名单     | 退出登录时将 token 加入 Redis 黑名单 | ⭐⭐   |
| 接口限流             | 用 Redis 计数器做 Rate Limiting      | ⭐     |
| Session / 刷新 Token | 将刷新 token 存入 Redis 支持主动失效 | ⭐     |
