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
	notificationHandler := handler.NewNotificationHandler(db) // Instanciando o Handler

	v1 := r.Group("/api/v1")
	{
		// Rotas Públicas
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
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
				notifications.GET("", notificationHandler.GetUserNotifications)       // GET /api/v1/notifications
				notifications.PATCH("/:id/read", notificationHandler.MarkAsRead)     // PATCH /api/v1/notifications/:id/read
				notifications.PATCH("/read-all", notificationHandler.MarkAllAsRead)  // PATCH /api/v1/notifications/read-all
				notifications.PATCH("/:id/unread", notificationHandler.MarkAsUnread) // PATCH /api/v1/notifications/:id/unread
			}

			// Trilhas
			protected.POST("/trails/:id/generate", trailHandler.GenerateInfiniteItems)

			// Usuários
			protected.GET("/users/me", userHandler.GetProfile)
			protected.GET("/users/ranking", userHandler.GetRanking)
			protected.PUT("/users/me", userHandler.UpdateProfile)
			protected.PATCH("/users/me/password", userHandler.UpdatePassword)

			// Postagens
			protected.POST("/posts", postHandler.CreatePost)
			protected.PUT("/posts/:id", postHandler.UpdatePostCaption)
			protected.DELETE("/posts/:id", postHandler.DeletePost)

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