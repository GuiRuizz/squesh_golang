package routes

import (
	"squesh_golang/internal/handler"
	"squesh_golang/internal/middleware"
	"squesh_golang/internal/service"
	"squesh_golang/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, store storage.Storage) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	// Services
	notifService := service.NewNotificationService(db)

	// Handlers
	authHandler := handler.NewAuthHandler(db)
	postHandler := handler.NewPostHandler(db)
	trailHandler := handler.NewTrailHandler(db)
	userHandler := handler.NewUserHandler(db)
	followHandler := handler.NewFollowHandler(db)
	shopHandler := handler.NewShopHandler(db, notifService)
	billingHandler := handler.NewBillingHandler(db, notifService)
	notificationHandler := handler.NewNotificationHandler(db)
	uploadHandler := handler.NewUploadHandler(store)

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
		v1.GET("/posts/:id/comments", postHandler.ListComments)
		v1.GET("/posts/:id/likes", postHandler.GetPostLikes)

		// Perfis públicos: quem segue / quem é seguido
		v1.GET("/users/:id/followers", followHandler.ListFollowers)
		v1.GET("/users/:id/following", followHandler.ListFollowing)

		// Leitura de Trilhas (Pública)
		v1.GET("/trails", trailHandler.ListTrails)
		v1.GET("/trails/:id", trailHandler.GetTrailByID)

		// Catálogo da Loja (Leitura Pública)
		v1.GET("/shop", shopHandler.GetItems)
		v1.GET("/shop/:id", shopHandler.GetItemByID)

		// Vitrine de planos (público: o app mostra os preços antes do login)
		v1.GET("/plans", billingHandler.ListPlans)

		// Upload de arquivos: o app faz o PUT direto na URL assinada
		// (a assinatura na query string é a autenticação — sem JWT aqui).
		v1.PUT("/uploads/*key", uploadHandler.Upload)

		// Arquivos públicos (driver local — vira CDN/bucket com S3 depois)
		if dir := store.UploadDir(); dir != "" {
			v1.Static("/files", dir)
		}

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
				trails.GET("/me", trailHandler.GetMyTrails)              // <--- Novo: todas as trilhas com progresso + limite diário
			trails.GET("/me/active", trailHandler.GetMyActiveTrail)        // <--- Novo: trilha atual + próxima etapa
				trails.GET("/me/completed", trailHandler.GetMyCompletedTrails) // <--- Novo: trilhas concluídas
				trails.POST("/generate", trailHandler.GenerateCompleteTrail)   // <--- Novo: gera trilha COMPLETA
				trails.POST("/:id/generate", trailHandler.GenerateInfiniteItems)
				trails.POST("/items/:itemId/complete", trailHandler.CompleteTrailItem)  // conclui item SEM etapas internas
				trails.PATCH("/items/:itemId/steps", trailHandler.ToggleStepCheck)  // marca/desmarca UMA etapa (refeição ou exercício)
				// Alias legado: o app anterior chamava /meals (o body aceita meal_index).
				trails.PATCH("/items/:itemId/meals", trailHandler.ToggleStepCheck)
			}

			// Usuários & Profile
			users := protected.Group("/users")
			{
				users.GET("/me", userHandler.GetProfile)
				users.GET("/me/streak", userHandler.GetStreak) // <--- Novo
				users.GET("/me/posts", postHandler.GetMyPosts) // <--- tela "Meus Posts"
				users.GET("/ranking", userHandler.GetRanking)
				users.PUT("/me", userHandler.UpdateProfile)
				users.PUT("/me/preferences", userHandler.UpdatePreferences)
				users.PATCH("/me/password", userHandler.UpdatePassword)
			}

			// Assinatura, formas de pagamento e endereços
			users.GET("/me/subscription", billingHandler.GetMySubscription)
			users.POST("/me/subscription", billingHandler.Subscribe)
			users.DELETE("/me/subscription", billingHandler.CancelSubscription)
			{
				users.GET("/me/payment-methods", billingHandler.ListPaymentMethods)
				users.POST("/me/payment-methods", billingHandler.CreatePaymentMethod)
				users.PUT("/me/payment-methods/:methodId", billingHandler.UpdatePaymentMethod)
				users.DELETE("/me/payment-methods/:methodId", billingHandler.DeletePaymentMethod)

				users.GET("/me/addresses", billingHandler.ListAddresses)
				users.POST("/me/addresses", billingHandler.CreateAddress)
				users.PUT("/me/addresses/:addressId", billingHandler.UpdateAddress)
				users.DELETE("/me/addresses/:addressId", billingHandler.DeleteAddress)
			}

			// Postagens & Comentários
			posts := protected.Group("/posts")
			{
				posts.GET("/feed", postHandler.GetPersonalizedFeed) // <--- Novo: feed de quem eu sigo + meus posts
				posts.POST("", postHandler.CreatePost)
				posts.POST("/:id/comments", postHandler.CreateComment)              // <--- Novo
				posts.DELETE("/:id/comments/:commentId", postHandler.DeleteComment) // <--- Novo
				posts.POST("/:id/like", postHandler.LikePost)                       // <--- Novo
				posts.DELETE("/:id/like", postHandler.UnlikePost)                   // <--- Novo
				posts.PUT("/:id", postHandler.UpdatePostCaption)
				posts.DELETE("/:id", postHandler.DeletePost)
			}

			// Seguir / Deixar de seguir (ações autenticadas)
			users.POST("/:id/follow", followHandler.FollowUser)     // <--- Novo
			users.DELETE("/:id/follow", followHandler.UnfollowUser) // <--- Novo

			// Upload (pedido de URL assinada)
			uploads := protected.Group("/uploads")
			{
				uploads.POST("/presign", uploadHandler.PresignUpload) // <--- Novo
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
