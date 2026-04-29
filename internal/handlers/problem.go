package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/cache"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type ProblemHandler struct {
	repo  *database.ProblemRepository
	cache *cache.Cache
	eval  *evaluator.Evaluator
}

func NewProblemHandler(repo *database.ProblemRepository, cache *cache.Cache, eval *evaluator.Evaluator) *ProblemHandler {
	return &ProblemHandler{repo: repo, cache: cache, eval: eval}
}

// GetProblems 获取所有题目（支持按名称模糊搜索）
// GET /api/problems?name=xxx
func (h *ProblemHandler) GetProblems(c *gin.Context) {
	name := c.Query("name")

	// 仅在无搜索关键词时使用缓存（全量列表）
	if name == "" {
		var problems []models.Problem
		if h.cache.Get(c.Request.Context(), &problems, "problems:list") {
			c.JSON(http.StatusOK, problems)
			return
		}
	}

	problems, err := h.repo.GetAll(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch problems",
		})
		return
	}

	// 无搜索关键词时缓存全量列表，60 秒过期
	if name == "" {
		h.cache.Set(c.Request.Context(), problems, 60*time.Second, "problems:list")
	}

	c.JSON(http.StatusOK, problems)
}

// GetDailyRecommendation 获取当前登录用户的每日一题推荐。
// GET /api/problems/daily-recommendation
func (h *ProblemHandler) GetDailyRecommendation(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	userID, ok := userIDVal.(int64)
	if !ok || userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid user identity",
		})
		return
	}

	recommendation, err := h.repo.GetDailyRecommendation(userID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch daily recommendation",
		})
		return
	}

	c.JSON(http.StatusOK, recommendation)
}

// GetProblem 获取单个题目详情
// GET /api/problems/:id
func (h *ProblemHandler) GetProblem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid problem ID",
		})
		return
	}

	// 尝试从缓存读取
	var problem models.Problem
	if h.cache.Get(c.Request.Context(), &problem, "problems:detail:"+idStr) {
		c.JSON(http.StatusOK, &problem)
		return
	}

	p, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Problem not found",
		})
		return
	}

	// 缓存 120 秒
	h.cache.Set(c.Request.Context(), p, 120*time.Second, "problems:detail:"+idStr)

	c.JSON(http.StatusOK, p)
}

// invalidateProblemCache 清除题目相关缓存
func (h *ProblemHandler) invalidateProblemCache(c *gin.Context, problemID string) {
	ctx := c.Request.Context()
	h.cache.Delete(ctx, "problems:list")
	if problemID != "" {
		h.cache.Delete(ctx, "problems:detail:"+problemID)
	}
}

// ValidateProblemDraft 校验标准程序是否能通过录题时提供的测试数据。
// POST /api/problems/validate
func (h *ProblemHandler) ValidateProblemDraft(c *gin.Context) {
	var req models.ValidateProblemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := validateProblemTestCases(req.TestCases); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	validationResult := h.buildValidationResult(req.StandardProgram.Code, req.StandardProgram.Language, req.TestCases)
	c.JSON(http.StatusOK, models.ProblemValidationResponse{
		Ready:  validationResult.Status == models.StatusAccepted,
		Result: validationResult,
	})
}

// CreateProblem 创建新题目
// POST /api/problems
func (h *ProblemHandler) CreateProblem(c *gin.Context) {
	var req models.CreateProblemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := validateProblemPayload(&req, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}
	createdBy := userIDVal.(int64)

	validationResult := h.buildValidationResult(req.StandardProgram.Code, req.StandardProgram.Language, req.TestCases)
	if validationResult.Status != models.StatusAccepted {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "Standard program must pass all test cases before creating the problem",
			"result": validationResult,
		})
		return
	}

	problemID, err := h.repo.Create(&req, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create problem",
		})
		return
	}

	// 清除列表缓存
	h.invalidateProblemCache(c, "")

	problem, err := h.repo.GetByID(problemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch created problem",
		})
		return
	}

	c.JSON(http.StatusCreated, problem)
}

// UpdateProblem 更新题目
// PUT /api/problems/:id
func (h *ProblemHandler) UpdateProblem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid problem ID",
		})
		return
	}

	var req models.CreateProblemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := validateProblemPayload(&req, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if req.StandardProgram != nil {
		validationResult := h.buildValidationResult(req.StandardProgram.Code, req.StandardProgram.Language, req.TestCases)
		if validationResult.Status != models.StatusAccepted {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "Standard program must pass all test cases before updating the problem",
				"result": validationResult,
			})
			return
		}
	}

	if err := h.repo.Update(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update problem",
		})
		return
	}

	// 清除该题目及列表缓存
	h.invalidateProblemCache(c, idStr)

	problem, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch updated problem",
		})
		return
	}

	c.JSON(http.StatusOK, problem)
}

// DeleteProblem 删除题目
// DELETE /api/problems/:id
func (h *ProblemHandler) DeleteProblem(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid problem ID",
		})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete problem",
		})
		return
	}

	// 清除该题目及列表缓存
	h.invalidateProblemCache(c, idStr)

	c.JSON(http.StatusOK, gin.H{
		"message": "Problem deleted successfully",
	})
}

// buildValidationResult 统一执行标准程序校验，供预校验与最终录题复用。
func (h *ProblemHandler) buildValidationResult(code string, language string, testCases []models.TestCase) models.CodeRunResult {
	trimmedLanguage := strings.TrimSpace(language)
	if trimmedLanguage == "" {
		trimmedLanguage = "JavaScript"
	}

	results := h.eval.EvaluateCode(code, trimmedLanguage, testCases)
	return models.CodeRunResult{
		ProblemID:   0,
		Language:    trimmedLanguage,
		Status:      h.eval.GetSubmissionStatus(results),
		Score:       h.eval.CalculateScore(results),
		TestResults: results,
		RanAt:       time.Now(),
	}
}

// validateProblemPayload 检查录题请求的关键信息，避免无效数据进入数据库。
func validateProblemPayload(req *models.CreateProblemRequest, requireStandardProgram bool) error {
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("problem title is required")
	}
	if strings.TrimSpace(req.Description) == "" {
		return fmt.Errorf("problem description is required")
	}
	if strings.TrimSpace(req.InputFormat) == "" {
		return fmt.Errorf("input format is required")
	}
	if strings.TrimSpace(req.OutputFormat) == "" {
		return fmt.Errorf("output format is required")
	}
	if strings.TrimSpace(req.Constraints) == "" {
		return fmt.Errorf("constraints are required")
	}

	if len(req.Examples) == 0 {
		return fmt.Errorf("at least one sample example is required")
	}

	for index, example := range req.Examples {
		if strings.TrimSpace(example.Input) == "" || strings.TrimSpace(example.Output) == "" {
			return fmt.Errorf("sample example %d must include both input and output", index+1)
		}
	}

	if err := validateProblemTestCases(req.TestCases); err != nil {
		return err
	}

	if requireStandardProgram {
		if req.StandardProgram == nil {
			return fmt.Errorf("standard program is required")
		}
		if strings.TrimSpace(req.StandardProgram.Code) == "" {
			return fmt.Errorf("standard program code is required")
		}
	}

	return nil
}

// validateProblemTestCases 检查测试点内容是否完整，并确保至少存在公开样例。
func validateProblemTestCases(testCases []models.TestCase) error {
	if len(testCases) == 0 {
		return fmt.Errorf("at least one test case is required")
	}

	hasSample := false
	for index, testCase := range testCases {
		if strings.TrimSpace(testCase.Input) == "" || strings.TrimSpace(testCase.ExpectedOutput) == "" {
			return fmt.Errorf("test case %d must include both input and expected output", index+1)
		}
		if testCase.IsSample {
			hasSample = true
		}
	}

	if !hasSample {
		return fmt.Errorf("at least one public sample test case is required")
	}

	return nil
}
