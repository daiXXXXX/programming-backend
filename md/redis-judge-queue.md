# Redis 评测队列（异步提交）

> 日期: 2026-03-20  
> 类型: 基于 Redis List 的异步评测队列

## 一、改动原因

当前 `POST /api/submissions` 接口是**同步评测**：用户提交代码后，服务器在同一个 HTTP 请求内完成编译 + 运行所有测试用例，整个过程耗时可能 1-10 秒（尤其是 C 语言需要编译）。这带来以下问题：

1. **用户体验差** — 前端需要等待很长时间才能收到响应，出现「转圈」
2. **请求超时风险** — 评测超时可能导致 HTTP 连接被网关/代理断开
3. **并发瓶颈** — 多人同时提交时，Gin 的 goroutine 全部阻塞在评测上，服务吞吐量急剧下降
4. **无法扩展** — 同步模式无法独立扩展评测能力

## 二、方案设计

### 架构变化

```
改动前（同步）:
  用户提交 → Handler 评测代码 → 保存结果 → 返回结果

改动后（异步）:
  用户提交 → Handler 保存 Pending → 推入 Redis 队列 → 立即返回 202
                                        ↓
                              Worker 消费 → 评测代码 → 更新数据库
                                        ↓
                              WebSocket 推送结果给用户
```

### 降级策略

- **Redis 可用** → 异步模式，接口返回 `202 Accepted`，后台 Worker 评测
- **Redis 不可用** → 自动降级为同步模式，接口行为与改动前完全一致
- **队列推送失败** → 单次降级为同步评测，不丢失提交

## 三、改动文件清单

```
programming-backend/
├─ internal/
│  ├─ worker/
│  │  └─ worker.go           ← 新增：评测队列消费者
│  ├─ cache/
│  │  └─ redis.go            ← 修改：增加队列操作方法
│  ├─ models/
│  │  └─ models.go           ← 修改：增加 StatusPending、WSTypeJudgeResult
│  ├─ database/
│  │  └─ submission.go       ← 修改：增加 CreatePending、UpdateResult 方法
│  └─ handlers/
│     └─ submission.go       ← 修改：异步提交逻辑 + 同步降级
├─ cmd/server/
│  └─ main.go                ← 修改：初始化并启动 Worker
```

## 四、改动内容详述

### 4.1 新增 `internal/worker/worker.go`

评测队列的核心消费者，主要结构和流程：

- **`JudgeTask`** — 入队/出队的任务结构体（submissionID、problemID、userID、code、language）
- **`JudgeWorker`** — 消费者主体，支持配置并发数（默认 2 个 goroutine）
- **消费循环** — 每个 goroutine 通过 `BRPOP` 阻塞等待任务（2s 超时轮询，便于检查停止信号）
- **评测流程**：获取题目测试用例 → 调用 Evaluator 评测 → 调用 `UpdateResult` 更新数据库 → 通过 WebSocket 推送结果
- **优雅退出** — 通过 `stopCh` channel 通知所有消费者退出

### 4.2 修改 `internal/cache/redis.go`

新增 4 个队列相关方法：

| 方法                                        | 说明                         |
| ------------------------------------------- | ---------------------------- |
| `QueuePush(ctx, value, keyParts...)`        | LPUSH 入队（左进）           |
| `QueuePop(ctx, dest, timeout, keyParts...)` | BRPOP 出队（右出，阻塞等待） |
| `QueueLen(ctx, keyParts...)`                | 获取队列长度                 |
| `IsAvailable()`                             | 检查 Redis 是否可用          |

队列使用 Redis List 数据结构，`LPUSH + BRPOP` 实现 FIFO（先进先出）。

### 4.3 修改 `internal/models/models.go`

- 新增 `StatusPending SubmissionStatus = "Pending"` — 提交后等待评测的状态
- 新增 `WSTypeJudgeResult WSMessageType = "judge_result"` — 评测完成的 WebSocket 消息类型

### 4.4 修改 `internal/database/submission.go`

新增两个方法：

| 方法                                                     | 说明                                                     |
| -------------------------------------------------------- | -------------------------------------------------------- |
| `CreatePending(submission)`                              | 创建 Pending 状态的提交记录（不含测试结果，不更新统计）  |
| `UpdateResult(submissionID, status, score, testResults)` | 评测完成后更新状态、插入测试结果、更新用户统计和每日活动 |

`UpdateResult` 在一个事务内完成所有更新，保证数据一致性。

### 4.5 修改 `internal/handlers/submission.go`

`SubmitCode` 方法重构为两条路径：

**异步路径（Redis 可用）**：

1. 验证请求参数和用户身份
2. 调用 `CreatePending` 创建 Pending 记录
3. 构建 `JudgeTask` 推入 Redis 队列
4. 返回 `202 Accepted`（含 submissionID 和 `"status": "Pending"`）

**同步路径（降级）**：

- Redis 不可用时调用 `syncSubmit`（与原逻辑完全一致）
- 队列推送失败时调用 `syncEvaluate`（对已创建的 Pending 记录直接评测）

### 4.6 修改 `cmd/server/main.go`

- 新增 `worker` 包 import
- 在 WebSocket Hub 启动后，如果 Redis 可用，创建并启动 `JudgeWorker`（2 并发）
- `NewSubmissionHandler` 增加 `redisCache` 参数
- `defer judgeWorker.Stop()` 确保服务退出时优雅关闭消费者

## 五、队列 Key 设计

| Key              | 数据结构   | 说明               |
| ---------------- | ---------- | ------------------ |
| `oj:queue:judge` | Redis List | 评测任务队列，FIFO |

每个队列元素是一个 JSON 序列化的 `JudgeTask`：

```json
{
  "submissionId": 42,
  "problemId": 3,
  "userId": 1,
  "code": "function processInput(input) { ... }",
  "language": "JavaScript"
}
```

## 六、WebSocket 通知格式

评测完成后，Worker 通过 WebSocket 向提交用户推送结果：

```json
{
  "type": "judge_result",
  "content": {
    "submissionId": 42,
    "status": "Accepted",
    "score": 100
  },
  "timestamp": "2026-03-20T14:30:00Z"
}
```

前端收到该消息后可以自动刷新提交状态，无需轮询。

## 七、前端适配指南

前端需要做以下调整来适配异步评测：

1. **`POST /api/submissions` 响应码变化**：
   - 异步模式返回 `202 Accepted`（而非 `201 Created`）
   - 响应体中 `status` 为 `"Pending"`

2. **获取最终结果的方式**（二选一）：
   - **WebSocket**（推荐）：监听 `judge_result` 消息类型，收到后更新界面
   - **轮询**：提交后每 1-2 秒调用 `GET /api/submissions/:id`，直到 `status` 不再是 `"Pending"`

3. **UI 提示**：提交后显示 "评测中..." 状态，收到结果后切换为最终状态

## 八、数据流时序图

```
用户          前端            后端 Handler        Redis Queue        Worker          数据库          WebSocket
 |             |                  |                  |                  |                |               |
 |--提交代码-->|                  |                  |                  |                |               |
 |             |---POST /api/--->|                  |                  |                |               |
 |             |                  |--CreatePending-->|                  |                |-->INSERT----->|
 |             |                  |--LPUSH task----->|                  |                |               |
 |             |<--202 Pending---|                  |                  |                |               |
 |<--评测中..--|                  |                  |                  |                |               |
 |             |                  |                  |--BRPOP task----->|                |               |
 |             |                  |                  |                  |--评测代码----->|               |
 |             |                  |                  |                  |--UpdateResult->|-->UPDATE----->|
 |             |                  |                  |                  |                |               |
 |             |                  |                  |                  |---SendToUser-->|-->WS push---->|
 |             |<----------------------------------------------judge_result------------|               |
 |<--结果-----|                  |                  |                  |                |               |
```
