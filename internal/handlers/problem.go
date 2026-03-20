package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/cache"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type ProblemHandler struct {
	repo  *database.ProblemRepository
	cache *cache.Cache
}

func NewProblemHandler(repo *database.ProblemRepository, cache *cache.Cache) *ProblemHandler {
	return &ProblemHandler{repo: repo, cache: cache}
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

	// TODO: 从 JWT 中获取用户ID，这里暂时硬编码
	createdBy := int64(1)

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
