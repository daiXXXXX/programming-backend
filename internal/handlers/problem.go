package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/models"
)

type ProblemHandler struct {
	repo *database.ProblemRepository
}

func NewProblemHandler(repo *database.ProblemRepository) *ProblemHandler {
	return &ProblemHandler{repo: repo}
}

// GetProblems 获取所有题目（支持按名称模糊搜索）
// GET /api/problems?name=xxx
func (h *ProblemHandler) GetProblems(c *gin.Context) {
	name := c.Query("name")
	problems, err := h.repo.GetAll(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch problems",
		})
		return
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

	problem, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Problem not found",
		})
		return
	}

	c.JSON(http.StatusOK, problem)
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

	c.JSON(http.StatusOK, gin.H{
		"message": "Problem deleted successfully",
	})
}
