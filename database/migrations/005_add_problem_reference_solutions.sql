-- 005_add_problem_reference_solutions.sql
-- 为题目录入流程补充标准程序留档表，便于校验测试数据和后续维护。

CREATE TABLE IF NOT EXISTS problem_reference_solutions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    problem_id BIGINT NOT NULL,
    language VARCHAR(50) NOT NULL,
    source_code MEDIUMTEXT NOT NULL,
    created_by BIGINT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_problem_reference_solution (problem_id),
    FOREIGN KEY (problem_id) REFERENCES problems(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_problem_reference_solutions_created_by ON problem_reference_solutions(created_by);
