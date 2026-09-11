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

	postHandler := handler.NewPostHandler(db)
	userHandler := handler.NewUserHandler(db)
	authHandler := handler.NewAuthHandler(db)

	v1 := r.Group("/api/v1")
	{
		// Rotas Públicas
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}
		v1.POST("/users", userHandler.CreateUser)
		v1.GET("/posts", postHandler.GetFeed)

		// Rotas Protegidas por JWT
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			protected.POST("/posts", postHandler.CreatePost)
			protected.PUT("/posts/:id", postHandler.UpdatePostCaption)
			protected.DELETE("/posts/:id", postHandler.DeletePost)
			protected.GET("/users/:user_id/posts", postHandler.GetUserPosts)
		}
	}

	return r
}