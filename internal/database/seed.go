package database

import (
	"log"

	"squesh_golang/internal/domain"

	"gorm.io/gorm"
)

// seedPlans garante que o catálogo de planos exista. É idempotente: procura
// cada plano pelo slug e só cria o que faltar, então pode rodar em toda
// inicialização sem duplicar nem sobrescrever o que o admin ajustou.
func seedPlans(db *gorm.DB) {
	plans := []domain.Plan{
		{
			Name:         "Mensal",
			Slug:         "mensal",
			Description:  "Acesso Pro por 1 mes, sem permanencia.",
			PriceCents:   2990,
			PeriodMonths: 1,
			Features: []string{
				"Todas as trilhas de treino e nutricao",
				"Sem anuncios na loja",
				"Badge PRO no perfil",
			},
			SortOrder: 1,
			IsActive:  true,
		},
		{
			Name:         "Trimestral",
			Slug:         "trimestral",
			Description:  "3 meses de Pro com 10% de desconto.",
			PriceCents:   8090,
			PeriodMonths: 3,
			Badge:        "MAIS ESCOLHIDO",
			Features: []string{
				"Tudo do plano Mensal",
				"10% de desconto no periodo",
				"Trilhas exclusivas de hipertrofia",
			},
			Highlight:  "Melhor custo-beneficio",
			IsPopular:  true,
			SortOrder:  2,
			IsActive:   true,
		},
		{
			Name:         "Anual",
			Slug:         "anual",
			Description:  "12 meses de Pro com 28% de desconto.",
			PriceCents:   25990,
			PeriodMonths: 12,
			Badge:        "MELHOR PRECO",
			Features: []string{
				"Tudo do plano Trimestral",
				"28% de desconto no periodo",
				"Consulta de plano de treino personalizada",
			},
			Highlight: " Economize R$ 90 por ano",
			SortOrder: 3,
			IsActive:  true,
		},
	}

	for _, plan := range plans {
		var existing domain.Plan
		err := db.Where("slug = ?", plan.Slug).First(&existing).Error
		if err == nil {
			continue // ja existe: nao sobrescreve mudanca do admin
		}
		if err != gorm.ErrRecordNotFound {
			log.Printf("Aviso: nao foi possivel verificar o plano %q: %v", plan.Slug, err)
			continue
		}
		if err := db.Create(&plan).Error; err != nil {
			log.Printf("Aviso: nao foi possivel criar o plano %q: %v", plan.Slug, err)
		}
	}
}
