package service

import (
	"time"

	"squesh_golang/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FulfillOrderPaid marca o pedido como pago e entrega os itens no inventário.
//
// ESTE é o ÚNICO caminho que gera inventário. Existem dois chamadores — a
// confirmação manual do admin e o webhook payment_intent.succeeded — e eles
// passam por esta função de propósito: duas implementações do mesmo passo
// divergem com o tempo (um dia uma corrige um bug de quantidade e a outra
// não), e divergir aqui significa entregar o produto duas vezes ou nunca.
//
// A trava contra o dobro é o próprio UPDATE:
//
//	UPDATE shop_orders SET status='paid' WHERE id=? AND status='pending'
//
// Se RowsAffected vier 0, outra requisição confirmou entre a leitura e o
// UPDATE, e esta devolve ErrOrderAlreadyConfirmed sem criar inventário. Duas
// confirmações simultâneas (o usuário tocou duas vezes, ou o webhook chegou
// junto com a confirmação do admin) não conseguem passar as duas.
//
// A notificação sai DEPOIS da transação, nunca dentro dela: se ela falhar,
// o pedido continua pago — que é o que importa. O inverso (notificar sem
// entregar) seria pior.
func FulfillOrderPaid(db *gorm.DB, notif *NotificationService, orderID uuid.UUID) (*domain.ShopOrder, error) {
	var order domain.ShopOrder
	if err := db.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
		return nil, err
	}
	switch {
	case order.IsPaid():
		return nil, ErrOrderAlreadyPaid
	case !order.IsPending():
		return nil, ErrOrderNotPending
	}

	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&domain.ShopOrder{}).
			Where("id = ? AND status = ?", order.ID, "pending").
			Updates(map[string]interface{}{
				"status":     "paid",
				"paid_at":    now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Outra confirmação já passou entre o SELECT e o UPDATE.
			return ErrOrderAlreadyPaid
		}
		if len(order.Items) == 0 {
			return nil
		}

		// Uma linha por unidade comprada: o inventário guarda o item, não a
		// quantidade, então "2x whey" são dois registros consumíveis.
		inventory := make([]domain.UserInventory, 0, len(order.Items))
		for _, line := range order.Items {
			for i := 0; i < line.Quantity; i++ {
				inventory = append(inventory, domain.UserInventory{
					UserID: order.UserID,
					ItemID: line.ItemID,
				})
			}
		}
		return tx.Create(&inventory).Error
	}); err != nil {
		return nil, err
	}

	order.Status = "paid"
	order.PaidAt = &now

	// Categoria "shop": se o usuário desligou as novidades da loja, o serviço
	// de notificações descarta a mensagem.
	if notif != nil {
		_ = notif.CreateNotification(order.UserID, "Pedido confirmado!",
			"Recebemos o pagamento do seu pedido. Os itens já estão no seu inventário.", "shop")
	}
	return &order, nil
}