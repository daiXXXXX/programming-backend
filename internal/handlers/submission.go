package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/cache"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/daiXXXXX/programming-backend/internal/worker"
	"github.com/gin-gonic/gin"
)

type SubmissionHandler struct {
	submissionRepo *database.SubmissionRepository
	problemRepo    *database.ProblemRepository
	evaluator      *evaluator.Evaluator
	cache          *cache.Cache
}

func NewSubmissionHandler(
	submissionRepo *database.SubmissionRepository,
	problemRepo *database.ProblemRepository,
	eval *evaluator.Evaluator,
	cache *cache.Cache,
) *SubmissionHandler {
	return &SubmissionHandler{
		submissionRepo: submissionRepo,
		problemRepo:    problemRepo,
		evaluator:      eval,
		cache:          cache,
	}
}

// SubmitCode 提交代码
// POST /api/submissions
// Redis 可用时异步评测（立即返回 Pending），否则同步评测
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

	// 检查题目是否存在
	_, err := h.problemRepo.GetByID(req.ProblemID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Problem not found",
		})
		return
	}

	if req.Language == "" {
		req.Language = "JavaScript"
	}

	// ==== 异步模式：Redis 可用时走队列 ====
	if h.cache.IsAvailable() {
		submission := &models.Submission{
			ProblemID: req.ProblemID,
			UserID:    req.UserID,
			Code:      req.Code,
			Language:  req.Language,
		}

		// 先在数据库创建 Pending 记录
		submissionID, err := h.submissionRepo.CreatePending(submission)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to save submission",
			})
			return
		}

		// 推入 Redis 评测队列
		task := worker.JudgeTask{
			SubmissionID: submissionID,
			ProblemID:    req.ProblemID,
			UserID:       req.UserID,
			Code:         req.Code,
			Language:     req.Language,
		}

		if err := h.cache.QueuePush(c.Request.Context(), &task, worker.QueueKey); err != nil {
			log.Printf("[Submit] 推入队列失败，回退到同步评测: %v", err)
			// 队列推送失败，回退到同步模式
			h.syncEvaluate(c, submissionID, req)
			return
		}

		queueLen := h.cache.QueueLen(c.Request.Context(), worker.QueueKey)
		log.Printf("[Submit] 任务已入队: submission=%d, 当前队列长度=%d", submissionID, queueLen)

		// 立即返回 Pending 状态
		c.JSON(http.StatusAccepted, gin.H{
			"id":          submissionID,
			"problemId":   req.ProblemID,
			"userId":      req.UserID,
			"code":        req.Code,
			"language":    req.Language,
			"status":      models.StatusPending,
			"score":       0,
			"submittedAt": time.Now(),
			"message":     "提交成功，正在评测中...",
		})
		return
	}

	// ==== 同步模式：Redis 不可用时走原逻辑 ====
	h.syncSubmit(c, req)
}

// syncSubmit 同步提交评测（原逻辑，Redis 不可用时的降级路径）
func (h *SubmissionHandler) syncSubmit(c *gin.Context, req models.SubmitCodeRequest) {
	problem, _ := h.problemRepo.GetByID(req.ProblemID)

	testResults := h.evaluator.EvaluateCode(req.Code, req.Language, problem.TestCases)
	score := h.evaluator.CalculateScore(testResults)
	status := h.evaluator.GetSubmissionStatus(testResults)

	submission := &models.Submission{
		ProblemID:   req.ProblemID,
		UserID:      req.UserID,
		Code:        req.Code,
		Language:    req.Language,
		Status:      status,
		Score:       score,
		TestResults: testResults,
	}

	submissionID, err := h.submissionRepo.Create(submission)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save submission"})
		return
	}

	savedSubmission, err := h.submissionRepo.GetByID(submissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submission"})
		return
	}

	c.JSON(http.StatusCreated, savedSubmission)
}

// syncEvaluate 同步评测已创建的 Pending 记录（队列推送失败时的回退）
func (h *SubmissionHandler) syncEvaluate(c *gin.Context, submissionID int64, req models.SubmitCodeRequest) {
	problem, _ := h.problemRepo.GetByID(req.ProblemID)

	testResults := h.evaluator.EvaluateCode(req.Code, req.Language, problem.TestCases)
	score := h.evaluator.CalculateScore(testResults)
	status := h.evaluator.GetSubmissionStatus(testResults)

	if err := h.submissionRepo.UpdateResult(submissionID, status, score, testResults); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update submission"})
		return
	}

	savedSubmission, err := h.submissionRepo.GetByID(submissionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch submission"})
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

// GetDailyActivity 获取用户每日活动数据（热力图/绿墙）
// GET /api/stats/user/:userId/activity?start=2025-01-01&end=2025-12-31
func (h *SubmissionHandler) GetDailyActivity(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// 获取日期范围参数，默认最近365天
	startDate := c.DefaultQuery("start", "")
	endDate := c.DefaultQuery("end", "")

	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(-1, 0, 0).Format("2006-01-02")
	}

	activities, err := h.submissionRepo.GetDailyActivity(userID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch daily activity",
		})
		return
	}

	if activities == nil {
		activities = []models.DailyActivity{}
	}

	c.JSON(http.StatusOK, activities)
}
