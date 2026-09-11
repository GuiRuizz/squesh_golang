package main

import (
	"squesh_golang/internal/database"
	"squesh_golang/internal/routes"
)

func main() {
	db := database.InitDB()

	r := routes.SetupRouter(db)

	r.Run("0.0.0.0:8080")
}
