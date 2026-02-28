package main

import (
	"log"
	"strings"

	"github.com/daiXXXXX/programming-backend/internal/auth"
	"github.com/daiXXXXX/programming-backend/internal/config"
	"github.com/daiXXXXX/programming-backend/internal/database"
	"github.com/daiXXXXX/programming-backend/internal/evaluator"
	"github.com/daiXXXXX/programming-backend/internal/handlers"
	"github.com/daiXXXXX/programming-backend/internal/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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
	userRepo := database.NewUserRepository(db)

	// 初始化JWT管理器
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret)

	// 初始化评测器
	eval := evaluator.NewEvaluator(cfg.Executor.Timeout)

	// 初始化处理器
	problemHandler := handlers.NewProblemHandler(problemRepo)
	submissionHandler := handlers.NewSubmissionHandler(submissionRepo, problemRepo, eval)
	authHandler := handlers.NewAuthHandler(userRepo, jwtManager)
	rankingHandler := handlers.NewRankingHandler(userRepo)

	// 创建路由
	router := gin.Default()

	// 配置 CORS
	router.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "https://localhost:") ||
				origin == "http://localhost" ||
				origin == "https://localhost"
		},
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
		// 认证相关路由（公开）
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", authHandler.Register)
			authRoutes.POST("/login", authHandler.Login)
			authRoutes.POST("/refresh", authHandler.RefreshToken)
		}

		// 认证相关路由（需要登录）
		authProtected := api.Group("/auth")
		authProtected.Use(middleware.AuthMiddleware(jwtManager))
		{
			authProtected.GET("/me", authHandler.GetCurrentUser)
			authProtected.PUT("/password", authHandler.ChangePassword)
		}

		// 题目相关路由（公开读取）
		problems := api.Group("/problems")
		problems.Use(middleware.OptionalAuthMiddleware(jwtManager))
		{
			problems.GET("", problemHandler.GetProblems)
			problems.GET("/:id", problemHandler.GetProblem)
		}

		// 题目管理路由（需要教师权限）
		problemsAdmin := api.Group("/problems")
		problemsAdmin.Use(middleware.AuthMiddleware(jwtManager))
		problemsAdmin.Use(middleware.InstructorOnly())
		{
			problemsAdmin.POST("", problemHandler.CreateProblem)
			problemsAdmin.PUT("/:id", problemHandler.UpdateProblem)
			problemsAdmin.DELETE("/:id", problemHandler.DeleteProblem)
		}

		// 提交相关路由（需要登录）
		submissions := api.Group("/submissions")
		submissions.Use(middleware.AuthMiddleware(jwtManager))
		{
			submissions.POST("", submissionHandler.SubmitCode)
			submissions.GET("/:id", submissionHandler.GetSubmission)
			submissions.GET("/user/:userId", submissionHandler.GetUserSubmissions)
			submissions.GET("/problem/:problemId", submissionHandler.GetProblemSubmissions)
		}

		// 统计相关路由（需要登录）
		stats := api.Group("/stats")
		stats.Use(middleware.AuthMiddleware(jwtManager))
		{
			stats.GET("/user/:userId", submissionHandler.GetUserStats)
		}

		// 排行榜路由（公开）
		ranking := api.Group("/ranking")
		{
			ranking.GET("/total", rankingHandler.GetTotalSolvedRanking)
			ranking.GET("/today", rankingHandler.GetTodaySolvedRanking)
		}
	}

	// 启动服务器
	addr := ":" + cfg.Server.Port
	log.Printf("Server starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
