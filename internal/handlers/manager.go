package handlers

import (
	"net/http"
	"strconv"

	"github.com/daiXXXXX/programming-backend/internal/auth"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/gin-gonic/gin"
)

// ManagerHandler 后台管理处理器
type ManagerHandler struct {
	classRepo *database.ClassRepository
}

// NewManagerHandler 创建后台管理处理器
func NewManagerHandler(classRepo *database.ClassRepository) *ManagerHandler {
	return &ManagerHandler{
		classRepo: classRepo,
	}
}

// GetMyClasses 获取当前教师的班级列表
// GET /api/manager/my-classes?search=keyword
func (h *ManagerHandler) GetMyClasses(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	search := c.Query("search")
	classes, err := h.classRepo.GetClassesByTeacher(userID.(int64), search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取班级列表失败"})
		return
	}

	c.JSON(http.StatusOK, classes)
}

// GetAllClasses 获取所有班级列表（管理员）
// GET /api/manager/classes?search=keyword
func (h *ManagerHandler) GetAllClasses(c *gin.Context) {
	// 验证是否为管理员
	role, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	userRole := role.(auth.UserRole)
	if userRole == auth.RoleAdmin {
		// 管理员：返回所有班级
		search := c.Query("search")
		classes, err := h.classRepo.GetAllClasses(search)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取班级列表失败"})
			return
		}
		c.JSON(http.StatusOK, classes)
	} else {
		// 教师：只返回自己的班级
		userID, _ := c.Get("userID")
		search := c.Query("search")
		classes, err := h.classRepo.GetClassesByTeacher(userID.(int64), search)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取班级列表失败"})
			return
		}
		c.JSON(http.StatusOK, classes)
	}
}

// GetClassDetail 获取班级详情
// GET /api/manager/classes/:id
func (h *ManagerHandler) GetClassDetail(c *gin.Context) {
	classIDStr := c.Param("id")
	classID, err := strconv.ParseInt(classIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的班级ID"})
		return
	}

	// 获取用户信息
	userID, _ := c.Get("userID")
	role, _ := c.Get("role")
	userRole := role.(auth.UserRole)

	// 如果不是管理员，需要验证该班级属于当前教师
	if userRole != auth.RoleAdmin {
		classInfo, err := h.classRepo.GetClassByID(classID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取班级信息失败"})
			return
		}
		if classInfo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "班级不存在"})
			return
		}
		if classInfo.TeacherID != userID.(int64) {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权查看该班级"})
			return
		}
	}

	// 获取班级完整详情
	detail, err := h.classRepo.GetClassDetail(classID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取班级详情失败"})
		return
	}
	if detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "班级不存在"})
		return
	}

	c.JSON(http.StatusOK, detail)
}
