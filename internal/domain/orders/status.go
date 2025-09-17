package orders

// OrderStatus is a custom type that represents the current status of an order in its lifecycle.
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

// CanTransition checks if from->to is allowed.
func CanTransition(from, to OrderStatus) bool {
	nexts := allowed[from]
	return nexts != nil && nexts[to]
}
