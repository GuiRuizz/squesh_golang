package routes

import (
	"squesh_golang/internal/handler"
	"squesh_golang/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	authHandler := handler.NewAuthHandler(db)
	postHandler := handler.NewPostHandler(db)
	trailHandler := handler.NewTrailHandler(db)
	userHandler := handler.NewUserHandler(db)
	shopHandler := handler.NewShopHandler(db)

	v1 := r.Group("/api/v1")
	{
		// Rotas Públicas
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		v1.GET("/posts", postHandler.GetFeed)

		// Leitura de Trilhas (Pública para todos verem)
		v1.GET("/trails", trailHandler.ListTrails)
		v1.GET("/trails/:id", trailHandler.GetTrailByID)

		shopGroup := v1.Group("/shop")
		{
			// Rotas Públicas (Leitura do catálogo)
			shopGroup.GET("", shopHandler.GetItems)        // Listar catálogo com suporte a busca/paginação
			shopGroup.GET("/:id", shopHandler.GetItemByID) // Buscar item específico por ID

			// Rotas de Admin (Para criar, atualizar e deletar itens)
			// Se você tiver um middleware de admin, adicione aqui (ex: middleware.AdminOnly())
			shopGroup.POST("", shopHandler.CreateItem)
			shopGroup.PUT("/:id", shopHandler.UpdateItem)
			shopGroup.DELETE("/:id", shopHandler.DeleteItem)

			// Rotas Protegidas (Exigem usuário autenticado via JWT)
			protected := shopGroup.Use(middleware.AuthMiddleware())
			{
				protected.POST("/buy", shopHandler.BuyItem)               // Realizar a compra de um item
				protected.GET("/inventory", shopHandler.GetUserInventory) // Listar inventário do usuário logado
			}
		}

		// Rotas Protegidas por Login
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Rotas de Usuário
			protected.GET("/users/me", userHandler.GetProfile)
			protected.GET("/users/ranking", userHandler.GetRanking)
			protected.PUT("/users/me", userHandler.UpdateProfile)
			protected.PATCH("/users/me/password", userHandler.UpdatePassword)

			// Rotas de Postagens
			protected.POST("/posts", postHandler.CreatePost)
			protected.PUT("/posts/:id", postHandler.UpdatePostCaption)
			protected.DELETE("/posts/:id", postHandler.DeletePost)

			// Rotas Exclusivas de ADMIN
			admin := protected.Group("")
			admin.Use(middleware.AdminMiddleware())
			{
				admin.POST("/trails", trailHandler.CreateTrail)
				admin.POST("/trails/:id/items", trailHandler.AddItemToTrail)
			}
		}
	}

	return r
}
