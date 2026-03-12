package router

import (
	"admin-system/handlers"
	"admin-system/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupRouter 设置路由
func SetupRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()

	// 配置CORS
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	r.Use(cors.New(config))

	// 静态文件服务（用于访问上传的图片）
	r.Use(static.Serve("/uploads", static.LocalFile("./uploads", false)))

	// 初始化处理器
	authHandler := handlers.NewAuthHandler(db)
	userHandler := handlers.NewUserHandler(db)
	feedbackHandler := handlers.NewFeedbackHandler(db)
	pageStatHandler := handlers.NewPageStatHandler(db)
	uploadHandler := handlers.NewUploadHandler("./uploads")

	// API路由组
	api := r.Group("/api")
	{
		// 认证相关（不需要token）
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/logout", authHandler.Logout)
		}

		// 需要认证的路由
		api.Use(middleware.AuthMiddleware())
		{
			// 获取当前用户信息
			api.GET("/me", authHandler.GetCurrentUser)

			// 文件上传
			api.POST("/upload", uploadHandler.UploadImage)

			// 用户管理
			users := api.Group("/users")
			{
				users.POST("", userHandler.CreateUser)
				users.GET("", userHandler.GetUsers)
				users.GET("/:id", userHandler.GetUser)
				users.PUT("/:id", userHandler.UpdateUser)
				users.DELETE("/:id", userHandler.DeleteUser)
			}

			// 意见反馈管理
			feedbacks := api.Group("/feedbacks")
			{
				feedbacks.POST("", feedbackHandler.CreateFeedback)
				feedbacks.GET("", feedbackHandler.GetFeedbacks)
				feedbacks.GET("/:id", feedbackHandler.GetFeedback)
				feedbacks.PUT("/:id", feedbackHandler.UpdateFeedback)
				feedbacks.DELETE("/:id", feedbackHandler.DeleteFeedback)
			}

			// 页面统计管理
			pageStats := api.Group("/page-stats")
			{
				pageStats.POST("", pageStatHandler.CreatePageStat)
				pageStats.GET("", pageStatHandler.GetPageStats)
				pageStats.GET("/:id", pageStatHandler.GetPageStat)
				pageStats.PUT("/:id", pageStatHandler.UpdatePageStat)
				pageStats.DELETE("/:id", pageStatHandler.DeletePageStat)
			}
		}
	}

	return r
}
