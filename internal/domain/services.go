package ports

import (
	"context"
	"time"
	"your/module/internal/domain"
)

// Application services (pure orchestrators over repos + bus). Implement in /internal/app/*.

// OrderService handles POST /orders flow: validate → total → priority → number → tx insert → publish.
type OrderService interface {
	PlaceOrder(ctx context.Context, cmd CreateOrderCommand) (OrderPlaced, error)
}

type CreateOrderCommand struct {
	CustomerName    string
	Type            domain.OrderType
	TableNumber     *int
	DeliveryAddress *string
	Items           []ItemInput
}

type ItemInput struct {
	Name     string
	Quantity int
	Price    domain.Money
}

type OrderPlaced struct {
	OrderNumber string
	Status      domain.OrderStatus
	TotalAmount domain.Money
	Priority    int
}

// KitchenService drives message consumption to cooking/ready with idempotency.
type KitchenService interface {
	// Called per incoming OrderMessage. Handles:
	// - set cooking (CAS from received) + log + publish status
	// - simulate cooking time (returned value tells adapter how long to sleep)
	// - set ready + log + publish status
	StartCooking(ctx context.Context, workerName string, msg domain.OrderMessage, now time.Time) (cookFor time.Duration, err error)
	FinishCooking(ctx context.Context, workerName string, msg domain.OrderMessage, startedAt time.Time, now time.Time) error
}

// WorkerService for lifecycle and heartbeats.
type WorkerService interface {
	RegisterOrExit(ctx context.Context, name, typ string) (ok bool, err error)
	Heartbeat(ctx context.Context, name string, now time.Time) error
	GracefulOffline(ctx context.Context, name string) error
}

// TrackingService powers GET /orders/* and /workers/status.
type TrackingService interface {
	GetOrderStatus(ctx context.Context, number string) (*OrderStatusView, error)
	GetOrderHistory(ctx context.Context, number string) ([]domain.StatusLog, error)
	ListWorkers(ctx context.Context, offlineIfOlderThan time.Duration, now time.Time) ([]WorkerView, error)
}

type OrderStatusView struct {
	OrderNumber         string
	CurrentStatus       domain.OrderStatus
	UpdatedAt           *time.Time
	EstimatedCompletion *time.Time
	ProcessedBy         *string
}

type WorkerView struct {
	WorkerName      string
	Status          domain.WorkerStatus
	OrdersProcessed int
	LastSeen        *time.Time
}
