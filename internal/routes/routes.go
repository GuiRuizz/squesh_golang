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

		// Rotas Protegidas por Login
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Rotas de Usuário
			protected.GET("/users/me", userHandler.GetProfile)
			protected.GET("/users/ranking", userHandler.GetRanking)

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
