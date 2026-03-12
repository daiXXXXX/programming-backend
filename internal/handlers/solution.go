package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/daiXXXXX/programming-backend/internal/ws"
	"github.com/gin-gonic/gin"
)

// SolutionHandler 题解处理器
type SolutionHandler struct {
	repo *database.SolutionRepository
	hub  *ws.Hub
}

// NewSolutionHandler 创建题解处理器
func NewSolutionHandler(repo *database.SolutionRepository, hub *ws.Hub) *SolutionHandler {
	return &SolutionHandler{repo: repo, hub: hub}
}

// GetSolutions 获取某题目的题解列表
// GET /api/solutions/problem/:problemId?order=newest&limit=20&offset=0
func (h *SolutionHandler) GetSolutions(c *gin.Context) {
	problemIDStr := c.Param("problemId")
	problemID, err := strconv.ParseInt(problemIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid problem ID"})
		return
	}

	orderBy := c.DefaultQuery("order", "newest")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// 获取当前用户ID（可选认证）
	var currentUserID int64
	if uid, exists := c.Get("userID"); exists {
		currentUserID = uid.(int64)
	}

	solutions, total, err := h.repo.ListByProblem(problemID, currentUserID, orderBy, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch solutions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"solutions": solutions,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetSolution 获取题解详情
// GET /api/solutions/:id
func (h *SolutionHandler) GetSolution(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	var currentUserID int64
	if uid, exists := c.Get("userID"); exists {
		currentUserID = uid.(int64)
	}

	solution, err := h.repo.GetByID(id, currentUserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Solution not found"})
		return
	}

	// 增加浏览量
	go h.repo.IncrementViewCount(id)

	c.JSON(http.StatusOK, solution)
}

// CreateSolution 创建题解
// POST /api/solutions
func (h *SolutionHandler) CreateSolution(c *gin.Context) {
	var req models.CreateSolutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID := c.GetInt64("userID")
	username, _ := c.Get("username")

	solution := &models.Solution{
		ProblemID: req.ProblemID,
		UserID:    userID,
		Title:     req.Title,
		Content:   req.Content,
	}

	if err := h.repo.Create(solution); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create solution"})
		return
	}

	// 通过 WS 通知该题目频道有新题解
	channel := fmt.Sprintf("problem:%d", req.ProblemID)
	wsMsg := &models.WSMessage{
		Type:    models.WSTypeNewSolution,
		Channel: channel,
		From: &models.SolutionAuthor{
			ID:       userID,
			Username: username.(string),
		},
		Content: map[string]interface{}{
			"solutionId": solution.ID,
			"title":      solution.Title,
			"problemId":  solution.ProblemID,
		},
		Timestamp: time.Now(),
	}
	go h.hub.BroadcastToChannel(channel, wsMsg)

	c.JSON(http.StatusCreated, solution)
}

// UpdateSolution 更新题解
// PUT /api/solutions/:id
func (h *SolutionHandler) UpdateSolution(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	var req models.UpdateSolutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID := c.GetInt64("userID")

	if err := h.repo.Update(id, userID, req.Title, req.Content); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Solution updated"})
}

// DeleteSolution 删除题解
// DELETE /api/solutions/:id
func (h *SolutionHandler) DeleteSolution(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	userID := c.GetInt64("userID")
	role, _ := c.Get("role")
	isAdmin := fmt.Sprintf("%v", role) == "admin"

	if err := h.repo.Delete(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Solution deleted"})
}

// ToggleLike 点赞/取消点赞
// POST /api/solutions/:id/like
func (h *SolutionHandler) ToggleLike(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	userID := c.GetInt64("userID")
	username, _ := c.Get("username")

	liked, likeCount, err := h.repo.ToggleLike(id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle like"})
		return
	}

	// 如果是点赞（而不是取消），通知题解作者
	if liked {
		authorID, _ := h.repo.GetSolutionAuthorID(id)
		if authorID != 0 && authorID != userID {
			wsMsg := &models.WSMessage{
				Type: models.WSTypeLikeNotify,
				From: &models.SolutionAuthor{
					ID:       userID,
					Username: username.(string),
				},
				Content: map[string]interface{}{
					"solutionId": id,
					"likeCount":  likeCount,
				},
				Timestamp: time.Now(),
			}
			go h.hub.SendToUser(authorID, wsMsg)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"liked":     liked,
		"likeCount": likeCount,
	})
}

// GetComments 获取评论列表
// GET /api/solutions/:id/comments?limit=20&offset=0
func (h *SolutionHandler) GetComments(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	comments, total, err := h.repo.GetComments(id, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comments": comments,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// CreateComment 创建评论
// POST /api/solutions/:id/comments
func (h *SolutionHandler) CreateComment(c *gin.Context) {
	idStr := c.Param("id")
	solutionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid solution ID"})
		return
	}

	var req models.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID := c.GetInt64("userID")
	username, _ := c.Get("username")
	avatar := ""
	if a, exists := c.Get("avatar"); exists {
		avatar = a.(string)
	}

	comment := &models.SolutionComment{
		SolutionID: solutionID,
		UserID:     userID,
		ParentID:   req.ParentID,
		Content:    req.Content,
	}

	if err := h.repo.CreateComment(comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	// 填充作者信息到返回
	comment.Author = &models.SolutionAuthor{
		ID:       userID,
		Username: username.(string),
		Avatar:   avatar,
	}

	// 通过 WS 通知该题解频道有新评论
	channel := fmt.Sprintf("solution:%d", solutionID)
	wsMsg := &models.WSMessage{
		Type:    models.WSTypeNewComment,
		Channel: channel,
		From:    comment.Author,
		Content: map[string]interface{}{
			"commentId":  comment.ID,
			"solutionId": solutionID,
			"content":    comment.Content,
			"parentId":   comment.ParentID,
		},
		Timestamp: time.Now(),
	}
	go h.hub.BroadcastToChannel(channel, wsMsg)

	// 通知题解作者有新评论（如果不是自己评论自己的）
	authorID, _ := h.repo.GetSolutionAuthorID(solutionID)
	if authorID != 0 && authorID != userID {
		notifyMsg := &models.WSMessage{
			Type: models.WSTypeSystemNotice,
			From: comment.Author,
			Content: map[string]interface{}{
				"message":    fmt.Sprintf("%s 评论了你的题解", username),
				"solutionId": solutionID,
				"commentId":  comment.ID,
			},
			Timestamp: time.Now(),
		}
		go h.hub.SendToUser(authorID, notifyMsg)
	}

	c.JSON(http.StatusCreated, comment)
}

// DeleteComment 删除评论
// DELETE /api/solutions/comments/:commentId
func (h *SolutionHandler) DeleteComment(c *gin.Context) {
	commentIDStr := c.Param("commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	userID := c.GetInt64("userID")
	role, _ := c.Get("role")
	isAdmin := fmt.Sprintf("%v", role) == "admin"

	if err := h.repo.DeleteComment(commentID, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
}

// HandleWebSocket 处理 WebSocket 连接
// GET /api/ws?token=xxx
func (h *SolutionHandler) HandleWebSocket(c *gin.Context) {
	userID := c.GetInt64("userID")
	username := ""
	avatar := ""
	if u, exists := c.Get("username"); exists {
		username = u.(string)
	}
	if a, exists := c.Get("avatar"); exists {
		avatar = a.(string)
	}

	ws.ServeWS(h.hub, c.Writer, c.Request, userID, username, avatar)
}
