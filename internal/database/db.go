package database

import (
	"fmt"
	"log"
	"os"

	"squesh_golang/internal/domain"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	host := getEnv("DB_HOST", "localhost")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgrespassword")
	dbname := getEnv("DB_NAME", "squesh_db")
	port := getEnv("DB_PORT", "5432")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=America/Sao_Paulo",
		host, user, password, dbname, port,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Erro ao conectar no banco via GORM: %v", err)
	}

	// O AutoMigrate adiciona tabelas e novas colunas (como role em User, Trail e TrailItem)
	err = db.AutoMigrate(
		&domain.User{},
		&domain.Post{},
		&domain.Comment{},
		&domain.Trail{},
		&domain.TrailItem{},
	)
	if err != nil {
		log.Fatalf("Erro ao executar AutoMigrate: %v", err)
	}

	fmt.Println("Conexão e AutoMigrate do [squesh_golang] executados com sucesso!")
	return db
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}