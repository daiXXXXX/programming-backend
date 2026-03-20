# AI代码查重实现说明

## 1. 目标

本次改动为 `programming-backend` 增加了一个教师/管理员可调用的 AI 代码查重能力，用来分析同一个班级、同一道题目下，不同学生提交代码之间是否存在明显抄袭或过度相似的情况。

实现目标有两个：

1. 满足“调用第三方 API”这一要求，实际接入 OpenAI Responses API。
2. 控制查重成本和误报率，不直接把全班代码两两全部送给大模型，而是先本地筛选，再让 AI 做最终判断。

## 2. 整体思路

查重流程采用“两阶段”方案：

1. 本地启发式预筛选
2. OpenAI 结构化判定

这样做的原因是：

- 如果一个班有 40 个学生，同一题目的两两组合就是 780 对，全部走 AI 成本高、速度慢。
- 直接让 AI 看所有代码，也更容易因为“大家都写的是同一道题”而产生误报。
- 先做结构化预筛，可以把真正值得怀疑的 pair 缩到很小，再让 AI 聚焦分析。

## 3. 查重数据来源

查重数据来自班级和提交记录：

- `class_students`：班级和学生关系
- `submissions`：学生提交记录
- `users`：用户名和头像等展示信息

新增的数据访问逻辑在：

- `internal/database/class.go`

核心方法：

- `GetRepresentativeProblemSubmissions(classID, problemID, acceptedOnly)`

这个方法会为每个学生挑出一份“代表性提交”：

- 如果 `acceptedOnly=true`，只取该学生这道题最近一次 `Accepted` 提交。
- 如果 `acceptedOnly=false`，优先取最近一次 `Accepted` 提交；如果没有通过记录，则退回到最近一次普通提交。

这样做的原因：

- 通过代码更能代表学生最终交付版本。
- 如果学生还没 AC，也不能完全忽略，所以允许回退到最近一次提交。

## 4. 本地预筛选怎么做

本地预筛选实现位于：

- `internal/plagiarism/service.go`

### 4.1 代码标准化

在比较前，先对代码做标准化处理：

- 统一换行
- 替换字符串字面量为占位符
- 替换数字字面量为占位符
- 去掉块注释和行注释
- 转为小写
- 将普通变量名统一折叠为 `id`
- 保留关键字、运算符、结构符号

这样可以降低以下干扰：

- 单纯改变量名
- 单纯调格式
- 单纯改字符串和数字常量

### 4.2 相似度计算

本地启发式分数由三部分组成：

- token shingle Jaccard
- 规范化后代码行集合的 Jaccard
- token 长度平衡度

最终组合权重：

- 0.65 * token 相似度
- 0.25 * 行结构相似度
- 0.10 * 长度相近程度

默认阈值：

- `minHeuristicScore = 0.55`

默认最多送 AI 的候选对数：

- `maxCandidates = 5`

如果没有 pair 达到阈值，但最高分已经明显偏高，也会保底选出 1 对做 AI 复核，避免“差一点但其实很可疑”的情况漏掉。

### 4.3 为什么不直接用字符串相等

因为学生如果只是：

- 改变量名
- 改空格和换行
- 改少量字面量

原始字符串就会完全不同，但程序结构可能几乎一样。标准化和 shingle 比较就是为了对抗这种“表面改写”。

## 5. OpenAI API 怎么接入

OpenAI 接入代码位于：

- `internal/ai/openai.go`

### 5.1 使用的接口

使用的是 OpenAI `Responses API`。

请求特点：

- 通过 `input` 发送系统提示词和用户载荷
- 通过 `text.format = json_schema` 强制模型按结构化 JSON 返回
- `store=false`，避免默认存储这次推理结果

### 5.2 为什么用结构化输出

因为查重结果不能只返回一大段自然语言，否则后端很难稳定消费。这里要求模型必须返回：

- `pairKey`
- `verdict`
- `riskLevel`
- `confidence`
- `summary`
- `evidence`
- `differences`

这样前端或后续管理页面都可以直接消费，不需要再做脆弱的字符串解析。

### 5.3 模型提示词原则

系统提示词明确强调：

- 同一道题使用相同算法是正常的
- 不能仅凭“都用了标准解法”就判抄袭
- 应重点关注结构、辅助函数拆分、冗余步骤、罕见写法、相同 bug、同样的注释思路等证据
- 要保守，降低误报

这一点非常重要，因为“同题同算法”天然会带来相似性，如果提示词不约束，大模型很容易过判。

## 6. 暴露出来的接口

新增接口：

- `POST /api/manager/classes/:id/plagiarism-check`

路由注册位置：

- `cmd/server/main.go`

处理器位置：

- `internal/handlers/manager.go`

### 6.1 权限控制

这个接口复用了班级访问权限校验：

- 管理员可以查任意班级
- 教师只能查自己名下班级

### 6.2 请求体

```json
{
  "problemId": 12,
  "acceptedOnly": false,
  "maxCandidates": 5,
  "minHeuristicScore": 0.55
}
```

字段说明：

- `problemId`：必填，要查重的题目
- `acceptedOnly`：是否只比较通过提交
- `maxCandidates`：最多送 AI 复核多少对候选 pair
- `minHeuristicScore`：本地预筛阈值，范围建议在 `0~1`

### 6.3 返回结果

返回内容包括：

- 班级 ID
- 题目 ID / 题目标题
- 实际比较了多少名学生
- 进入 AI 复核的候选 pair 数
- AI 总结
- 每一对可疑 pair 的详细结论

每条 pair 结果包含：

- 两个学生信息
- 对应提交 ID、语言、状态、时间、选择来源
- 本地启发式分数
- AI 置信度
- 风险等级
- 判定结论
- 可疑证据
- 区分点

## 7. 新增配置项

配置文件：

- `.env.example`
- `internal/config/config.go`

新增环境变量：

- `OPENAI_API_KEY`
- `OPENAI_BASE_URL`
- `OPENAI_MODEL`
- `OPENAI_REASONING_EFFORT`
- `OPENAI_REQUEST_TIMEOUT_MS`

默认模型：

- `gpt-5-mini`

说明：

- `OPENAI_API_KEY` 不配置时，接口会返回 `503`
- `OPENAI_REASONING_EFFORT` 默认留空，让模型走默认行为
- `OPENAI_BASE_URL` 预留是为了后续兼容代理或网关

## 8. 关键文件清单

本次与 AI 查重直接相关的文件如下：

- `cmd/server/main.go`
- `.env.example`
- `internal/config/config.go`
- `internal/database/class.go`
- `internal/handlers/manager.go`
- `internal/models/plagiarism.go`
- `internal/ai/openai.go`
- `internal/plagiarism/service.go`
- `internal/plagiarism/service_test.go`

## 9. 为什么这样设计

### 9.1 为什么挂到 manager 路由下

因为查重是教师/管理员使用的后台能力，不属于学生侧公共 API。

### 9.2 为什么不落库

当前版本选择“按需分析，实时返回”，不新建数据库表，原因是：

- 先验证流程是否顺手
- 降低迁移成本
- 避免一开始就引入结果缓存、历史记录、失效策略等复杂度

后续如果需要“保存查重报告”，再补表结构会更合适。

### 9.3 为什么先筛后判

这是整个实现里最关键的取舍：

- 纯启发式：便宜，但不够灵活
- 纯大模型：灵活，但成本高、延迟大
- 混合方案：成本、效果、工程复杂度之间更平衡

## 10. 当前限制

当前版本也有一些明确限制：

1. 默认只比较同语言提交，跨语言抄袭暂不重点处理。
2. AI 查重依赖外部网络和 OpenAI Key，本地没有 Key 时无法真正调用。
3. 目前是同步接口，请求时间会受到候选 pair 数量和 API 延迟影响。
4. 结果是“辅助判定”，不能直接替代人工定责。

## 11. 建议的后续增强

如果后面继续扩展，建议按这个顺序做：

1. 把查重报告落库，支持历史查询
2. 增加前端页面，展示高风险 pair
3. 加入跨语言比较策略
4. 引入更细粒度的 AST/语法树特征
5. 为 AI 查重增加异步任务队列，避免长请求阻塞

## 12. 本次改动原因总结

本次改动的核心原因是：你希望在毕业设计后端中加入“同班同学代码 AI 查重”能力，并要求调用第三方 API，例如 OpenAI API。

因此本次实现不是单纯加一个文档，而是把以下闭环补齐了：

- 班级维度的数据抽取
- 代码预筛逻辑
- OpenAI 调用
- 教师接口
- 配置项
- 单元测试
- 说明文档

这样后面无论你是继续接前端页面、做演示，还是写毕业设计论文里的“系统实现”章节，都已经有一套比较完整的后端基础。
