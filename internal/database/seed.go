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
			Highlight: "Melhor custo-beneficio",
			IsPopular: true,
			SortOrder: 2,
			IsActive:  true,
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

// seedShopItems garante que a loja tenha o catálogo inicial.
//
// Antes esses itens viviam só como mock dentro do app Flutter (uma lista
// hardcoded). Com o catálogo no banco, o app passa a ler de verdade — e este
// seed é o que mantém a loja com conteúdo em qualquer ambiente novo.
//
// Idempotente como o seedPlans: procura pelo nome e não sobrescreve o que o
// admin ajustou (preço, imagem, categoria).
func seedShopItems(db *gorm.DB) {
	items := []domain.ShopItem{
		{
			Name:        "Whey Protein Concentrado 1kg",
			Description: "Proteína do soro do leite, 24 g de proteína por dose. Sabor chocolate.",
			PriceCents:  11990,
			Category:    "Suplementos",
			Rating:      4.9,
			ImageURL:    "https://images.unsplash.com/photo-1579722821273-0f6c7d44362f?q=80&w=600&auto=format&fit=crop",
		},
		{
			Name:        "Creatina Monohidratada 300g",
			Description: "Creatina pura, sem adoçantes. O complemento mais bem indicado para força e recuperação.",
			PriceCents:  8990,
			Category:    "Suplementos",
			Rating:      4.8,
			ImageURL:    "https://images.unsplash.com/photo-1593095948071-474c5cc2989d?q=80&w=600&auto=format&fit=crop",
		},
		{
			Name:        "Camiseta Oversized Performance",
			Description: "Modelagem ampla em tecido dry fit, ideal para o treino e para sair.",
			PriceCents:  7990,
			Category:    "Roupas",
			Rating:      4.7,
			ImageURL:    "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?q=80&w=600&auto=format&fit=crop",
		},
		{
			Name:        "Strap de Puxada de Couro",
			Description: "Alça de couro com gancho metálico para puxada e costas.",
			PriceCents:  4500,
			Category:    "Acessórios",
			Rating:      4.9,
			ImageURL:    "https://images.unsplash.com/photo-1517838277536-f5f99be501cd?q=80&w=600&auto=format&fit=crop",
		},
	}

	for _, item := range items {
		var existing domain.ShopItem
		err := db.Where("name = ?", item.Name).First(&existing).Error
		if err == nil {
			continue // ja existe: nao sobrescreve ajuste do admin
		}
		if err != gorm.ErrRecordNotFound {
			log.Printf("Aviso: nao foi possivel verificar o item %q: %v", item.Name, err)
			continue
		}
		item.IsActive = true
		if err := db.Create(&item).Error; err != nil {
			log.Printf("Aviso: nao foi possivel criar o item %q: %v", item.Name, err)
		}
	}
}
