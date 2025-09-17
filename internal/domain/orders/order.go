package orders

import (
	"strconv"
	"time"
)

// OrderType is a custom type that represents the type of order.
type OrderType string

const (
	OrderTypeDineIn   OrderType = "dine_in"
	OrderTypeTakeout  OrderType = "takeout"
	OrderTypeDelivery OrderType = "delivery"

	PriorityLow  = 1
	PriorityMid  = 5
	PriorityHigh = 10

	TotalMid  = Money(5_000)  // $50.00
	TotalHigh = Money(10_000) // $100.00
)

// OrderItem represents a single item in an order.
type OrderItem struct {
	ID       int64 // DB PK
	OrderID  int64
	Name     string
	Quantity int
	Price    Money // per-unit in cents
}

// Order represents a customer's order.
type Order struct {
	ID              int64
	Number          string // follows the format: ORD_YYYYMMDD_NNN
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CustomerName    string
	Type            OrderType
	TableNumber     *int
	DeliveryAddress *string
	Items           []OrderItem
	TotalAmount     Money
	Priority        int // 1,5,10
	Status          OrderStatus
	ProcessedBy     *string
	CompletedAt     *time.Time
}

// SetTotalAmount recomputes total from items.
func (order *Order) SetTotalAmount() {
	var sum Money
	for _, it := range order.Items {
		sum += Money(it.Quantity) * it.Price
	}
	order.TotalAmount = sum
}

// SetPriorityFromTotalAmount sets order.Priority based on TotalAmount thresholds.
func (order *Order) SetPriorityFromTotalAmount() {
	t := order.TotalAmount
	switch {
	case t > TotalHigh:
		order.Priority = PriorityHigh
	case t >= TotalMid:
		order.Priority = PriorityMid
	default:
		order.Priority = PriorityLow
	}
}

// RoutingKey builds "kitchen.{order_type}.{priority}".
func (order *Order) RoutingKey() string {
	return "kitchen." + string(order.Type) + "." + strconv.Itoa(order.Priority)
}
