package routes

import (
	"squesh_golang/internal/handler"
	"squesh_golang/internal/middleware"
	"squesh_golang/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	// Services
	notifService := service.NewNotificationService(db)

	// Handlers
	authHandler := handler.NewAuthHandler(db)
	postHandler := handler.NewPostHandler(db)
	trailHandler := handler.NewTrailHandler(db)
	userHandler := handler.NewUserHandler(db)
	shopHandler := handler.NewShopHandler(db, notifService)
	notificationHandler := handler.NewNotificationHandler(db)

	v1 := r.Group("/api/v1")
	{
		// Rotas Públicas
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		v1.GET("/posts", postHandler.GetFeed)

		// Leitura de Trilhas (Pública)
		v1.GET("/trails", trailHandler.ListTrails)
		v1.GET("/trails/:id", trailHandler.GetTrailByID)

		// Catálogo da Loja (Leitura Pública)
		v1.GET("/shop", shopHandler.GetItems)
		v1.GET("/shop/:id", shopHandler.GetItemByID)

		// Rotas Protegidas por Login
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Loja (Ações do Usuário)
			protected.POST("/shop/buy", shopHandler.BuyItem)
			protected.GET("/shop/inventory", shopHandler.GetUserInventory)

			// Central de Notificações
			notifications := protected.Group("/notifications")
			{
				notifications.GET("", notificationHandler.GetUserNotifications)
				notifications.PATCH("/:id/read", notificationHandler.MarkAsRead)
				notifications.PATCH("/read-all", notificationHandler.MarkAllAsRead)
				notifications.PATCH("/:id/unread", notificationHandler.MarkAsUnread)
			}

			// Trilhas & Progresso
			trails := protected.Group("/trails")
			{
				trails.POST("/generate", trailHandler.GenerateCompleteTrail)             // <--- Novo: gera trilha COMPLETA
				trails.POST("/:id/generate", trailHandler.GenerateInfiniteItems)
				trails.POST("/items/:itemId/complete", trailHandler.CompleteTrailItem)        // <--- Novo
				trails.PATCH("/items/:itemId/meals/:mealIndex", trailHandler.ToggleMealCheck) // <--- Novo
			}

			// Usuários & Profile
			users := protected.Group("/users")
			{
				users.GET("/me", userHandler.GetProfile)
				users.GET("/me/streak", userHandler.GetStreak) // <--- Novo
				users.GET("/ranking", userHandler.GetRanking)
				users.PUT("/me", userHandler.UpdateProfile)
				users.PATCH("/me/password", userHandler.UpdatePassword)
			}

			// Postagens
			posts := protected.Group("/posts")
			{
				posts.POST("", postHandler.CreatePost)
				posts.PUT("/:id", postHandler.UpdatePostCaption)
				posts.DELETE("/:id", postHandler.DeletePost)
			}

			// Rotas Exclusivas de ADMIN
			admin := protected.Group("")
			admin.Use(middleware.AdminMiddleware())
			{
				// Gerenciamento da Loja (Admin)
				admin.POST("/shop", shopHandler.CreateItem)
				admin.PUT("/shop/:id", shopHandler.UpdateItem)
				admin.DELETE("/shop/:id", shopHandler.DeleteItem)

				// Gerenciamento de Trilhas (Admin)
				admin.POST("/trails", trailHandler.CreateTrail)
				admin.POST("/trails/:id/items", trailHandler.AddItemToTrail)
			}
		}
	}

	return r
}
