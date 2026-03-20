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

// RankingHandler 排行榜处理器
type RankingHandler struct {
	userRepo *database.UserRepository
	cache    *cache.Cache
}

// NewRankingHandler 创建排行榜处理器
func NewRankingHandler(userRepo *database.UserRepository, cache *cache.Cache) *RankingHandler {
	return &RankingHandler{
		userRepo: userRepo,
		cache:    cache,
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

	// 尝试从缓存读取
	cacheKey := "ranking:total:" + strconv.Itoa(limit)
	var users []models.RankingUser
	if h.cache.Get(c.Request.Context(), &users, cacheKey) {
		c.JSON(http.StatusOK, users)
		return
	}

	users, err := h.userRepo.GetTotalSolvedRanking(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取排行榜失败"})
		return
	}

	// 写入缓存，30 秒过期
	h.cache.Set(c.Request.Context(), users, 30*time.Second, cacheKey)

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

	// 尝试从缓存读取
	cacheKey := "ranking:today:" + strconv.Itoa(limit)
	var users []models.RankingUser
	if h.cache.Get(c.Request.Context(), &users, cacheKey) {
		c.JSON(http.StatusOK, users)
		return
	}

	users, err := h.userRepo.GetTodaySolvedRanking(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取排行榜失败"})
		return
	}

	// 写入缓存，15 秒过期（今日排行变化更频繁）
	h.cache.Set(c.Request.Context(), users, 15*time.Second, cacheKey)

	c.JSON(http.StatusOK, users)
}
