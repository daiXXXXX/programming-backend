package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// ClassInfo 班级列表信息
type ClassInfo struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	TeacherID       int64     `json:"teacherId"`
	TeacherName     string    `json:"teacherName"`
	StudentCount    int       `json:"studentCount"`
	ExperimentCount int       `json:"experimentCount"`
	CreatedAt       time.Time `json:"createdAt"`
}

// ExperimentInfo 实验信息
type ExperimentInfo struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	IsActive     bool      `json:"isActive"`
	ProblemCount int       `json:"problemCount"`
}

// StudentProgress 学生进度
type StudentProgress struct {
	UserID              int64      `json:"userId"`
	Username            string     `json:"username"`
	Avatar              string     `json:"avatar"`
	TotalProblems       int        `json:"totalProblems"`
	SolvedProblems      int        `json:"solvedProblems"`
	TotalSubmissions    int        `json:"totalSubmissions"`
	AcceptedSubmissions int        `json:"acceptedSubmissions"`
	LastSubmissionAt    *time.Time `json:"lastSubmissionAt"`
}

// ClassProblemInfo 班级实验题目摘要，供教师在管理端选择查重目标题目。
type ClassProblemInfo struct {
	ID              int64                  `json:"id"`
	Title           string                 `json:"title"`
	Difficulty      models.DifficultyLevel `json:"difficulty"`
	ExperimentCount int                    `json:"experimentCount"`
}

// ClassDetailData 班级详情
type ClassDetailData struct {
	ClassInfo   ClassInfo          `json:"classInfo"`
	Experiments []ExperimentInfo   `json:"experiments"`
	Problems    []ClassProblemInfo `json:"problems"`
	Students    []StudentProgress  `json:"students"`
}

// ClassRepository 班级仓库
type ClassRepository struct {
	db *DB
}

// NewClassRepository 创建班级仓库
func NewClassRepository(db *DB) *ClassRepository {
	return &ClassRepository{db: db}
}

// GetClassesByTeacher 获取教师的班级列表（支持搜索）
func (r *ClassRepository) GetClassesByTeacher(teacherID int64, search string) ([]ClassInfo, error) {
	query := `
		SELECT 
			c.id, c.name, COALESCE(c.description, '') as description, 
			c.teacher_id, u.username as teacher_name,
			(SELECT COUNT(*) FROM class_students cs WHERE cs.class_id = c.id) as student_count,
			(SELECT COUNT(*) FROM class_experiments ce WHERE ce.class_id = c.id) as experiment_count,
			c.created_at
		FROM classes c
		JOIN users u ON c.teacher_id = u.id
		WHERE c.teacher_id = ?
	`
	args := []interface{}{teacherID}

	if search != "" {
		query += " AND c.name LIKE ?"
		args = append(args, "%"+search+"%")
	}

	query += " ORDER BY c.created_at DESC"

	return r.scanClassInfos(query, args...)
}

// GetAllClasses 获取所有班级列表（管理员，支持搜索）
func (r *ClassRepository) GetAllClasses(search string) ([]ClassInfo, error) {
	query := `
		SELECT 
			c.id, c.name, COALESCE(c.description, '') as description, 
			c.teacher_id, u.username as teacher_name,
			(SELECT COUNT(*) FROM class_students cs WHERE cs.class_id = c.id) as student_count,
			(SELECT COUNT(*) FROM class_experiments ce WHERE ce.class_id = c.id) as experiment_count,
			c.created_at
		FROM classes c
		JOIN users u ON c.teacher_id = u.id
	`
	var args []interface{}

	if search != "" {
		query += " WHERE c.name LIKE ?"
		args = append(args, "%"+search+"%")
	}

	query += " ORDER BY c.created_at DESC"

	return r.scanClassInfos(query, args...)
}

// scanClassInfos 扫描班级信息列表
func (r *ClassRepository) scanClassInfos(query string, args ...interface{}) ([]ClassInfo, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var classes []ClassInfo
	for rows.Next() {
		var c ClassInfo
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Description,
			&c.TeacherID, &c.TeacherName,
			&c.StudentCount, &c.ExperimentCount,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		classes = append(classes, c)
	}

	if classes == nil {
		classes = []ClassInfo{}
	}
	return classes, nil
}

// GetClassByID 根据ID获取班级基本信息
func (r *ClassRepository) GetClassByID(classID int64) (*ClassInfo, error) {
	query := `
		SELECT 
			c.id, c.name, COALESCE(c.description, '') as description, 
			c.teacher_id, u.username as teacher_name,
			(SELECT COUNT(*) FROM class_students cs WHERE cs.class_id = c.id) as student_count,
			(SELECT COUNT(*) FROM class_experiments ce WHERE ce.class_id = c.id) as experiment_count,
			c.created_at
		FROM classes c
		JOIN users u ON c.teacher_id = u.id
		WHERE c.id = ?
	`
	var c ClassInfo
	err := r.db.QueryRow(query, classID).Scan(
		&c.ID, &c.Name, &c.Description,
		&c.TeacherID, &c.TeacherName,
		&c.StudentCount, &c.ExperimentCount,
		&c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetClassExperiments 获取班级关联的实验列表
func (r *ClassRepository) GetClassExperiments(classID int64) ([]ExperimentInfo, error) {
	query := `
		SELECT 
			e.id, e.title, COALESCE(e.description, '') as description,
			e.start_time, e.end_time, COALESCE(e.is_active, 1) as is_active,
			(SELECT COUNT(*) FROM experiment_problems ep WHERE ep.experiment_id = e.id) as problem_count
		FROM experiments e
		JOIN class_experiments ce ON e.id = ce.experiment_id
		WHERE ce.class_id = ?
		ORDER BY e.start_time DESC
	`
	rows, err := r.db.Query(query, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var experiments []ExperimentInfo
	for rows.Next() {
		var exp ExperimentInfo
		if err := rows.Scan(
			&exp.ID, &exp.Title, &exp.Description,
			&exp.StartTime, &exp.EndTime, &exp.IsActive,
			&exp.ProblemCount,
		); err != nil {
			return nil, err
		}
		experiments = append(experiments, exp)
	}

	if experiments == nil {
		experiments = []ExperimentInfo{}
	}
	return experiments, nil
}

// GetClassStudentProgress 获取班级学生的练习进度
// 该函数计算每个学生针对该班级所有实验中题目的完成情况
func (r *ClassRepository) GetClassStudentProgress(classID int64) ([]StudentProgress, error) {
	// 先获取该班级下所有实验关联的题目总数
	totalProblemsQuery := `
		SELECT COUNT(DISTINCT ep.problem_id)
		FROM class_experiments ce
		JOIN experiment_problems ep ON ce.experiment_id = ep.experiment_id
		WHERE ce.class_id = ?
	`
	var totalProblems int
	if err := r.db.QueryRow(totalProblemsQuery, classID).Scan(&totalProblems); err != nil {
		return nil, err
	}

	// 获取每个学生的进度信息
	// 通过子查询分别获取：已解决的题目数、总提交数、通过提交数、最近提交时间
	query := `
		SELECT 
			u.id as user_id,
			u.username,
			COALESCE(u.avatar, '') as avatar,
			? as total_problems,
			COALESCE(solved.solved_count, 0) as solved_problems,
			COALESCE(sub_stats.total_submissions, 0) as total_submissions,
			COALESCE(sub_stats.accepted_submissions, 0) as accepted_submissions,
			sub_stats.last_submission_at
		FROM class_students cs
		JOIN users u ON cs.student_id = u.id
		LEFT JOIN (
			-- 每个学生在该班级实验题目中解决的不同题目数
			SELECT 
				s.user_id,
				COUNT(DISTINCT s.problem_id) as solved_count
			FROM submissions s
			WHERE s.status = 'Accepted'
			AND s.problem_id IN (
				SELECT DISTINCT ep.problem_id
				FROM class_experiments ce
				JOIN experiment_problems ep ON ce.experiment_id = ep.experiment_id
				WHERE ce.class_id = ?
			)
			AND s.user_id IN (SELECT student_id FROM class_students WHERE class_id = ?)
			GROUP BY s.user_id
		) solved ON u.id = solved.user_id
		LEFT JOIN (
			-- 每个学生在该班级实验题目中的提交统计
			SELECT 
				s.user_id,
				COUNT(*) as total_submissions,
				SUM(CASE WHEN s.status = 'Accepted' THEN 1 ELSE 0 END) as accepted_submissions,
				MAX(s.submitted_at) as last_submission_at
			FROM submissions s
			WHERE s.problem_id IN (
				SELECT DISTINCT ep.problem_id
				FROM class_experiments ce
				JOIN experiment_problems ep ON ce.experiment_id = ep.experiment_id
				WHERE ce.class_id = ?
			)
			AND s.user_id IN (SELECT student_id FROM class_students WHERE class_id = ?)
			GROUP BY s.user_id
		) sub_stats ON u.id = sub_stats.user_id
		WHERE cs.class_id = ?
		ORDER BY solved_problems DESC, u.username ASC
	`

	rows, err := r.db.Query(query, totalProblems, classID, classID, classID, classID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var students []StudentProgress
	for rows.Next() {
		var s StudentProgress
		var lastSubmission sql.NullTime
		if err := rows.Scan(
			&s.UserID, &s.Username, &s.Avatar,
			&s.TotalProblems, &s.SolvedProblems,
			&s.TotalSubmissions, &s.AcceptedSubmissions,
			&lastSubmission,
		); err != nil {
			return nil, err
		}
		if lastSubmission.Valid {
			s.LastSubmissionAt = &lastSubmission.Time
		}
		students = append(students, s)
	}

	if students == nil {
		students = []StudentProgress{}
	}
	return students, nil
}

// GetRepresentativeProblemSubmissions returns one representative submission per student
// for a problem inside a class. By default it prefers the latest accepted submission,
// and falls back to the latest submission when a student has no accepted solution.
func (r *ClassRepository) GetRepresentativeProblemSubmissions(classID, problemID int64, acceptedOnly bool) ([]models.ClassProblemSubmission, error) {
	query := `
		SELECT
			s.id,
			s.problem_id,
			s.user_id,
			u.username,
			COALESCE(u.avatar, '') as avatar,
			s.code,
			s.language,
			s.status,
			s.score,
			s.submitted_at
		FROM class_students cs
		JOIN users u ON cs.student_id = u.id
		JOIN submissions s ON s.user_id = cs.student_id
		WHERE cs.class_id = ? AND s.problem_id = ?
	`
	args := []interface{}{classID, problemID}

	if acceptedOnly {
		query += " AND s.status = 'Accepted'"
	}

	query += `
		ORDER BY
			s.user_id ASC,
			CASE WHEN s.status = 'Accepted' THEN 0 ELSE 1 END ASC,
			s.submitted_at DESC,
			s.id DESC
	`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	selected := make(map[int64]struct{})
	var submissions []models.ClassProblemSubmission

	for rows.Next() {
		var submission models.ClassProblemSubmission
		var status string

		if err := rows.Scan(
			&submission.SubmissionID,
			&submission.ProblemID,
			&submission.UserID,
			&submission.Username,
			&submission.Avatar,
			&submission.Code,
			&submission.Language,
			&status,
			&submission.Score,
			&submission.SubmittedAt,
		); err != nil {
			return nil, err
		}

		if _, exists := selected[submission.UserID]; exists {
			continue
		}

		submission.Status = models.SubmissionStatus(status)
		if acceptedOnly || submission.Status == models.StatusAccepted {
			submission.Selection = "latest_accepted"
		} else {
			submission.Selection = "latest_submission"
		}

		submissions = append(submissions, submission)
		selected[submission.UserID] = struct{}{}
	}

	if submissions == nil {
		submissions = []models.ClassProblemSubmission{}
	}

	tagMap, err := r.getSubmissionTagsMap(extractSubmissionIDs(submissions))
	if err != nil {
		return nil, err
	}
	for i := range submissions {
		submissions[i].Tags = tagMap[submissions[i].SubmissionID]
		submissions[i].MarkedCheating = hasSubmissionTag(submissions[i].Tags, models.SubmissionTagCheating)
	}

	return submissions, rows.Err()
}

// GetClassProblems 返回班级全部实验中出现过的去重题目列表。
func (r *ClassRepository) GetClassProblems(classID int64) ([]ClassProblemInfo, error) {
	query := `
		SELECT
			p.id,
			p.title,
			p.difficulty,
			COUNT(DISTINCT ce.experiment_id) AS experiment_count
		FROM class_experiments ce
		JOIN experiment_problems ep ON ce.experiment_id = ep.experiment_id
		JOIN problems p ON p.id = ep.problem_id
		WHERE ce.class_id = ?
		GROUP BY p.id, p.title, p.difficulty
		ORDER BY p.title ASC, p.id ASC
	`

	rows, err := r.db.Query(query, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	problems := make([]ClassProblemInfo, 0)
	for rows.Next() {
		var item ClassProblemInfo
		if err := rows.Scan(&item.ID, &item.Title, &item.Difficulty, &item.ExperimentCount); err != nil {
			return nil, err
		}
		problems = append(problems, item)
	}

	if problems == nil {
		problems = []ClassProblemInfo{}
	}
	return problems, rows.Err()
}

// GetClassProblemSubmissionPair 校验两条提交是否都属于当前班级、当前题目，
// 并返回打标后需要回显给前端的提交摘要。
func (r *ClassRepository) GetClassProblemSubmissionPair(classID, problemID, submissionAID, submissionBID int64) (*models.ClassProblemSubmission, *models.ClassProblemSubmission, error) {
	query := `
		SELECT
			s.id,
			s.problem_id,
			s.user_id,
			u.username,
			COALESCE(u.avatar, '') AS avatar,
			s.code,
			s.language,
			s.status,
			s.score,
			s.submitted_at
		FROM submissions s
		JOIN class_students cs ON cs.student_id = s.user_id
		JOIN users u ON u.id = s.user_id
		WHERE cs.class_id = ?
		  AND s.problem_id = ?
		  AND s.id IN (?, ?)
		ORDER BY s.user_id ASC, s.id ASC
	`

	rows, err := r.db.Query(query, classID, problemID, submissionAID, submissionBID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	submissions := make([]models.ClassProblemSubmission, 0, 2)
	for rows.Next() {
		var submission models.ClassProblemSubmission
		var status string
		if err := rows.Scan(
			&submission.SubmissionID,
			&submission.ProblemID,
			&submission.UserID,
			&submission.Username,
			&submission.Avatar,
			&submission.Code,
			&submission.Language,
			&status,
			&submission.Score,
			&submission.SubmittedAt,
		); err != nil {
			return nil, nil, err
		}
		submission.Status = models.SubmissionStatus(status)
		submission.Selection = "manual_review"
		submissions = append(submissions, submission)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(submissions) != 2 {
		return nil, nil, fmt.Errorf("submission pair not found in class")
	}
	if submissions[0].SubmissionID == submissions[1].SubmissionID {
		return nil, nil, fmt.Errorf("submission pair must contain two different submissions")
	}

	tagMap, err := r.getSubmissionTagsMap([]int64{submissions[0].SubmissionID, submissions[1].SubmissionID})
	if err != nil {
		return nil, nil, err
	}
	for i := range submissions {
		submissions[i].Tags = tagMap[submissions[i].SubmissionID]
		submissions[i].MarkedCheating = hasSubmissionTag(submissions[i].Tags, models.SubmissionTagCheating)
	}

	left, right := submissions[0], submissions[1]
	if left.UserID > right.UserID {
		left, right = right, left
	}
	return &left, &right, nil
}

// GetClassDetail 获取班级完整详情
func (r *ClassRepository) GetClassDetail(classID int64) (*ClassDetailData, error) {
	// 获取班级基本信息
	classInfo, err := r.GetClassByID(classID)
	if err != nil {
		return nil, err
	}
	if classInfo == nil {
		return nil, nil
	}

	// 获取实验列表
	experiments, err := r.GetClassExperiments(classID)
	if err != nil {
		return nil, err
	}

	// 获取班级题目列表，供教师在管理页直接选择查重目标。
	problems, err := r.GetClassProblems(classID)
	if err != nil {
		return nil, err
	}

	// 获取学生进度
	students, err := r.GetClassStudentProgress(classID)
	if err != nil {
		return nil, err
	}

	return &ClassDetailData{
		ClassInfo:   *classInfo,
		Experiments: experiments,
		Problems:    problems,
		Students:    students,
	}, nil
}

func (r *ClassRepository) getSubmissionTagsMap(submissionIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(submissionIDs))
	if len(submissionIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, 0, len(submissionIDs))
	args := make([]interface{}, 0, len(submissionIDs))
	for _, id := range submissionIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
		result[id] = []string{}
	}

	query := fmt.Sprintf(`
		SELECT submission_id, tag
		FROM submission_tags
		WHERE submission_id IN (%s)
		ORDER BY submission_id ASC, id ASC
	`, strings.Join(placeholders, ", "))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		if isMissingTableError(err, "submission_tags") {
			return result, nil
		}
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var submissionID int64
		var tag string
		if err := rows.Scan(&submissionID, &tag); err != nil {
			return nil, err
		}
		result[submissionID] = append(result[submissionID], tag)
	}

	return result, rows.Err()
}

func extractSubmissionIDs(submissions []models.ClassProblemSubmission) []int64 {
	ids := make([]int64, 0, len(submissions))
	for _, submission := range submissions {
		ids = append(ids, submission.SubmissionID)
	}
	return ids
}

func hasSubmissionTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
}

func isMissingTableError(err error, tableName string) bool {
	return err != nil && strings.Contains(err.Error(), tableName) && strings.Contains(strings.ToLower(err.Error()), "doesn't exist")
}
