package handlers

import (
	"net/http"
	"strconv"

	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/gin-gonic/gin"
)

// RankingHandler 排行榜处理器
type RankingHandler struct {
	userRepo *database.UserRepository
}

// NewRankingHandler 创建排行榜处理器
func NewRankingHandler(userRepo *database.UserRepository) *RankingHandler {
	return &RankingHandler{
		userRepo: userRepo,
	}
}

// GetTotalSolvedRanking 获取总刷题数排行榜
// GET /api/ranking/total?limit=50
func (h *RankingHandler) GetTotalSolvedRanking(c *gin.Context) {
	limit := 50 // 默认返回前50名
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	users, err := h.userRepo.GetTotalSolvedRanking(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取排行榜失败"})
		return
	}

	c.JSON(http.StatusOK, users)
}

// GetTodaySolvedRanking 获取今日刷题数排行榜
// GET /api/ranking/today?limit=50
func (h *RankingHandler) GetTodaySolvedRanking(c *gin.Context) {
	limit := 50 // 默认返回前50名
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	users, err := h.userRepo.GetTodaySolvedRanking(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取排行榜失败"})
		return
	}

	c.JSON(http.StatusOK, users)
}
