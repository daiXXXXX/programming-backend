package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/daiXXXXX/programming-backend/internal/auth"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/daiXXXXX/programming-backend/internal/plagiarism"
	"github.com/gin-gonic/gin"
)

// ManagerHandler defines instructor/admin management endpoints.
type ManagerHandler struct {
	classRepo         *database.ClassRepository
	problemRepo       *database.ProblemRepository
	plagiarismService *plagiarism.Service
}

// NewManagerHandler creates the manager handler.
func NewManagerHandler(
	classRepo *database.ClassRepository,
	problemRepo *database.ProblemRepository,
	plagiarismService *plagiarism.Service,
) *ManagerHandler {
	return &ManagerHandler{
		classRepo:         classRepo,
		problemRepo:       problemRepo,
		plagiarismService: plagiarismService,
	}
}

// GetMyClasses returns classes owned by the current instructor.
// GET /api/manager/my-classes?search=keyword
func (h *ManagerHandler) GetMyClasses(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	search := c.Query("search")
	classes, err := h.classRepo.GetClassesByTeacher(userID.(int64), search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch classes"})
		return
	}

	c.JSON(http.StatusOK, classes)
}

// GetAllClasses returns all classes for admins, or owned classes for instructors.
// GET /api/manager/classes?search=keyword
func (h *ManagerHandler) GetAllClasses(c *gin.Context) {
	role, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	search := c.Query("search")
	userRole := role.(auth.UserRole)
	if userRole == auth.RoleAdmin {
		classes, err := h.classRepo.GetAllClasses(search)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch classes"})
			return
		}
		c.JSON(http.StatusOK, classes)
		return
	}

	userID, _ := c.Get("userID")
	classes, err := h.classRepo.GetClassesByTeacher(userID.(int64), search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch classes"})
		return
	}

	c.JSON(http.StatusOK, classes)
}

func (h *ManagerHandler) ensureClassAccess(c *gin.Context, classID int64) bool {
	userID, userExists := c.Get("userID")
	role, roleExists := c.Get("role")
	if !userExists || !roleExists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return false
	}

	classInfo, err := h.classRepo.GetClassByID(classID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch class info"})
		return false
	}
	if classInfo == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Class not found"})
		return false
	}

	userRole := role.(auth.UserRole)
	if userRole != auth.RoleAdmin && classInfo.TeacherID != userID.(int64) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return false
	}

	return true
}

// GetClassDetail returns the full class detail for instructors/admins.
// GET /api/manager/classes/:id
func (h *ManagerHandler) GetClassDetail(c *gin.Context) {
	classIDStr := c.Param("id")
	classID, err := strconv.ParseInt(classIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid class ID"})
		return
	}

	if !h.ensureClassAccess(c, classID) {
		return
	}

	detail, err := h.classRepo.GetClassDetail(classID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch class detail"})
		return
	}
	if detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Class not found"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// CheckClassPlagiarism analyzes suspicious submissions inside a class for one problem.
// POST /api/manager/classes/:id/plagiarism-check
func (h *ManagerHandler) CheckClassPlagiarism(c *gin.Context) {
	classIDStr := c.Param("id")
	classID, err := strconv.ParseInt(classIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid class ID"})
		return
	}

	if !h.ensureClassAccess(c, classID) {
		return
	}

	var req models.PlagiarismCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	problem, err := h.problemRepo.GetByID(req.ProblemID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Problem not found"})
		return
	}

	submissions, err := h.classRepo.GetRepresentativeProblemSubmissions(classID, req.ProblemID, req.AcceptedOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch class submissions"})
		return
	}

	report, err := h.plagiarismService.CheckClassProblem(c.Request.Context(), classID, problem, req, submissions)
	if err != nil {
		if errors.Is(err, plagiarism.ErrAnalyzerNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "AI plagiarism analysis is not configured",
				"message": "Set OPENAI_API_KEY before using the plagiarism check endpoint",
			})
			return
		}

		c.JSON(http.StatusBadGateway, gin.H{"error": "AI plagiarism analysis failed"})
		return
	}

	c.JSON(http.StatusOK, report)
}
