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

	// Migrações manuais ANTES do AutoMigrate (idempotentes): o modelo
	// padronizou "meal" para "step" (uma etapa pode ser um dia de alimentação
	// ou uma sessão de treino), então renomeamos as colunas sem perder dado.
	// Se a coluna antiga não existir (banco novo), não há nada a fazer.
	renames := []struct{ table, from, to string }{
		{"trail_items", "meals", "steps"},
		{"user_trail_progresses", "meal_index", "step_index"},
	}
	for _, r := range renames {
		stmt := fmt.Sprintf(`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = '%s' AND column_name = '%s'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = '%s' AND column_name = '%s'
			) THEN
				ALTER TABLE %s RENAME COLUMN %s TO %s;
			END IF;
		END $$;`, r.table, r.from, r.table, r.to, r.table, r.from, r.to)
		if err := db.Exec(stmt).Error; err != nil {
			log.Fatalf("Erro ao renomear %s.%s para %s: %v", r.table, r.from, r.to, err)
		}
	}

	// O AutoMigrate adiciona tabelas e novas colunas (como role em User, Trail e TrailItem)
	err = db.AutoMigrate(domain.GetModels()...)
	if err != nil {
		log.Fatalf("Erro ao executar AutoMigrate: %v", err)
	}

	// Limpeza de índices únicos legados do progresso: ele era único por
	// (user_id, trail_item_id), o que bloqueava a 2ª etapa do dia/da sessão.
	// O AutoMigrate cria o novo (idx_user_item_step), mas não remove os antigos.
	for _, legacy := range []string{"idx_user_item", "idx_user_item_meal"} {
		if err := db.Exec("DROP INDEX IF EXISTS " + legacy).Error; err != nil {
			log.Fatalf("Erro ao remover o índice legado %s: %v", legacy, err)
		}
	}

	fmt.Println("Conexão e AutoMigrate do [squesh_golang] executados com sucesso!")

	// shop_items mudou de price (reais em float) para price_cents (centavos),
	// o mesmo formato dos planos. O AutoMigrate criou a coluna nova, então
	// agora é só converter o que existia e descartar a antiga — o app já só
	// lê price_cents.
	if err := db.Exec(`DO $$
	BEGIN
		IF EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'shop_items' AND column_name = 'price'
		) THEN
			UPDATE shop_items
			SET price_cents = ROUND(price * 100)::int
			WHERE price_cents = 0 AND price IS NOT NULL;
			ALTER TABLE shop_items DROP COLUMN price;
		END IF;
	END $$;`).Error; err != nil {
		log.Fatalf("Erro ao migrar shop_items.price para price_cents: %v", err)
	}

	// Catálogo: idempotente, só cria o que falta.
	seedPlans(db)
	seedShopItems(db)

	return db
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
