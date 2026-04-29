package database

import (
	"database/sql"
	"fmt"
	"strings"
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

const dailyRecommendationRecentLimit = 20

type recentAcceptedProblem struct {
	ID         int64
	Difficulty models.DifficultyLevel
}

type recommendationProfile struct {
	HasRecentAccepted bool
	TagWeights        map[string]float64
	DifficultyWeights map[models.DifficultyLevel]float64
}

// GetAll 获取所有题目（不包含测试用例详情），支持按标题模糊搜索
func (r *ProblemRepository) GetAll(name string) ([]models.Problem, error) {
	var rows *sql.Rows
	var err error

	if name != "" {
		query := `
			SELECT id, title, difficulty, description, input_format, output_format, 
			       constraints_text, created_at, updated_at
			FROM problems
			WHERE title LIKE ?
			ORDER BY id ASC
		`
		rows, err = r.db.Query(query, "%"+name+"%")
	} else {
		query := `
			SELECT id, title, difficulty, description, input_format, output_format, 
			       constraints_text, created_at, updated_at
			FROM problems
			ORDER BY id ASC
		`
		rows, err = r.db.Query(query)
	}
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
		       constraints_text, created_at, updated_at
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

// GetDailyRecommendation 为用户选择一道未 AC 且贴近最近 AC 画像的每日题目。
func (r *ProblemRepository) GetDailyRecommendation(userID int64, today time.Time) (*models.DailyProblemRecommendation, error) {
	profile, err := r.buildRecommendationProfile(userID, dailyRecommendationRecentLimit)
	if err != nil {
		return nil, err
	}

	candidates, err := r.getRecommendationCandidates(userID)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return &models.DailyProblemRecommendation{
			Reason:      "no_available_problem",
			MatchedTags: []string{},
		}, nil
	}

	bestIndex := 0
	bestScore, bestMatchedTags := scoreRecommendationCandidate(candidates[0], profile)
	bestTie := dailyRecommendationTie(candidates[0].ID, today)

	for i := 1; i < len(candidates); i++ {
		score, matchedTags := scoreRecommendationCandidate(candidates[i], profile)
		tie := dailyRecommendationTie(candidates[i].ID, today)
		if score > bestScore || (score == bestScore && tie > bestTie) {
			bestIndex = i
			bestScore = score
			bestTie = tie
			bestMatchedTags = matchedTags
		}
	}

	problem := candidates[bestIndex]
	// 推荐卡片进入详情页时复用题目列表所需的轻量示例和公开测试点元数据。
	problem.Examples, _ = r.GetExamples(problem.ID)
	problem.TestCases, _ = r.GetTestCasesMeta(problem.ID)

	reason := "cold_start"
	if profile.HasRecentAccepted {
		reason = "matched_recent_difficulty"
		if len(bestMatchedTags) > 0 {
			reason = "matched_recent_tags"
		}
	}

	return &models.DailyProblemRecommendation{
		Problem:     &problem,
		Reason:      reason,
		MatchedTags: bestMatchedTags,
	}, nil
}

func (r *ProblemRepository) buildRecommendationProfile(userID int64, limit int) (recommendationProfile, error) {
	profile := recommendationProfile{
		TagWeights:        make(map[string]float64),
		DifficultyWeights: make(map[models.DifficultyLevel]float64),
	}

	query := `
		SELECT ac.problem_id, p.difficulty
		FROM (
			SELECT problem_id, MAX(submitted_at) AS latest_accepted_at
			FROM submissions
			WHERE user_id = ? AND status = ?
			GROUP BY problem_id
			ORDER BY latest_accepted_at DESC
			LIMIT ?
		) ac
		JOIN problems p ON p.id = ac.problem_id
		ORDER BY ac.latest_accepted_at DESC
	`
	rows, err := r.db.Query(query, userID, string(models.StatusAccepted), limit)
	if err != nil {
		return profile, err
	}
	defer rows.Close()

	recentProblems := make([]recentAcceptedProblem, 0, limit)
	for rows.Next() {
		var item recentAcceptedProblem
		if err := rows.Scan(&item.ID, &item.Difficulty); err != nil {
			return profile, err
		}
		recentProblems = append(recentProblems, item)
	}
	if err := rows.Err(); err != nil {
		return profile, err
	}
	if len(recentProblems) == 0 {
		return profile, nil
	}

	profile.HasRecentAccepted = true
	problemIDs := make([]int64, 0, len(recentProblems))
	for index, item := range recentProblems {
		weight := float64(len(recentProblems) - index)
		profile.DifficultyWeights[item.Difficulty] += weight
		problemIDs = append(problemIDs, item.ID)
	}

	tagsByProblem, err := r.getProblemTagsMap(problemIDs)
	if err != nil {
		return profile, err
	}
	for index, item := range recentProblems {
		weight := float64(len(recentProblems) - index)
		for _, tag := range tagsByProblem[item.ID] {
			normalizedTag := normalizeRecommendationTag(tag)
			if normalizedTag != "" {
				profile.TagWeights[normalizedTag] += weight
			}
		}
	}

	return profile, nil
}

func (r *ProblemRepository) getRecommendationCandidates(userID int64) ([]models.Problem, error) {
	query := `
		SELECT p.id, p.title, p.difficulty, p.description, p.input_format, p.output_format,
		       p.constraints_text, p.created_at, p.updated_at,
		       GROUP_CONCAT(pt.tag ORDER BY pt.id SEPARATOR ',') AS tags
		FROM problems p
		LEFT JOIN problem_tags pt ON pt.problem_id = p.id
		WHERE NOT EXISTS (
			SELECT 1
			FROM submissions s
			WHERE s.user_id = ?
			  AND s.problem_id = p.id
			  AND s.status = ?
		)
		GROUP BY p.id, p.title, p.difficulty, p.description, p.input_format, p.output_format,
		         p.constraints_text, p.created_at, p.updated_at
		ORDER BY p.id ASC
	`

	rows, err := r.db.Query(query, userID, string(models.StatusAccepted))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]models.Problem, 0)
	for rows.Next() {
		var problem models.Problem
		var tags sql.NullString
		if err := rows.Scan(
			&problem.ID, &problem.Title, &problem.Difficulty, &problem.Description,
			&problem.InputFormat, &problem.OutputFormat, &problem.Constraints,
			&problem.CreatedAt, &problem.UpdatedAt, &tags,
		); err != nil {
			return nil, err
		}
		if tags.Valid {
			problem.Tags = splitRecommendationTags(tags.String)
		} else {
			problem.Tags = []string{}
		}
		candidates = append(candidates, problem)
	}

	return candidates, rows.Err()
}

func (r *ProblemRepository) getProblemTagsMap(problemIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(problemIDs))
	if len(problemIDs) == 0 {
		return result, nil
	}

	placeholders, args := buildInt64Placeholders(problemIDs)
	query := fmt.Sprintf(`
		SELECT problem_id, tag
		FROM problem_tags
		WHERE problem_id IN (%s)
		ORDER BY id
	`, placeholders)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var problemID int64
		var tag string
		if err := rows.Scan(&problemID, &tag); err != nil {
			return nil, err
		}
		result[problemID] = append(result[problemID], tag)
	}

	return result, rows.Err()
}

func buildInt64Placeholders(values []int64) (string, []interface{}) {
	placeholders := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	return strings.Join(placeholders, ","), args
}

func scoreRecommendationCandidate(problem models.Problem, profile recommendationProfile) (float64, []string) {
	if !profile.HasRecentAccepted {
		return coldStartDifficultyScore(problem.Difficulty), []string{}
	}

	score := 0.0
	matchedTags := make([]string, 0)
	seenTags := make(map[string]struct{}, len(problem.Tags))
	for _, tag := range problem.Tags {
		normalizedTag := normalizeRecommendationTag(tag)
		if normalizedTag == "" {
			continue
		}
		if _, exists := seenTags[normalizedTag]; exists {
			continue
		}
		seenTags[normalizedTag] = struct{}{}

		if weight, ok := profile.TagWeights[normalizedTag]; ok {
			score += weight * 3
			matchedTags = append(matchedTags, tag)
		}
	}

	for difficulty, weight := range profile.DifficultyWeights {
		score += weight * difficultySimilarity(problem.Difficulty, difficulty)
	}

	return score, matchedTags
}

func normalizeRecommendationTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

func splitRecommendationTags(raw string) []string {
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func difficultySimilarity(candidate, recent models.DifficultyLevel) float64 {
	distance := difficultyRank(candidate) - difficultyRank(recent)
	if distance < 0 {
		distance = -distance
	}

	switch distance {
	case 0:
		return 1
	case 1:
		return 0.55
	default:
		return 0.15
	}
}

func difficultyRank(difficulty models.DifficultyLevel) int {
	switch difficulty {
	case models.DifficultyEasy:
		return 1
	case models.DifficultyMedium:
		return 2
	case models.DifficultyHard:
		return 3
	default:
		return 2
	}
}

func coldStartDifficultyScore(difficulty models.DifficultyLevel) float64 {
	switch difficulty {
	case models.DifficultyEasy:
		return 3
	case models.DifficultyMedium:
		return 2
	case models.DifficultyHard:
		return 1
	default:
		return 2
	}
}

func dailyRecommendationTie(problemID int64, today time.Time) int64 {
	dayKey := int64(today.Year()*1000 + today.YearDay())
	return (problemID*1103515245 + dayKey*12345) % 2147483647
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
		                      output_format, constraints_text, created_by)
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

	// 录题时同步保存标准程序，便于后续复查测试数据与题目配置。
	if err = r.upsertStandardProgram(tx, problemID, req.StandardProgram, createdBy); err != nil {
		return 0, err
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
		    output_format = ?, constraints_text = ?, updated_at = ?
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

	// 若前端提交了新的标准程序，则替换旧版本；未提交时保留现有留档数据。
	if req.StandardProgram != nil {
		if err = r.upsertStandardProgram(tx, id, req.StandardProgram, 0); err != nil {
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

// upsertStandardProgram 在题目创建或更新时同步维护标准程序留档。
func (r *ProblemRepository) upsertStandardProgram(tx *sql.Tx, problemID int64, program *models.ProblemStandardProgram, createdBy int64) error {
	if program == nil {
		return nil
	}

	if _, err := tx.Exec(`DELETE FROM problem_reference_solutions WHERE problem_id = ?`, problemID); err != nil {
		return err
	}

	var createdByValue interface{}
	if createdBy > 0 {
		createdByValue = createdBy
	}

	_, err := tx.Exec(
		`INSERT INTO problem_reference_solutions (problem_id, language, source_code, created_by)
		 VALUES (?, ?, ?, ?)`,
		problemID, program.Language, program.Code, createdByValue,
	)
	return err
}
