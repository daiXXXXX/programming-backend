package database

import (
	"database/sql"
	"fmt"
	"time"

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
	query := `
		INSERT INTO submissions (problem_id, user_id, code, language, status, score)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := tx.Exec(
		query, submission.ProblemID, submission.UserID, submission.Code,
		submission.Language, submission.Status, submission.Score,
	)
	if err != nil {
		return 0, err
	}
	submissionID, err := result.LastInsertId()
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
				 VALUES (?, ?, ?, ?, ?, ?)`,
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

	// 更新每日活动统计
	if err = r.updateDailyActivity(tx, submission.UserID); err != nil {
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
		WHERE id = ?
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
		WHERE user_id = ?
		ORDER BY submitted_at DESC
		LIMIT ? OFFSET ?
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
		WHERE problem_id = ?
		ORDER BY submitted_at DESC
		LIMIT ? OFFSET ?
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
		WHERE tr.submission_id = ?
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
	query := `
		INSERT INTO user_stats (user_id, total_solved, easy_solved, medium_solved, hard_solved, 
		                        total_submissions, accepted_submissions, today_solved, today_date)
		SELECT 
			?,
			(SELECT COUNT(DISTINCT s.problem_id) FROM submissions s WHERE s.user_id = ? AND s.status = 'Accepted'),
			(SELECT COUNT(DISTINCT s.problem_id) FROM submissions s JOIN problems p ON s.problem_id = p.id WHERE s.user_id = ? AND s.status = 'Accepted' AND p.difficulty = 'Easy'),
			(SELECT COUNT(DISTINCT s.problem_id) FROM submissions s JOIN problems p ON s.problem_id = p.id WHERE s.user_id = ? AND s.status = 'Accepted' AND p.difficulty = 'Medium'),
			(SELECT COUNT(DISTINCT s.problem_id) FROM submissions s JOIN problems p ON s.problem_id = p.id WHERE s.user_id = ? AND s.status = 'Accepted' AND p.difficulty = 'Hard'),
			(SELECT COUNT(*) FROM submissions WHERE user_id = ?),
			(SELECT COUNT(*) FROM submissions WHERE user_id = ? AND status = 'Accepted'),
			(SELECT COUNT(DISTINCT s.problem_id) FROM submissions s WHERE s.user_id = ? AND s.status = 'Accepted' AND DATE(s.submitted_at) = CURDATE()),
			CURDATE()
		ON DUPLICATE KEY UPDATE
			total_solved = VALUES(total_solved),
			easy_solved = VALUES(easy_solved),
			medium_solved = VALUES(medium_solved),
			hard_solved = VALUES(hard_solved),
			total_submissions = VALUES(total_submissions),
			accepted_submissions = VALUES(accepted_submissions),
			today_solved = VALUES(today_solved),
			today_date = CURDATE(),
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := tx.Exec(query, userID, userID, userID, userID, userID, userID, userID, userID)
	return err
}

// GetUserStats 获取用户统计信息
func (r *SubmissionRepository) GetUserStats(userID int64) (*models.UserStats, error) {
	query := `
		SELECT user_id, total_solved, easy_solved, medium_solved, hard_solved,
		       total_submissions, accepted_submissions, updated_at
		FROM user_stats
		WHERE user_id = ?
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
				UserID:      userID,
				SuccessRate: 0,
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

// updateDailyActivity 更新每日活动统计
func (r *SubmissionRepository) updateDailyActivity(tx *sql.Tx, userID int64) error {
	query := `
		INSERT INTO daily_activity (user_id, activity_date, submission_count, solved_count)
		SELECT 
			?,
			CURDATE(),
			(SELECT COUNT(*) FROM submissions WHERE user_id = ? AND DATE(submitted_at) = CURDATE()),
			(SELECT COUNT(DISTINCT problem_id) FROM submissions WHERE user_id = ? AND status = 'Accepted' AND DATE(submitted_at) = CURDATE())
		ON DUPLICATE KEY UPDATE
			submission_count = VALUES(submission_count),
			solved_count = VALUES(solved_count),
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := tx.Exec(query, userID, userID, userID)
	return err
}

// GetDailyActivity 获取用户指定日期范围内的每日活动数据
func (r *SubmissionRepository) GetDailyActivity(userID int64, startDate, endDate string) ([]models.DailyActivity, error) {
	query := `
		SELECT user_id, activity_date, submission_count, solved_count
		FROM daily_activity
		WHERE user_id = ? AND activity_date >= ? AND activity_date <= ?
		ORDER BY activity_date ASC
	`

	rows, err := r.db.Query(query, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []models.DailyActivity
	for rows.Next() {
		var a models.DailyActivity
		var dateVal time.Time
		err := rows.Scan(&a.UserID, &dateVal, &a.SubmissionCount, &a.SolvedCount)
		if err != nil {
			return nil, err
		}
		a.ActivityDate = dateVal.Format("2006-01-02")
		activities = append(activities, a)
	}

	return activities, rows.Err()
}

// CreatePending 创建一条 Pending 状态的提交记录（不含测试结果）
func (r *SubmissionRepository) CreatePending(submission *models.Submission) (int64, error) {
	query := `
		INSERT INTO submissions (problem_id, user_id, code, language, status, score)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(
		query, submission.ProblemID, submission.UserID, submission.Code,
		submission.Language, models.StatusPending, 0,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateResult 评测完成后更新提交结果（状态、分数、测试结果、统计）
func (r *SubmissionRepository) UpdateResult(submissionID int64, status models.SubmissionStatus, score int, testResults []models.TestResult) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 更新提交状态和分数
	_, err = tx.Exec(
		`UPDATE submissions SET status = ?, score = ? WHERE id = ?`,
		status, score, submissionID,
	)
	if err != nil {
		return err
	}

	// 2. 插入测试结果
	for _, result := range testResults {
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
			 VALUES (?, ?, ?, ?, ?, ?)`,
			submissionID, result.TestCaseID, result.Passed, actualOutput, errorMsg, result.ExecutionTime,
		)
		if err != nil {
			return err
		}
	}

	// 3. 获取 user_id 以更新统计
	var userID int64
	err = tx.QueryRow(`SELECT user_id FROM submissions WHERE id = ?`, submissionID).Scan(&userID)
	if err != nil {
		return err
	}

	// 4. 更新用户统计
	if err = r.updateUserStats(tx, userID); err != nil {
		return err
	}

	// 5. 更新每日活动统计
	if err = r.updateDailyActivity(tx, userID); err != nil {
		return err
	}

	return tx.Commit()
}
