-- 006_add_submission_tags.sql
-- 为提交记录补充标签能力，供教师在 AI 查重复核后标注“作弊”等业务标签。

CREATE TABLE IF NOT EXISTS submission_tags (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    submission_id BIGINT NOT NULL,
    tag VARCHAR(50) NOT NULL,
    created_by BIGINT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_submission_tag (submission_id, tag),
    FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_submission_tags_submission_id ON submission_tags(submission_id);
CREATE INDEX idx_submission_tags_created_by ON submission_tags(created_by);
