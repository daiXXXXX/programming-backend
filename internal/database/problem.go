package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// ProblemRepository 题目数据访问层
type ProblemRepository struct {
	db *DB
}

func NewProblemRepository(db *DB) *ProblemRepository {
	return &ProblemRepository{db: db}
}

// GetAll 获取所有题目（不包含测试用例详情）
func (r *ProblemRepository) GetAll() ([]models.Problem, error) {
	query := `
		SELECT id, title, difficulty, description, input_format, output_format, 
		       constraints, created_at, updated_at
		FROM problems
		ORDER BY id ASC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []models.Problem
	for rows.Next() {
		var p models.Problem
		err := rows.Scan(
			&p.ID, &p.Title, &p.Difficulty, &p.Description,
			&p.InputFormat, &p.OutputFormat, &p.Constraints,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		// 加载标签
		p.Tags, _ = r.GetTags(p.ID)
		// 加载示例
		p.Examples, _ = r.GetExamples(p.ID)
		// 加载测试用例（不包含完整细节）
		p.TestCases, _ = r.GetTestCasesMeta(p.ID)

		problems = append(problems, p)
	}

	return problems, rows.Err()
}

// GetByID 根据ID获取题目详情（包含所有测试用例）
func (r *ProblemRepository) GetByID(id int64) (*models.Problem, error) {
	query := `
		SELECT id, title, difficulty, description, input_format, output_format, 
		       constraints, created_at, updated_at
		FROM problems
		WHERE id = ?
	`

	var p models.Problem
	err := r.db.QueryRow(query, id).Scan(
		&p.ID, &p.Title, &p.Difficulty, &p.Description,
		&p.InputFormat, &p.OutputFormat, &p.Constraints,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("problem not found")
		}
		return nil, err
	}

	// 加载标签
	p.Tags, _ = r.GetTags(p.ID)
	// 加载示例
	p.Examples, _ = r.GetExamples(p.ID)
	// 加载完整测试用例
	p.TestCases, _ = r.GetTestCases(p.ID)

	return &p, nil
}

// GetTags 获取题目标签
func (r *ProblemRepository) GetTags(problemID int64) ([]string, error) {
	query := `SELECT tag FROM problem_tags WHERE problem_id = ? ORDER BY id`

	rows, err := r.db.Query(query, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, rows.Err()
}

// GetExamples 获取题目示例
func (r *ProblemRepository) GetExamples(problemID int64) ([]models.Example, error) {
	query := `
		SELECT id, input, output, explanation
		FROM problem_examples
		WHERE problem_id = ?
		ORDER BY display_order, id
	`

	rows, err := r.db.Query(query, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var examples []models.Example
	for rows.Next() {
		var ex models.Example
		var explanation sql.NullString
		if err := rows.Scan(&ex.ID, &ex.Input, &ex.Output, &explanation); err != nil {
			return nil, err
		}
		if explanation.Valid {
			ex.Explanation = explanation.String
		}
		examples = append(examples, ex)
	}

	return examples, rows.Err()
}

// GetTestCasesMeta 获取测试用例元信息（不包含完整输入输出）
func (r *ProblemRepository) GetTestCasesMeta(problemID int64) ([]models.TestCase, error) {
	query := `
		SELECT id, input, expected_output
		FROM test_cases
		WHERE problem_id = ? AND is_sample = true
		ORDER BY display_order, id
		LIMIT 3
	`

	rows, err := r.db.Query(query, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var testCases []models.TestCase
	for rows.Next() {
		var tc models.TestCase
		if err := rows.Scan(&tc.ID, &tc.Input, &tc.ExpectedOutput); err != nil {
			return nil, err
		}
		testCases = append(testCases, tc)
	}

	return testCases, rows.Err()
}

// GetTestCases 获取所有测试用例（用于评测）
func (r *ProblemRepository) GetTestCases(problemID int64) ([]models.TestCase, error) {
	query := `
		SELECT id, input, expected_output, description, is_sample
		FROM test_cases
		WHERE problem_id = ?
		ORDER BY display_order, id
	`

	rows, err := r.db.Query(query, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var testCases []models.TestCase
	for rows.Next() {
		var tc models.TestCase
		var description sql.NullString
		if err := rows.Scan(&tc.ID, &tc.Input, &tc.ExpectedOutput, &description, &tc.IsSample); err != nil {
			return nil, err
		}
		if description.Valid {
			tc.Description = description.String
		}
		testCases = append(testCases, tc)
	}

	return testCases, rows.Err()
}

// Create 创建新题目
func (r *ProblemRepository) Create(req *models.CreateProblemRequest, createdBy int64) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 插入题目
	query := `
		INSERT INTO problems (title, difficulty, description, input_format, 
		                      output_format, constraints, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := tx.Exec(
		query, req.Title, req.Difficulty, req.Description,
		req.InputFormat, req.OutputFormat, req.Constraints, createdBy,
	)
	if err != nil {
		return 0, err
	}
	problemID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// 插入标签
	if len(req.Tags) > 0 {
		for _, tag := range req.Tags {
			_, err = tx.Exec(
				`INSERT INTO problem_tags (problem_id, tag) VALUES (?, ?)`,
				problemID, tag,
			)
			if err != nil {
				return 0, err
			}
		}
	}

	// 插入示例
	if len(req.Examples) > 0 {
		for i, ex := range req.Examples {
			_, err = tx.Exec(
				`INSERT INTO problem_examples (problem_id, input, output, explanation, display_order) 
				 VALUES (?, ?, ?, ?, ?)`,
				problemID, ex.Input, ex.Output, ex.Explanation, i,
			)
			if err != nil {
				return 0, err
			}
		}
	}

	// 插入测试用例
	if len(req.TestCases) > 0 {
		for i, tc := range req.TestCases {
			_, err = tx.Exec(
				`INSERT INTO test_cases (problem_id, input, expected_output, description, is_sample, display_order) 
				 VALUES (?, ?, ?, ?, ?, ?)`,
				problemID, tc.Input, tc.ExpectedOutput, tc.Description, tc.IsSample, i,
			)
			if err != nil {
				return 0, err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return problemID, nil
}

// Update 更新题目
func (r *ProblemRepository) Update(id int64, req *models.CreateProblemRequest) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 更新题目基本信息
	query := `
		UPDATE problems 
		SET title = ?, difficulty = ?, description = ?, input_format = ?,
		    output_format = ?, constraints = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = tx.Exec(
		query, req.Title, req.Difficulty, req.Description,
		req.InputFormat, req.OutputFormat, req.Constraints,
		time.Now(), id,
	)
	if err != nil {
		return err
	}

	// 删除旧标签并插入新标签
	_, err = tx.Exec(`DELETE FROM problem_tags WHERE problem_id = ?`, id)
	if err != nil {
		return err
	}
	for _, tag := range req.Tags {
		_, err = tx.Exec(
			`INSERT INTO problem_tags (problem_id, tag) VALUES (?, ?)`,
			id, tag,
		)
		if err != nil {
			return err
		}
	}

	// 删除旧示例并插入新示例
	_, err = tx.Exec(`DELETE FROM problem_examples WHERE problem_id = ?`, id)
	if err != nil {
		return err
	}
	for i, ex := range req.Examples {
		_, err = tx.Exec(
			`INSERT INTO problem_examples (problem_id, input, output, explanation, display_order) 
			 VALUES (?, ?, ?, ?, ?)`,
			id, ex.Input, ex.Output, ex.Explanation, i,
		)
		if err != nil {
			return err
		}
	}

	// 删除旧测试用例并插入新测试用例
	_, err = tx.Exec(`DELETE FROM test_cases WHERE problem_id = ?`, id)
	if err != nil {
		return err
	}
	for i, tc := range req.TestCases {
		_, err = tx.Exec(
			`INSERT INTO test_cases (problem_id, input, expected_output, description, is_sample, display_order) 
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, tc.Input, tc.ExpectedOutput, tc.Description, tc.IsSample, i,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Delete 删除题目
func (r *ProblemRepository) Delete(id int64) error {
	query := `DELETE FROM problems WHERE id = ?`
	result, err := r.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("problem not found")
	}

	return nil
}
