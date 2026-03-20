# 前端异步提交评测适配

## 改动背景

后端已实现基于 Redis 队列的异步评测功能：提交代码后返回 `202 + Pending` 状态，评测完成后通过 WebSocket 推送 `judge_result` 消息。前端需要适配这一异步流程，支持"提交→等待→接收结果"的完整交互体验。

## 改动文件清单

### 1. `src/lib/api.ts` — API 类型与请求方法

**改动内容：**

- `SubmissionStatus` 类型新增 `'Pending'` 状态
- `WSMessageType` 类型新增 `'judge_result'` 消息类型
- `submitCode()` 方法重写：不再使用通用的 `this.request<T>()` 方法，而是直接使用 `fetch` 进行请求，以便区分处理 201（同步评测）和 202（异步评测）两种响应码。两种情况下都直接返回 `Submission` 对象，异步模式下 `status` 为 `'Pending'`。

### 2. `src/lib/types.ts` — 共享类型定义

**改动内容：**

- `SubmissionStatus` 类型新增 `'Pending'` 状态（与 `api.ts` 保持一致）

### 3. `src/store/appStore.ts` — Zustand 全局状态

**改动内容：**

- 新增 `updateSubmission(id: number, updates: Partial<Submission>)` action
- 用于在收到 WebSocket 评测结果时，按 ID 原地更新 Pending 状态的提交记录

### 4. `src/hooks/use-submissions.ts` — 提交管理 Hook

**改动内容：**

- 从 store 中解构新增的 `updateSubmission` 方法
- `submitCode` 回调新增 Pending 分支：提交成功且状态为 Pending 时，显示"评测中..."提示，不再立即显示结果消息
- 将 `updateSubmission` 方法通过 hook 返回值暴露给组件使用

### 5. `src/hooks/use-websocket.ts` — WebSocket Hook

**改动内容：**

- 在 `handleMessage` 的 switch 中新增 `case 'judge_result'`，收到评测结果时 dispatch 给订阅的监听器

### 6. `src/components/ProblemDetail.tsx` — 题目详情与提交组件

**改动内容：**

- 引入 `useWebSocket` 和 `useAppStore`
- 新增 `useEffect` 监听 `judge_result` 类型的 WebSocket 消息：
  - 收到结果后调用 `updateSubmission` 更新 store 中的提交记录
  - 如果是当前正在等待的提交（`lastSubmission`），同步更新本地状态
  - 恢复提交按钮状态
- `handleSubmit` 增加 Pending 判断：
  - 异步模式下保持 `isSubmitting=true`，直到 WebSocket 推送结果
  - 同步模式下 1 秒后恢复
- `getStatusIcon` 新增 Pending 分支：显示蓝色旋转 `LoadingOutlined`
- `getStatusColor` 新增 Pending 分支：返回 `'processing'`（蓝色动画标签）
- 提交按钮文案：Pending 时显示"评测中..."
- 测试结果卡片：Pending 时显示加载动画和提示文案，评测完成后切换为正常结果展示

### 7. `src/components/SubmissionHistory.tsx` — 提交历史列表

**改动内容：**

- 引入 `LoadingOutlined` 图标
- `getStatusIcon` 新增 Pending 分支：蓝色旋转加载图标
- `getStatusColor` 新增 Pending 分支：`'processing'`

### 8. `src/lib/i18n/zh.ts` — 中文国际化

**新增 key：**

- `'messages.judging'`: `'评测中...'`
- `'messages.judgingDesc'`: `'代码已提交，正在评测中，请稍候...'`

### 9. `src/lib/i18n/en.ts` — 英文国际化

**新增 key：**

- `'messages.judging'`: `'Judging...'`
- `'messages.judgingDesc'`: `'Your code has been submitted and is being judged, please wait...'`

## 交互流程

```
用户点击提交 → API 请求 → 后端返回
                              ├─ 201 (同步): 直接显示评测结果
                              └─ 202 (异步):
                                  ├─ 前端显示 Pending 加载动画
                                  ├─ 提交按钮显示"评测中..."
                                  └─ WebSocket 收到 judge_result
                                      ├─ 更新 store 中的提交状态
                                      ├─ 更新 ProblemDetail 的 lastSubmission
                                      ├─ 显示最终评测结果
                                      └─ 恢复提交按钮
```

## 兼容性说明

- **向后兼容**：如果后端没有启用 Redis 队列（返回 201），前端行为与之前完全一致
- **降级处理**：WebSocket 未连接时，用户仍可通过刷新页面或切换 tab 获取最新提交状态
- **类型安全**：所有类型定义（`api.ts`、`types.ts`）都已同步更新
