package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/models"
)

type SubmissionHandler struct {
	submissionRepo *database.SubmissionRepository
	problemRepo    *database.ProblemRepository
	evaluator      *evaluator.Evaluator
}

func NewSubmissionHandler(
	submissionRepo *database.SubmissionRepository,
	problemRepo *database.ProblemRepository,
	eval *evaluator.Evaluator,
) *SubmissionHandler {
	return &SubmissionHandler{
		submissionRepo: submissionRepo,
		problemRepo:    problemRepo,
		evaluator:      eval,
	}
}

// SubmitCode 提交代码
// POST /api/submissions
func (h *SubmissionHandler) SubmitCode(c *gin.Context) {
	var req models.SubmitCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 从 JWT 中间件获取用户ID（不信任前端传来的 userID）
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}
	req.UserID = userIDVal.(int64)

	// 获取题目和测试用例
	problem, err := h.problemRepo.GetByID(req.ProblemID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Problem not found",
		})
		return
	}

	// 评测代码
	testResults := h.evaluator.EvaluateCode(req.Code, req.Language, problem.TestCases)
	score := h.evaluator.CalculateScore(testResults)
	status := h.evaluator.GetSubmissionStatus(testResults)

	// 创建提交记录
	submission := &models.Submission{
		ProblemID:   req.ProblemID,
		UserID:      req.UserID,
		Code:        req.Code,
		Language:    req.Language,
		Status:      status,
		Score:       score,
		TestResults: testResults,
	}

	if submission.Language == "" {
		submission.Language = "JavaScript"
	}

	submissionID, err := h.submissionRepo.Create(submission)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save submission",
		})
		return
	}

	// 获取完整的提交记录（包含测试结果）
	savedSubmission, err := h.submissionRepo.GetByID(submissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch submission",
		})
		return
	}

	c.JSON(http.StatusCreated, savedSubmission)
}

// GetSubmission 获取提交详情
// GET /api/submissions/:id
func (h *SubmissionHandler) GetSubmission(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid submission ID",
		})
		return
	}

	submission, err := h.submissionRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Submission not found",
		})
		return
	}

	c.JSON(http.StatusOK, submission)
}

// GetUserSubmissions 获取用户的提交历史
// GET /api/submissions/user/:userId
func (h *SubmissionHandler) GetUserSubmissions(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// 分页参数
	limitStr := c.DefaultQuery("limit", "100")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	submissions, err := h.submissionRepo.GetByUserID(userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch submissions",
		})
		return
	}

	c.JSON(http.StatusOK, submissions)
}

// GetProblemSubmissions 获取题目的提交记录
// GET /api/submissions/problem/:problemId
func (h *SubmissionHandler) GetProblemSubmissions(c *gin.Context) {
	problemIDStr := c.Param("problemId")
	problemID, err := strconv.ParseInt(problemIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid problem ID",
		})
		return
	}

	// 分页参数
	limitStr := c.DefaultQuery("limit", "100")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	submissions, err := h.submissionRepo.GetByProblemID(problemID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch submissions",
		})
		return
	}

	c.JSON(http.StatusOK, submissions)
}

// GetUserStats 获取用户统计信息
// GET /api/stats/user/:userId
func (h *SubmissionHandler) GetUserStats(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	stats, err := h.submissionRepo.GetUserStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch user stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
