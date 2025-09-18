package ports

import (
	"context"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/workers"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
)

// OrderService handles POST /orders flow: validate → total → priority → number → tx insert → publish.
type OrderService interface {
	PlaceOrder(ctx context.Context, cmd CreateOrderCommand) (OrderPlaced, error)
}

type CreateOrderCommand struct {
	CustomerName    string
	Type            orders.OrderType
	TableNumber     *int
	DeliveryAddress *string
	Items           []ItemInput
}

type ItemInput struct {
	Name     string
	Quantity int
	Price    orders.Money
}

type OrderPlaced struct {
	OrderNumber string
	Status      orders.OrderStatus
	TotalAmount orders.Money
	Priority    int
}

// KitchenService drives message consumption to cooking/ready with idempotency.
type KitchenService interface {
	StartCooking(ctx context.Context, workerName string, msg contracts.OrderMessage, now time.Time) (cookFor time.Duration, err error)
	FinishCooking(ctx context.Context, workerName string, msg contracts.OrderMessage, startedAt time.Time, now time.Time) error
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
	GetOrderHistory(ctx context.Context, number string) ([]orders.StatusLog, error)
	ListWorkers(ctx context.Context, offlineIfOlderThan time.Duration, now time.Time) ([]WorkerView, error)
}

type OrderStatusView struct {
	OrderNumber         string
	CurrentStatus       orders.OrderStatus
	UpdatedAt           *time.Time
	EstimatedCompletion *time.Time
	ProcessedBy         *string
}

type WorkerView struct {
	WorkerName      string
	Status          workers.WorkerStatus
	OrdersProcessed int
	LastSeen        *time.Time
}
