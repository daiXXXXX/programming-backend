# 班级与实验表结构补充

## 改动原因

后端代码 `internal/database/class.go` 和 `internal/handlers/manager.go` 中已实现了完整的班级管理逻辑（教师/管理员按班级管理学生、关联实验、查看学生进度等），但数据库中缺少对应的表结构，导致相关功能无法正常运行。本次改动补齐了所有缺失的数据库表。

## 改动文件清单

| 文件                                                      | 操作 | 说明                       |
| --------------------------------------------------------- | ---- | -------------------------- |
| `database/migrations/004_add_classes_and_experiments.sql` | 新增 | 班级与实验相关表的迁移脚本 |

## 新增表结构

### 1. `classes` — 班级表

| 字段        | 类型                 | 说明     |
| ----------- | -------------------- | -------- |
| id          | BIGINT PK            | 主键     |
| name        | VARCHAR(100)         | 班级名称 |
| description | TEXT                 | 班级描述 |
| teacher_id  | BIGINT FK → users.id | 所属教师 |
| created_at  | TIMESTAMP            | 创建时间 |
| updated_at  | TIMESTAMP            | 更新时间 |

### 2. `class_students` — 班级-学生关联表

| 字段       | 类型                   | 说明     |
| ---------- | ---------------------- | -------- |
| id         | BIGINT PK              | 主键     |
| class_id   | BIGINT FK → classes.id | 班级 ID  |
| student_id | BIGINT FK → users.id   | 学生 ID  |
| joined_at  | TIMESTAMP              | 加入时间 |

唯一约束：`(class_id, student_id)`，确保同一学生不会重复加入同一班级。

### 3. `experiments` — 实验表

| 字段        | 类型                 | 说明     |
| ----------- | -------------------- | -------- |
| id          | BIGINT PK            | 主键     |
| title       | VARCHAR(200)         | 实验标题 |
| description | TEXT                 | 实验描述 |
| start_time  | TIMESTAMP            | 开始时间 |
| end_time    | TIMESTAMP            | 结束时间 |
| is_active   | TINYINT(1)           | 是否启用 |
| created_by  | BIGINT FK → users.id | 创建者   |
| created_at  | TIMESTAMP            | 创建时间 |
| updated_at  | TIMESTAMP            | 更新时间 |

### 4. `class_experiments` — 班级-实验关联表

| 字段          | 类型                       | 说明     |
| ------------- | -------------------------- | -------- |
| id            | BIGINT PK                  | 主键     |
| class_id      | BIGINT FK → classes.id     | 班级 ID  |
| experiment_id | BIGINT FK → experiments.id | 实验 ID  |
| added_at      | TIMESTAMP                  | 关联时间 |

唯一约束：`(class_id, experiment_id)`。

### 5. `experiment_problems` — 实验-题目关联表

| 字段          | 类型                       | 说明     |
| ------------- | -------------------------- | -------- |
| id            | BIGINT PK                  | 主键     |
| experiment_id | BIGINT FK → experiments.id | 实验 ID  |
| problem_id    | BIGINT FK → problems.id    | 题目 ID  |
| display_order | INT                        | 显示顺序 |
| added_at      | TIMESTAMP                  | 关联时间 |

唯一约束：`(experiment_id, problem_id)`。

## 新增索引

- `idx_classes_teacher_id` — 加速按教师查询班级
- `idx_class_students_class_id` / `idx_class_students_student_id` — 加速班级学生查询
- `idx_class_experiments_class_id` / `idx_class_experiments_experiment_id` — 加速班级实验查询
- `idx_experiment_problems_experiment_id` / `idx_experiment_problems_problem_id` — 加速实验题目查询
- `idx_experiments_is_active` — 加速按启用状态筛选实验

## 关联的后端代码

- `internal/database/class.go` — ClassRepository，提供班级列表、详情、学生进度等查询
- `internal/handlers/manager.go` — ManagerHandler，提供 `/api/manager/classes` 等接口
