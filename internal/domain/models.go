// internal/domain/models.go
package domain

// GetModels retorna todas as structs de modelo para auto-migration
func GetModels() []interface{} {
	return []interface{}{
		&User{},
		&RefreshToken{},
		&Post{},
		&Comment{},
		&PostLike{},
		&Follow{},
		&Trail{},
		&TrailItem{},
		&UserTrailProgress{},
		&ShopItem{},
		&ShopOrder{},
		&ShopOrderItem{},
		&UserInventory{},
		&Notification{},
		&Plan{},
		&UserSubscription{},
		&PaymentMethod{},
		&Address{},
		&XPEvent{},
	}
}
