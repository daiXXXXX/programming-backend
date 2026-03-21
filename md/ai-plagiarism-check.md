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

当前本地启发式分数只保留一条主规则：

- 先把标准化后的代码切成 token 序列
- 再按固定窗口生成 token shingle
- 最后计算两份代码 shingle 集合的 Jaccard 相似度

这个分数直接作为 `heuristicScore`，不再额外叠加多组权重。

候选 pair 的筛选规则也同步简化为：

- 分数达到 `minHeuristicScore` 才进入候选集合
- 候选集合按分数从高到低排序
- 最多保留 `maxCandidates` 对送给 AI 复核
- 如果没有 pair 达到阈值，就直接结束，不再做“保底送 1 对”的兜底逻辑

默认阈值：

- `minHeuristicScore = 0.55`

默认最多送 AI 的候选对数：

- `maxCandidates = 5`

这样做的好处是，本地筛选的行为更稳定、更容易解释：它只负责“粗筛可疑 pair”，最终是否判定为抄袭，仍然交给后续的 AI 结构化分析。

### 4.3 按方法看本地筛选流程

如果按 `internal/plagiarism/service.go` 里的实际执行顺序来看，本地筛选大致分为下面几步：

1. `CheckClassProblem(...)`
2. `resolveMaxCandidates(...)` 和 `resolveMinHeuristic(...)`
3. `buildCandidatePairs(...)`
4. `heuristicSimilarity(...)`
5. `normalizeSourceCode(...)`
6. `normalizeTokens(...)`
7. `buildShingles(...)`
8. `shingleJaccard(...)`
9. `sortCandidates(...)`

可以把这条链路理解为：

- 入口方法先做基础校验
- 再把请求参数收敛到安全范围
- 然后枚举所有学生 pair
- 对每一对代码做标准化
- 把标准化结果切成 token
- 生成 shingle 集合
- 计算 Jaccard 相似度
- 用阈值过滤
- 按分数排序后只保留前 N 对

下面按方法分别说明。

#### 4.3.1 `CheckClassProblem(...)`

这是查重服务的入口方法，也是“本地筛选”真正开始的地方。

它先做三件事：

- 创建返回用的 `report`
- 检查题目信息是否存在
- 检查代表性提交数量是否至少为 2

只有这些前置条件满足后，它才会调用：

- `resolveMaxCandidates(req.MaxCandidates)`
- `resolveMinHeuristic(req.MinHeuristicScore)`
- `buildCandidatePairs(...)`

也就是说，本地筛选并不是单独暴露出去的方法，而是由 `CheckClassProblem(...)` 在正式调用 AI 之前触发的一步“预筛”。

如果 `buildCandidatePairs(...)` 返回空集合，`CheckClassProblem(...)` 会直接返回，不再调用 OpenAI。

#### 4.3.2 `resolveMaxCandidates(...)`

这个方法负责清洗“最多送 AI 复核多少对”的参数。

规则很简单：

- 如果传入值小于等于 0，使用默认值 `5`
- 如果传入值大于上限，截断为 `10`
- 否则使用用户传入值

它的作用不是算相似度，而是防止请求参数过大，导致一次查重送太多 pair 给 AI。

#### 4.3.3 `resolveMinHeuristic(...)`

这个方法负责清洗“本地筛选阈值”。

规则是：

- 如果传入值小于等于 `0` 或大于等于 `1`，回退到默认值 `0.55`
- 否则直接使用传入值

它保证阈值总是在合理区间内，避免出现负数、`1` 以上之类无效配置。

#### 4.3.4 `buildCandidatePairs(...)`

这个方法是本地筛选的核心调度器。

它会双重循环遍历所有学生提交，两两组成 pair。对每一对 pair，它会依次做这些事情：

- 调用 `orderedPair(...)`，把用户 ID 小的放左边，保证同一对学生永远生成同样的顺序
- 调用 `comparableLanguages(...)`，只允许语言一致，或者其中一方语言为空时继续比较
- 调用 `heuristicSimilarity(...)`，计算这两份代码的本地相似度
- 调用 `makePairKey(...)` 生成形如 `1:2` 的唯一标识
- 只有当分数 `>= minHeuristicScore` 时，才把这对 pair 放进候选集合

所有 pair 处理完之后，它还会：

- 调用 `sortCandidates(...)` 按分数从高到低排序
- 只保留前 `maxCandidates` 对

所以 `buildCandidatePairs(...)` 的职责很明确：它不做 AI 判定，只负责从“全量 pair”里筛出“值得送 AI 的可疑 pair”。

#### 4.3.5 `orderedPair(...)`

这个方法的作用是固定 pair 的左右顺序。

例如学生 2 和学生 5 这对，不管外层循环先拿到谁，最后都会被整理成：

- 左边：用户 ID 较小的那位
- 右边：用户 ID 较大的那位

这样做有两个好处：

- `pairKey` 稳定，不会一会儿是 `2:5` 一会儿又是 `5:2`
- 排序、去重、结果展示都更一致

#### 4.3.6 `comparableLanguages(...)`

这个方法控制“哪些代码允许进入本地比较”。

当前规则很保守：

- 两边语言相同，可以比较
- 任意一边语言为空，也允许比较
- 明确是两种不同语言时，直接跳过

所以当前版本主要还是处理“同语言代码是否高度相似”，并没有重点做跨语言抄袭识别。

#### 4.3.7 `heuristicSimilarity(...)`

这个方法真正产出本地分数。

它内部只做三件事：

- 调用 `normalizeSourceCode(leftCode)`
- 调用 `normalizeSourceCode(rightCode)`
- 调用 `shingleJaccard(leftTokens, rightTokens, shingleSize)`

如果任意一边标准化之后 token 为空，就直接返回 `0`。

否则，它把 `shingleJaccard(...)` 算出来的结果再交给 `roundSimilarity(...)` 做三位小数保留，最终得到 `heuristicScore`。

所以现在的本地分数非常直接，本质上就是：

`标准化后的 token shingle Jaccard 相似度`

#### 4.3.8 `normalizeSourceCode(...)`

这个方法负责把原始代码清洗成“便于比较的 token 流”。

它的处理顺序是：

- 统一换行符，把 `\r\n` 和 `\r` 统一成 `\n`
- 用 `STR` 替换双引号、单引号、反引号字符串
- 删掉块注释、行注释、`#` 注释
- 用 `NUM` 替换数字字面量
- 把整份代码转成小写
- 用正则提取 token
- 把 token 交给 `normalizeTokens(...)` 继续处理

这一步的目标是去掉“表面差异”，保留真正反映程序结构的部分。

#### 4.3.9 `normalizeTokens(...)`

这个方法负责进一步压缩 token 差异。

它会逐个处理 token：

- 去空格
- 转小写
- 空 token 直接跳过
- 如果 token 是标识符，并且不是关键字，就统一替换成 `id`

例如：

- `cache` 会变成 `id`
- `answerMap` 会变成 `id`
- `for`、`if`、`return` 这种关键字会保留

这一步非常关键，因为它直接削弱了“只改变量名”的干扰。

#### 4.3.10 `buildShingles(...)`

这个方法把 token 序列切成固定长度的片段集合。

当前常量 `shingleSize = 5`，也就是说它会把 token 流按长度为 5 的滑动窗口切片，例如：

- `t1 t2 t3 t4 t5`
- `t2 t3 t4 t5 t6`
- `t3 t4 t5 t6 t7`

如果 token 总数不足 5，就把整段 token 当成一个 shingle。

为什么要这样做？

- 单个 token 太粗糙，区分度不够
- 连续 token 片段更能体现代码结构和局部写法

#### 4.3.11 `shingleJaccard(...)`

这个方法用来计算两份代码的相似度分数。

它先把左右两边 token 流都交给 `buildShingles(...)`，得到两个 shingle 集合，然后计算：

- 交集大小
- 并集大小
- `交集 / 并集`

这就是 Jaccard 相似度。

含义可以直观理解为：

- 两边共有的 shingle 越多，分数越高
- 两边独有的 shingle 越多，分数越低

如果两个集合之一为空，就直接返回 `0`。

#### 4.3.12 `sortCandidates(...)`

这个方法只做候选 pair 排序，不负责筛选。

排序规则是：

- 先按 `HeuristicScore` 从高到低
- 如果分数一样，再按 `PairKey` 字典序

这样排序后，`buildCandidatePairs(...)` 就可以很直接地截取前 `maxCandidates` 个 pair。

#### 4.3.13 `makePairKey(...)`、`roundSimilarity(...)`、`clamp01(...)`

这几个方法属于辅助方法：

- `makePairKey(...)`：生成稳定的 pair 唯一标识
- `roundSimilarity(...)`：把分数保留到 3 位小数
- `clamp01(...)`：确保分数始终落在 `0~1`

它们不决定“谁可疑”，但保证结果格式稳定、返回值可控。

#### 4.3.14 本地筛选结束后做什么

本地筛选结束后，如果确实筛出候选 pair，`CheckClassProblem(...)` 才会继续调用 `buildAnalysisRequest(...)`，把这些 pair 打包后送给 OpenAI。

所以整个职责边界是：

- 本地筛选：回答“哪些 pair 值得进一步怀疑”
- AI 分析：回答“这些可疑 pair 到底像不像抄袭”

### 4.4 为什么不直接用字符串相等

因为学生如果只是：

- 改变量名
- 改空格和换行
- 改少量字面量

原始字符串就会完全不同，但程序结构可能依然高度接近。当前版本保留“标准化 + token shingle 相似度”这一条最核心的规则，就是为了在逻辑尽量简单的前提下，继续对抗这种“表面改写”。

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
