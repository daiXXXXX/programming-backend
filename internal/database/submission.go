package database

import (
	"database/sql"
	"fmt"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// SubmissionRepository 提交数据访问层
type SubmissionRepository struct {
	db *DB
}

func NewSubmissionRepository(db *DB) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

// Create 创建提交记录
func (r *SubmissionRepository) Create(submission *models.Submission) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 插入提交记录
	var submissionID int64
	query := `
		INSERT INTO submissions (problem_id, user_id, code, language, status, score)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	err = tx.QueryRow(
		query, submission.ProblemID, submission.UserID, submission.Code,
		submission.Language, submission.Status, submission.Score,
	).Scan(&submissionID)
	if err != nil {
		return 0, err
	}

	// 插入测试结果
	if len(submission.TestResults) > 0 {
		for _, result := range submission.TestResults {
			var errorMsg sql.NullString
			if result.Error != "" {
				errorMsg = sql.NullString{String: result.Error, Valid: true}
			}

			var actualOutput sql.NullString
			if result.ActualOutput != "" {
				actualOutput = sql.NullString{String: result.ActualOutput, Valid: true}
			}

			_, err = tx.Exec(
				`INSERT INTO test_results (submission_id, test_case_id, passed, actual_output, error_message, execution_time)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				submissionID, result.TestCaseID, result.Passed, actualOutput, errorMsg, result.ExecutionTime,
			)
			if err != nil {
				return 0, err
			}
		}
	}

	// 更新用户统计
	if err = r.updateUserStats(tx, submission.UserID); err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return submissionID, nil
}

// GetByID 根据ID获取提交详情
func (r *SubmissionRepository) GetByID(id int64) (*models.Submission, error) {
	query := `
		SELECT id, problem_id, user_id, code, language, status, score, submitted_at
		FROM submissions
		WHERE id = $1
	`

	var s models.Submission
	err := r.db.QueryRow(query, id).Scan(
		&s.ID, &s.ProblemID, &s.UserID, &s.Code, &s.Language,
		&s.Status, &s.Score, &s.SubmittedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("submission not found")
		}
		return nil, err
	}

	// 加载测试结果
	s.TestResults, err = r.getTestResults(id)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// GetByUserID 获取用户的所有提交
func (r *SubmissionRepository) GetByUserID(userID int64, limit, offset int) ([]models.Submission, error) {
	query := `
		SELECT id, problem_id, user_id, code, language, status, score, submitted_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY submitted_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var submissions []models.Submission
	for rows.Next() {
		var s models.Submission
		err := rows.Scan(
			&s.ID, &s.ProblemID, &s.UserID, &s.Code, &s.Language,
			&s.Status, &s.Score, &s.SubmittedAt,
		)
		if err != nil {
			return nil, err
		}

		// 加载测试结果
		s.TestResults, _ = r.getTestResults(s.ID)
		submissions = append(submissions, s)
	}

	return submissions, rows.Err()
}

// GetByProblemID 获取题目的所有提交
func (r *SubmissionRepository) GetByProblemID(problemID int64, limit, offset int) ([]models.Submission, error) {
	query := `
		SELECT id, problem_id, user_id, code, language, status, score, submitted_at
		FROM submissions
		WHERE problem_id = $1
		ORDER BY submitted_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(query, problemID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var submissions []models.Submission
	for rows.Next() {
		var s models.Submission
		err := rows.Scan(
			&s.ID, &s.ProblemID, &s.UserID, &s.Code, &s.Language,
			&s.Status, &s.Score, &s.SubmittedAt,
		)
		if err != nil {
			return nil, err
		}

		// 加载测试结果
		s.TestResults, _ = r.getTestResults(s.ID)
		submissions = append(submissions, s)
	}

	return submissions, rows.Err()
}

// getTestResults 获取提交的测试结果
func (r *SubmissionRepository) getTestResults(submissionID int64) ([]models.TestResult, error) {
	query := `
		SELECT tr.id, tr.test_case_id, tr.passed, tr.actual_output, 
		       tr.error_message, tr.execution_time,
		       tc.input, tc.expected_output
		FROM test_results tr
		JOIN test_cases tc ON tr.test_case_id = tc.id
		WHERE tr.submission_id = $1
		ORDER BY tr.id
	`

	rows, err := r.db.Query(query, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.TestResult
	for rows.Next() {
		var tr models.TestResult
		var actualOutput, errorMsg sql.NullString
		var executionTime sql.NullInt64

		err := rows.Scan(
			&tr.ID, &tr.TestCaseID, &tr.Passed, &actualOutput,
			&errorMsg, &executionTime, &tr.Input, &tr.ExpectedOutput,
		)
		if err != nil {
			return nil, err
		}

		if actualOutput.Valid {
			tr.ActualOutput = actualOutput.String
		}
		if errorMsg.Valid {
			tr.Error = errorMsg.String
		}
		if executionTime.Valid {
			tr.ExecutionTime = int(executionTime.Int64)
		}

		results = append(results, tr)
	}

	return results, rows.Err()
}

// updateUserStats 更新用户统计信息
func (r *SubmissionRepository) updateUserStats(tx *sql.Tx, userID int64) error {
	// 计算统计信息
	query := `
		WITH solved_problems AS (
			SELECT DISTINCT s.problem_id, p.difficulty
			FROM submissions s
			JOIN problems p ON s.problem_id = p.id
			WHERE s.user_id = $1 AND s.status = 'Accepted'
		),
		submission_stats AS (
			SELECT 
				COUNT(*) as total_submissions,
				COUNT(CASE WHEN status = 'Accepted' THEN 1 END) as accepted_submissions
			FROM submissions
			WHERE user_id = $1
		)
		INSERT INTO user_stats (user_id, total_solved, easy_solved, medium_solved, hard_solved, 
		                        total_submissions, accepted_submissions)
		SELECT 
			$1,
			COUNT(*) as total_solved,
			COUNT(CASE WHEN difficulty = 'Easy' THEN 1 END) as easy_solved,
			COUNT(CASE WHEN difficulty = 'Medium' THEN 1 END) as medium_solved,
			COUNT(CASE WHEN difficulty = 'Hard' THEN 1 END) as hard_solved,
			ss.total_submissions,
			ss.accepted_submissions
		FROM solved_problems sp
		CROSS JOIN submission_stats ss
		ON CONFLICT (user_id) 
		DO UPDATE SET
			total_solved = EXCLUDED.total_solved,
			easy_solved = EXCLUDED.easy_solved,
			medium_solved = EXCLUDED.medium_solved,
			hard_solved = EXCLUDED.hard_solved,
			total_submissions = EXCLUDED.total_submissions,
			accepted_submissions = EXCLUDED.accepted_submissions,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := tx.Exec(query, userID)
	return err
}

// GetUserStats 获取用户统计信息
func (r *SubmissionRepository) GetUserStats(userID int64) (*models.UserStats, error) {
	query := `
		SELECT user_id, total_solved, easy_solved, medium_solved, hard_solved,
		       total_submissions, accepted_submissions, updated_at
		FROM user_stats
		WHERE user_id = $1
	`

	var stats models.UserStats
	err := r.db.QueryRow(query, userID).Scan(
		&stats.UserID, &stats.TotalSolved, &stats.EasySolved, &stats.MediumSolved,
		&stats.HardSolved, &stats.TotalSubmissions, &stats.AcceptedSubmissions,
		&stats.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// 如果没有统计信息，返回默认值
			return &models.UserStats{
				UserID:       userID,
				SuccessRate:  0,
			}, nil
		}
		return nil, err
	}

	// 计算成功率
	if stats.TotalSubmissions > 0 {
		stats.SuccessRate = float64(stats.AcceptedSubmissions) / float64(stats.TotalSubmissions) * 100
	}

	return &stats, nil
}
