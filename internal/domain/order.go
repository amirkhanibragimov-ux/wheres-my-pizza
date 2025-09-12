package domain

import "time"

// Money represents currency in minor units (cents) to avoid float issues.
// DB adapters convert to/from NUMERIC(10,2).
type Money int64

func NewMoneyFromFloat2(f float64) Money {
	return Money(f * 100.0)
}

type OrderType string

const (
	OrderTypeDineIn   OrderType = "dine_in"
	OrderTypeTakeout  OrderType = "takeout"
	OrderTypeDelivery OrderType = "delivery"
)

type OrderStatus string

const (
	StatusReceived  OrderStatus = "received"
	StatusCooking   OrderStatus = "cooking"
	StatusReady     OrderStatus = "ready"
	StatusCompleted OrderStatus = "completed"
	StatusCancelled OrderStatus = "cancelled"
)

// Allowed state transitions as per lifecycle in the spec.
var allowed = map[OrderStatus]map[OrderStatus]bool{
	StatusReceived:  {StatusCooking: true, StatusCancelled: true},
	StatusCooking:   {StatusReady: true, StatusCancelled: true},
	StatusReady:     {StatusCompleted: true, StatusCancelled: true},
	StatusCompleted: {},
	StatusCancelled: {},
}

func CanTransition(from, to OrderStatus) bool {
	nexts := allowed[from]
	return nexts != nil && nexts[to]
}

// OrderItem is immutable in domain after creation; changes happen via new order versions (not required here).
type OrderItem struct {
	ID       int64 // DB PK; zero in-memory before persistence
	OrderID  int64 // set by repo on persist
	Name     string
	Quantity int
	Price    Money // per-unit in cents
}

type Order struct {
	ID              int64
	Number          string // business identity: ORD_YYYYMMDD_NNN
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

// PriorityFromTotal implements spec thresholds: >100 →10, 50..100 →5, else 1.
func PriorityFromTotal(total Money) int {
	if total > 10000 { // > $100.00
		return 10
	}
	if total >= 5000 { // $50.00 .. $100.00
		return 5
	}
	return 1
}

// RoutingKey builds "kitchen.{order_type}.{priority}".
func (o Order) RoutingKey() string {
	return "kitchen." + string(o.Type) + "." + itoa(o.Priority)
}

// SumTotal recomputes total from items.
func (order *Order) SumTotal() {
	var sum Money
	for _, it := range o.Items {
		sum += Money(it.Quantity) * it.Price
	}
	order.TotalAmount = sum
}

// itoa is tiny local to avoid strconv in the core file; adapters can replace.
func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}

	var b [20]byte
	p := len(b)
	n := i
	for n > 0 {
		p--
		b[p] = digits[n%10]
		n /= 10
	}

	return string(b[p:])
}
