-- Programming Online Judge Database Schema

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'student', -- student, instructor, admin
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 题目表
CREATE TABLE IF NOT EXISTS problems (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    difficulty VARCHAR(20) NOT NULL, -- Easy, Medium, Hard
    description TEXT NOT NULL,
    input_format TEXT NOT NULL,
    output_format TEXT NOT NULL,
    constraints TEXT NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 题目示例
CREATE TABLE IF NOT EXISTS problem_examples (
    id BIGSERIAL PRIMARY KEY,
    problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    input TEXT NOT NULL,
    output TEXT NOT NULL,
    explanation TEXT,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 测试用例
CREATE TABLE IF NOT EXISTS test_cases (
    id BIGSERIAL PRIMARY KEY,
    problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    input TEXT NOT NULL,
    expected_output TEXT NOT NULL,
    description TEXT,
    is_sample BOOLEAN NOT NULL DEFAULT false, -- 是否为示例测试用例
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 题目标签
CREATE TABLE IF NOT EXISTS problem_tags (
    id BIGSERIAL PRIMARY KEY,
    problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    tag VARCHAR(50) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(problem_id, tag)
);

-- 提交记录
CREATE TABLE IF NOT EXISTS submissions (
    id BIGSERIAL PRIMARY KEY,
    problem_id BIGINT NOT NULL REFERENCES problems(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    code TEXT NOT NULL,
    language VARCHAR(50) NOT NULL DEFAULT 'JavaScript',
    status VARCHAR(50) NOT NULL, -- Accepted, Wrong Answer, Runtime Error, Time Limit Exceeded
    score INT NOT NULL DEFAULT 0,
    submitted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 测试结果
CREATE TABLE IF NOT EXISTS test_results (
    id BIGSERIAL PRIMARY KEY,
    submission_id BIGINT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id),
    passed BOOLEAN NOT NULL,
    actual_output TEXT,
    error_message TEXT,
    execution_time INT, -- 毫秒
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 用户解题统计（缓存表，用于提升性能）
CREATE TABLE IF NOT EXISTS user_stats (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    total_solved INT NOT NULL DEFAULT 0,
    easy_solved INT NOT NULL DEFAULT 0,
    medium_solved INT NOT NULL DEFAULT 0,
    hard_solved INT NOT NULL DEFAULT 0,
    total_submissions INT NOT NULL DEFAULT 0,
    accepted_submissions INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX idx_problems_difficulty ON problems(difficulty);
CREATE INDEX idx_problems_created_at ON problems(created_at);
CREATE INDEX idx_submissions_user_id ON submissions(user_id);
CREATE INDEX idx_submissions_problem_id ON submissions(problem_id);
CREATE INDEX idx_submissions_status ON submissions(status);
CREATE INDEX idx_submissions_submitted_at ON submissions(submitted_at);
CREATE INDEX idx_test_results_submission_id ON test_results(submission_id);
CREATE INDEX idx_problem_tags_problem_id ON problem_tags(problem_id);
CREATE INDEX idx_problem_examples_problem_id ON problem_examples(problem_id);
CREATE INDEX idx_test_cases_problem_id ON test_cases(problem_id);

-- 插入初始数据
INSERT INTO users (username, email, password_hash, role) VALUES 
('demo_student', 'student@example.com', '$2a$10$dummyhash', 'student'),
('demo_instructor', 'instructor@example.com', '$2a$10$dummyhash', 'instructor')
ON CONFLICT (username) DO NOTHING;
