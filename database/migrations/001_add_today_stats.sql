-- 为 user_stats 表添加今日刷题统计字段
-- 修复排行榜接口 /api/ranking/total 和 /api/ranking/today 报错

ALTER TABLE user_stats
    ADD COLUMN today_solved INT NOT NULL DEFAULT 0 AFTER accepted_submissions,
    ADD COLUMN today_date DATE DEFAULT NULL AFTER today_solved;
