package contracts

import (
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
)

// OrderMessage is published after order commit (orders_topic, persistent, with priority).
type OrderMessage struct {
	OrderNumber     string             `json:"order_number"`
	CustomerName    string             `json:"customer_name"`
	OrderType       orders.OrderType   `json:"order_type"`
	TableNumber     *int               `json:"table_number"`
	DeliveryAddress *string            `json:"delivery_address"`
	Items           []orders.OrderItem `json:"items"`
	TotalAmount     float64            `json:"total_amount"` // adapters convert from Money
	Priority        int                `json:"priority"`
}

// StatusUpdateMessage is fan-out notification payload.
type StatusUpdateMessage struct {
	OrderNumber         string             `json:"order_number"`
	OldStatus           orders.OrderStatus `json:"old_status"`
	NewStatus           orders.OrderStatus `json:"new_status"`
	ChangedBy           string             `json:"changed_by"`
	Timestamp           time.Time          `json:"timestamp"`
	EstimatedCompletion *time.Time         `json:"estimated_completion,omitempty"`
}
