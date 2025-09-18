package contracts

import "time"

// OrderItemMessage is the wire-format for a single item in an order message.
type OrderItemMessage struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"` // unit price in dollars
}

// OrderMessage is published to "orders_topic" after a successful DB commit.
type OrderMessage struct {
	OrderNumber     string             `json:"order_number"`
	CustomerName    string             `json:"customer_name"`
	OrderType       string             `json:"order_type"`       // "dine_in" | "takeout" | "delivery"
	TableNumber     *int               `json:"table_number"`     // null when not applicable
	DeliveryAddress *string            `json:"delivery_address"` // null when not applicable
	Items           []OrderItemMessage `json:"items"`
	TotalAmount     float64            `json:"total_amount"` // total in dollars
	Priority        int                `json:"priority"`     // 1 | 5 | 10
}

// StatusUpdateMessage is published to "notifications_fanout".
type StatusUpdateMessage struct {
	OrderNumber         string     `json:"order_number"`
	OldStatus           string     `json:"old_status"`
	NewStatus           string     `json:"new_status"`
	ChangedBy           string     `json:"changed_by"`
	Timestamp           time.Time  `json:"timestamp"`
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`
}
