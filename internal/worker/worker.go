package worker

import (
	"context"
	"log"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/cache"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/daiXXXXX/programming-backend/internal/ws"
)

const (
	// QueueKey Redis 队列 key（不含前缀，Cache 会自动加）
	QueueKey = "queue:judge"
)

// JudgeTask 评测任务（入队/出队时序列化的结构）
type JudgeTask struct {
	SubmissionID int64  `json:"submissionId"`
	ProblemID    int64  `json:"problemId"`
	UserID       int64  `json:"userId"`
	Code         string `json:"code"`
	Language     string `json:"language"`
}

// JudgeWorker 评测消费者
type JudgeWorker struct {
	cache          *cache.Cache
	submissionRepo *database.SubmissionRepository
	problemRepo    *database.ProblemRepository
	evaluator      *evaluator.Evaluator
	wsHub          *ws.Hub
	concurrency    int
	stopCh         chan struct{}
}

// NewJudgeWorker 创建评测消费者
func NewJudgeWorker(
	cache *cache.Cache,
	submissionRepo *database.SubmissionRepository,
	problemRepo *database.ProblemRepository,
	eval *evaluator.Evaluator,
	wsHub *ws.Hub,
	concurrency int,
) *JudgeWorker {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &JudgeWorker{
		cache:          cache,
		submissionRepo: submissionRepo,
		problemRepo:    problemRepo,
		evaluator:      eval,
		wsHub:          wsHub,
		concurrency:    concurrency,
		stopCh:         make(chan struct{}),
	}
}

// Start 启动 worker（多个 goroutine 并发消费）
func (w *JudgeWorker) Start() {
	log.Printf("[Worker] 启动评测消费者，并发数: %d", w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		go w.consume(i)
	}
}

// Stop 停止 worker
func (w *JudgeWorker) Stop() {
	close(w.stopCh)
	log.Println("[Worker] 评测消费者已停止")
}

// consume 单个消费者循环
func (w *JudgeWorker) consume(id int) {
	log.Printf("[Worker-%d] 消费者已启动，等待任务...", id)
	for {
		select {
		case <-w.stopCh:
			log.Printf("[Worker-%d] 收到停止信号，退出", id)
			return
		default:
			w.processOne(id)
		}
	}
}

// processOne 从队列取一个任务并处理
func (w *JudgeWorker) processOne(workerID int) {
	ctx := context.Background()

	var task JudgeTask
	// 阻塞等待 2 秒，超时后回到 select 检查 stopCh
	err := w.cache.QueuePop(ctx, &task, 2*time.Second, QueueKey)
	if err != nil {
		// 超时或空队列，正常情况，不打日志
		return
	}

	log.Printf("[Worker-%d] 开始评测: submission=%d, problem=%d, user=%d",
		workerID, task.SubmissionID, task.ProblemID, task.UserID)

	startTime := time.Now()

	// 1. 获取题目测试用例
	problem, err := w.problemRepo.GetByID(task.ProblemID)
	if err != nil {
		log.Printf("[Worker-%d] 获取题目失败: %v", workerID, err)
		w.submissionRepo.UpdateResult(task.SubmissionID, models.StatusRuntimeError, 0, nil)
		w.notifyUser(task.UserID, task.SubmissionID, string(models.StatusRuntimeError), 0)
		return
	}

	// 2. 执行评测
	testResults := w.evaluator.EvaluateCode(task.Code, task.Language, problem.TestCases)
	score := w.evaluator.CalculateScore(testResults)
	status := w.evaluator.GetSubmissionStatus(testResults)

	// 3. 更新数据库
	if err := w.submissionRepo.UpdateResult(task.SubmissionID, status, score, testResults); err != nil {
		log.Printf("[Worker-%d] 更新评测结果失败: %v", workerID, err)
		return
	}

	elapsed := time.Since(startTime)
	log.Printf("[Worker-%d] 评测完成: submission=%d, status=%s, score=%d, 耗时=%v",
		workerID, task.SubmissionID, status, score, elapsed)

	// 4. 通过 WebSocket 通知用户
	w.notifyUser(task.UserID, task.SubmissionID, string(status), score)
}

// notifyUser 通过 WebSocket 推送评测结果给用户
func (w *JudgeWorker) notifyUser(userID int64, submissionID int64, status string, score int) {
	if w.wsHub == nil {
		return
	}

	msg := &models.WSMessage{
		Type: models.WSTypeJudgeResult,
		Content: map[string]interface{}{
			"submissionId": submissionID,
			"status":       status,
			"score":        score,
		},
		Timestamp: time.Now(),
	}
	w.wsHub.SendToUser(userID, msg)
}
