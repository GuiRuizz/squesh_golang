package main

import (
	"squesh_golang/internal/database"
	"squesh_golang/internal/handler"

	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func main() {
	db := database.InitDB()

	r := gin.Default()
	r.Use(CORSMiddleware())

	postHandler := handler.NewPostHandler(db)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/posts", postHandler.GetFeed)
		v1.GET("/users/:user_id/posts", postHandler.GetUserPosts)
		v1.PUT("/posts/:id", postHandler.UpdatePostCaption)
		v1.DELETE("/posts/:id", postHandler.DeletePost)
	}

	r.Run(":8080")
}