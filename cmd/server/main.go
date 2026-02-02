package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/daiXXXXX/programming-backend/internal/config"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/handlers"
	"github.com/daiXXXXX/programming-backend/internal/middleware"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.GinMode)

	// 连接数据库
	db, err := database.Connect(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 初始化仓库
	problemRepo := database.NewProblemRepository(db)
	submissionRepo := database.NewSubmissionRepository(db)

	// 初始化评测器
	eval := evaluator.NewEvaluator(cfg.Executor.Timeout)

	// 初始化处理器
	problemHandler := handlers.NewProblemHandler(problemRepo)
	submissionHandler := handlers.NewSubmissionHandler(submissionRepo, problemRepo, eval)

	// 创建路由
	router := gin.Default()

	// 配置 CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 使用中间件
	router.Use(middleware.Logger())
	router.Use(middleware.Recovery())

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API 路由组
	api := router.Group("/api")
	{
		// 题目相关路由
		problems := api.Group("/problems")
		{
			problems.GET("", problemHandler.GetProblems)
			problems.GET("/:id", problemHandler.GetProblem)
			problems.POST("", problemHandler.CreateProblem)
			problems.PUT("/:id", problemHandler.UpdateProblem)
			problems.DELETE("/:id", problemHandler.DeleteProblem)
		}

		// 提交相关路由
		submissions := api.Group("/submissions")
		{
			submissions.POST("", submissionHandler.SubmitCode)
			submissions.GET("/:id", submissionHandler.GetSubmission)
			submissions.GET("/user/:userId", submissionHandler.GetUserSubmissions)
			submissions.GET("/problem/:problemId", submissionHandler.GetProblemSubmissions)
		}

		// 统计相关路由
		stats := api.Group("/stats")
		{
			stats.GET("/user/:userId", submissionHandler.GetUserStats)
		}
	}

	// 启动服务器
	addr := ":" + cfg.Server.Port
	log.Printf("Server starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
