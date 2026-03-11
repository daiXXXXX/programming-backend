-- 每日刷题活动表（用于热力图/绿墙展示）
-- 记录每个用户每天的提交数和解题数
CREATE TABLE IF NOT EXISTS daily_activity (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    activity_date DATE NOT NULL,
    submission_count INT NOT NULL DEFAULT 0,   -- 当天提交次数
    solved_count INT NOT NULL DEFAULT 0,       -- 当天新解决的题目数（去重）
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_user_date (user_id, activity_date),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 创建索引以加速按用户和日期范围查询
CREATE INDEX idx_daily_activity_user_date ON daily_activity(user_id, activity_date);

-- 为已有数据回填 daily_activity（从 submissions 表聚合）
INSERT IGNORE INTO daily_activity (user_id, activity_date, submission_count, solved_count)
SELECT 
    s.user_id,
    DATE(s.submitted_at) as activity_date,
    COUNT(*) as submission_count,
    COUNT(DISTINCT CASE WHEN s.status = 'Accepted' THEN s.problem_id END) as solved_count
FROM submissions s
GROUP BY s.user_id, DATE(s.submitted_at);
