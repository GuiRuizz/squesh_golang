// internal/domain/models.go
package domain

// GetModels retorna todas as structs de modelo para auto-migration
func GetModels() []interface{} {
	return []interface{}{
		&User{},
		&Post{},
		&Comment{},
		&Trail{},
		&TrailItem{},
		&ShopItem{},
		&UserInventory{},
	}
}