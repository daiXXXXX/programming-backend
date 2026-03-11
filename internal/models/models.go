package models

import "time"

type DifficultyLevel string

const (
	DifficultyEasy   DifficultyLevel = "Easy"
	DifficultyMedium DifficultyLevel = "Medium"
	DifficultyHard   DifficultyLevel = "Hard"
)

type SubmissionStatus string

const (
	StatusAccepted          SubmissionStatus = "Accepted"
	StatusWrongAnswer       SubmissionStatus = "Wrong Answer"
	StatusRuntimeError      SubmissionStatus = "Runtime Error"
	StatusTimeLimitExceeded SubmissionStatus = "Time Limit Exceeded"
)

// Problem 题目
type Problem struct {
	ID           int64           `json:"id"`
	Title        string          `json:"title"`
	Difficulty   DifficultyLevel `json:"difficulty"`
	Description  string          `json:"description"`
	InputFormat  string          `json:"inputFormat"`
	OutputFormat string          `json:"outputFormat"`
	Constraints  string          `json:"constraints"`
	Examples     []Example       `json:"examples"`
	TestCases    []TestCase      `json:"testCases"`
	Tags         []string        `json:"tags"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

// Example 题目示例
type Example struct {
	ID          int64  `json:"id,omitempty"`
	ProblemID   int64  `json:"problemId,omitempty"`
	Input       string `json:"input"`
	Output      string `json:"output"`
	Explanation string `json:"explanation,omitempty"`
}

// TestCase 测试用例
type TestCase struct {
	ID             int64  `json:"id"`
	ProblemID      int64  `json:"problemId,omitempty"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	Description    string `json:"description,omitempty"`
	IsSample       bool   `json:"isSample,omitempty"`
}

// Submission 提交记录
type Submission struct {
	ID          int64            `json:"id"`
	ProblemID   int64            `json:"problemId"`
	UserID      int64            `json:"userId"`
	Code        string           `json:"code"`
	Language    string           `json:"language"`
	Status      SubmissionStatus `json:"status"`
	Score       int              `json:"score"`
	TestResults []TestResult     `json:"testResults,omitempty"`
	SubmittedAt time.Time        `json:"submittedAt"`
}

// TestResult 测试结果
type TestResult struct {
	ID             int64  `json:"id,omitempty"`
	SubmissionID   int64  `json:"submissionId,omitempty"`
	TestCaseID     int64  `json:"testCaseId"`
	Passed         bool   `json:"passed"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	ActualOutput   string `json:"actualOutput,omitempty"`
	Error          string `json:"error,omitempty"`
	ExecutionTime  int    `json:"executionTime,omitempty"` // 毫秒
}

// UserStats 用户统计
type UserStats struct {
	UserID              int64     `json:"userId"`
	TotalSolved         int       `json:"totalSolved"`
	EasySolved          int       `json:"easySolved"`
	MediumSolved        int       `json:"mediumSolved"`
	HardSolved          int       `json:"hardSolved"`
	TotalSubmissions    int       `json:"totalSubmissions"`
	AcceptedSubmissions int       `json:"acceptedSubmissions"`
	TodaySolved         int       `json:"todaySolved"`
	TodayDate           string    `json:"todayDate,omitempty"`
	SuccessRate         float64   `json:"successRate"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// RankingUser 排行榜用户信息
type RankingUser struct {
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	Avatar      string `json:"avatar"`
	TotalSolved int    `json:"totalSolved"`
	TodaySolved int    `json:"todaySolved"`
	Rank        int    `json:"rank"`
}

// DailyActivity 每日刷题活动（用于热力图/绿墙）
type DailyActivity struct {
	UserID          int64  `json:"userId"`
	ActivityDate    string `json:"date"`            // 格式: "2006-01-02"
	SubmissionCount int    `json:"submissionCount"` // 当天提交次数
	SolvedCount     int    `json:"solvedCount"`     // 当天解决题目数
}

// User 用户（简化版，后续可扩展）
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // 不在 JSON 中返回
	Role         string    `json:"role"`
	Avatar       string    `json:"avatar"`
	Bio          string    `json:"bio"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// SubmitCodeRequest 提交代码请求
type SubmitCodeRequest struct {
	ProblemID int64  `json:"problemId" binding:"required"`
	Code      string `json:"code" binding:"required"`
	Language  string `json:"language"`
	UserID    int64  `json:"userId"` // 暂时从请求中获取，后续从 JWT 中提取
}

// CreateProblemRequest 创建题目请求
type CreateProblemRequest struct {
	Title        string          `json:"title" binding:"required"`
	Difficulty   DifficultyLevel `json:"difficulty" binding:"required"`
	Description  string          `json:"description" binding:"required"`
	InputFormat  string          `json:"inputFormat" binding:"required"`
	OutputFormat string          `json:"outputFormat" binding:"required"`
	Constraints  string          `json:"constraints" binding:"required"`
	Examples     []Example       `json:"examples" binding:"required"`
	TestCases    []TestCase      `json:"testCases" binding:"required"`
	Tags         []string        `json:"tags"`
}
