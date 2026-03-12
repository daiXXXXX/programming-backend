package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
)

// SolutionRepository 题解数据仓库
type SolutionRepository struct {
	db *sql.DB
}

// NewSolutionRepository 创建题解仓库
func NewSolutionRepository(db *sql.DB) *SolutionRepository {
	return &SolutionRepository{db: db}
}

// Create 创建题解
func (r *SolutionRepository) Create(solution *models.Solution) error {
	query := `INSERT INTO solutions (problem_id, user_id, title, content) VALUES (?, ?, ?, ?)`
	result, err := r.db.Exec(query, solution.ProblemID, solution.UserID, solution.Title, solution.Content)
	if err != nil {
		return fmt.Errorf("failed to create solution: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	solution.ID = id
	solution.CreatedAt = time.Now()
	solution.UpdatedAt = time.Now()
	return nil
}

// GetByID 根据ID获取题解（含作者信息）
func (r *SolutionRepository) GetByID(id int64, currentUserID int64) (*models.Solution, error) {
	query := `
		SELECT s.id, s.problem_id, s.user_id, s.title, s.content,
			   s.view_count, s.like_count, s.comment_count,
			   s.created_at, s.updated_at,
			   u.id, u.username, u.avatar,
			   CASE WHEN sl.id IS NOT NULL THEN 1 ELSE 0 END as liked
		FROM solutions s
		JOIN users u ON s.user_id = u.id
		LEFT JOIN solution_likes sl ON sl.solution_id = s.id AND sl.user_id = ?
		WHERE s.id = ?
	`
	var sol models.Solution
	var author models.SolutionAuthor
	var liked int

	err := r.db.QueryRow(query, currentUserID, id).Scan(
		&sol.ID, &sol.ProblemID, &sol.UserID, &sol.Title, &sol.Content,
		&sol.ViewCount, &sol.LikeCount, &sol.CommentCount,
		&sol.CreatedAt, &sol.UpdatedAt,
		&author.ID, &author.Username, &author.Avatar,
		&liked,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("solution not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get solution: %w", err)
	}

	sol.Author = &author
	sol.Liked = liked == 1
	return &sol, nil
}

// ListByProblem 获取某题目的题解列表
func (r *SolutionRepository) ListByProblem(problemID int64, currentUserID int64, orderBy string, limit, offset int) ([]models.Solution, int, error) {
	// 先获取总数
	countQuery := `SELECT COUNT(*) FROM solutions WHERE problem_id = ?`
	var total int
	if err := r.db.QueryRow(countQuery, problemID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count solutions: %w", err)
	}

	// 排序方式
	order := "s.created_at DESC"
	switch orderBy {
	case "likes":
		order = "s.like_count DESC, s.created_at DESC"
	case "views":
		order = "s.view_count DESC, s.created_at DESC"
	case "newest":
		order = "s.created_at DESC"
	case "oldest":
		order = "s.created_at ASC"
	}

	query := fmt.Sprintf(`
		SELECT s.id, s.problem_id, s.user_id, s.title, s.content,
			   s.view_count, s.like_count, s.comment_count,
			   s.created_at, s.updated_at,
			   u.id, u.username, u.avatar,
			   CASE WHEN sl.id IS NOT NULL THEN 1 ELSE 0 END as liked
		FROM solutions s
		JOIN users u ON s.user_id = u.id
		LEFT JOIN solution_likes sl ON sl.solution_id = s.id AND sl.user_id = ?
		WHERE s.problem_id = ?
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, order)

	rows, err := r.db.Query(query, currentUserID, problemID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list solutions: %w", err)
	}
	defer rows.Close()

	var solutions []models.Solution
	for rows.Next() {
		var sol models.Solution
		var author models.SolutionAuthor
		var liked int

		err := rows.Scan(
			&sol.ID, &sol.ProblemID, &sol.UserID, &sol.Title, &sol.Content,
			&sol.ViewCount, &sol.LikeCount, &sol.CommentCount,
			&sol.CreatedAt, &sol.UpdatedAt,
			&author.ID, &author.Username, &author.Avatar,
			&liked,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan solution: %w", err)
		}

		sol.Author = &author
		sol.Liked = liked == 1
		solutions = append(solutions, sol)
	}
	return solutions, total, nil
}

// Update 更新题解（仅作者可更新）
func (r *SolutionRepository) Update(id int64, userID int64, title, content string) error {
	query := `UPDATE solutions SET title = ?, content = ? WHERE id = ? AND user_id = ?`
	result, err := r.db.Exec(query, title, content, id, userID)
	if err != nil {
		return fmt.Errorf("failed to update solution: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("solution not found or permission denied")
	}
	return nil
}

// Delete 删除题解（作者或管理员可删除）
func (r *SolutionRepository) Delete(id int64, userID int64, isAdmin bool) error {
	var query string
	var err error
	if isAdmin {
		query = `DELETE FROM solutions WHERE id = ?`
		_, err = r.db.Exec(query, id)
	} else {
		query = `DELETE FROM solutions WHERE id = ? AND user_id = ?`
		_, err = r.db.Exec(query, id, userID)
	}
	if err != nil {
		return fmt.Errorf("failed to delete solution: %w", err)
	}
	return nil
}

// IncrementViewCount 增加浏览量
func (r *SolutionRepository) IncrementViewCount(id int64) error {
	_, err := r.db.Exec(`UPDATE solutions SET view_count = view_count + 1 WHERE id = ?`, id)
	return err
}

// ToggleLike 切换点赞状态，返回新的点赞状态和当前总赞数
func (r *SolutionRepository) ToggleLike(solutionID, userID int64) (bool, int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 检查是否已点赞
	var existingID int64
	err = tx.QueryRow(`SELECT id FROM solution_likes WHERE solution_id = ? AND user_id = ?`, solutionID, userID).Scan(&existingID)

	if err == sql.ErrNoRows {
		// 未点赞，添加点赞
		_, err = tx.Exec(`INSERT INTO solution_likes (solution_id, user_id) VALUES (?, ?)`, solutionID, userID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to add like: %w", err)
		}
		_, err = tx.Exec(`UPDATE solutions SET like_count = like_count + 1 WHERE id = ?`, solutionID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to update like count: %w", err)
		}
	} else if err != nil {
		return false, 0, fmt.Errorf("failed to check like: %w", err)
	} else {
		// 已点赞，取消点赞
		_, err = tx.Exec(`DELETE FROM solution_likes WHERE id = ?`, existingID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to remove like: %w", err)
		}
		_, err = tx.Exec(`UPDATE solutions SET like_count = GREATEST(like_count - 1, 0) WHERE id = ?`, solutionID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to update like count: %w", err)
		}
	}

	// 获取最新的赞数
	var likeCount int
	err = tx.QueryRow(`SELECT like_count FROM solutions WHERE id = ?`, solutionID).Scan(&likeCount)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get like count: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return false, 0, fmt.Errorf("failed to commit: %w", err)
	}

	liked := existingID == 0 // 如果之前不存在，说明现在已点赞
	return liked, likeCount, nil
}

// CreateComment 创建评论
func (r *SolutionRepository) CreateComment(comment *models.SolutionComment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO solution_comments (solution_id, user_id, parent_id, content) VALUES (?, ?, ?, ?)`
	result, err := tx.Exec(query, comment.SolutionID, comment.UserID, comment.ParentID, comment.Content)
	if err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	comment.ID = id
	comment.CreatedAt = time.Now()
	comment.UpdatedAt = time.Now()

	// 更新评论计数
	_, err = tx.Exec(`UPDATE solutions SET comment_count = comment_count + 1 WHERE id = ?`, comment.SolutionID)
	if err != nil {
		return fmt.Errorf("failed to update comment count: %w", err)
	}

	return tx.Commit()
}

// GetComments 获取题解的评论列表（嵌套结构）
func (r *SolutionRepository) GetComments(solutionID int64, limit, offset int) ([]models.SolutionComment, int, error) {
	// 获取顶级评论总数
	countQuery := `SELECT COUNT(*) FROM solution_comments WHERE solution_id = ? AND parent_id IS NULL`
	var total int
	if err := r.db.QueryRow(countQuery, solutionID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}

	// 获取所有评论（包括回复），一次性加载后在内存中组装嵌套结构
	query := `
		SELECT c.id, c.solution_id, c.user_id, c.parent_id, c.content,
			   c.like_count, c.created_at, c.updated_at,
			   u.id, u.username, u.avatar
		FROM solution_comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.solution_id = ?
		ORDER BY c.created_at ASC
	`
	rows, err := r.db.Query(query, solutionID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get comments: %w", err)
	}
	defer rows.Close()

	// 先收集所有评论
	allComments := make(map[int64]*models.SolutionComment)
	var topLevelIDs []int64

	for rows.Next() {
		var c models.SolutionComment
		var author models.SolutionAuthor
		var parentID sql.NullInt64

		err := rows.Scan(
			&c.ID, &c.SolutionID, &c.UserID, &parentID, &c.Content,
			&c.LikeCount, &c.CreatedAt, &c.UpdatedAt,
			&author.ID, &author.Username, &author.Avatar,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan comment: %w", err)
		}

		c.Author = &author
		if parentID.Valid {
			pid := parentID.Int64
			c.ParentID = &pid
		}

		allComments[c.ID] = &c
		if c.ParentID == nil {
			topLevelIDs = append(topLevelIDs, c.ID)
		}
	}

	// 构建嵌套结构
	for _, c := range allComments {
		if c.ParentID != nil {
			if parent, ok := allComments[*c.ParentID]; ok {
				parent.Replies = append(parent.Replies, *c)
			}
		}
	}

	// 分页顶级评论
	start := offset
	end := offset + limit
	if start > len(topLevelIDs) {
		start = len(topLevelIDs)
	}
	if end > len(topLevelIDs) {
		end = len(topLevelIDs)
	}

	var result []models.SolutionComment
	for _, id := range topLevelIDs[start:end] {
		if c, ok := allComments[id]; ok {
			result = append(result, *c)
		}
	}

	return result, total, nil
}

// DeleteComment 删除评论
func (r *SolutionRepository) DeleteComment(commentID, userID int64, isAdmin bool) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 获取评论的solutionID 和 ownerID
	var solutionID int64
	var ownerID int64
	err = tx.QueryRow(`SELECT solution_id, user_id FROM solution_comments WHERE id = ?`, commentID).Scan(&solutionID, &ownerID)
	if err != nil {
		return fmt.Errorf("comment not found: %w", err)
	}

	if !isAdmin && ownerID != userID {
		return fmt.Errorf("permission denied")
	}

	// 统计将被级联删除的评论数（自身 + 所有子孙评论）
	var deleteCount int
	err = tx.QueryRow(`
		WITH RECURSIVE descendants AS (
			SELECT id FROM solution_comments WHERE id = ?
			UNION ALL
			SELECT c.id FROM solution_comments c INNER JOIN descendants d ON c.parent_id = d.id
		)
		SELECT COUNT(*) FROM descendants
	`, commentID).Scan(&deleteCount)
	if err != nil {
		deleteCount = 1 // fallback
	}

	// 删除评论（级联删除子评论由外键处理）
	_, err = tx.Exec(`DELETE FROM solution_comments WHERE id = ?`, commentID)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	// 更新评论计数（减去被删除的所有评论数）
	_, err = tx.Exec(`UPDATE solutions SET comment_count = GREATEST(comment_count - ?, 0) WHERE id = ?`, deleteCount, solutionID)
	if err != nil {
		return fmt.Errorf("failed to update comment count: %w", err)
	}

	return tx.Commit()
}

// GetSolutionAuthorID 获取题解作者ID（用于通知）
func (r *SolutionRepository) GetSolutionAuthorID(solutionID int64) (int64, error) {
	var authorID int64
	err := r.db.QueryRow(`SELECT user_id FROM solutions WHERE id = ?`, solutionID).Scan(&authorID)
	return authorID, err
}
