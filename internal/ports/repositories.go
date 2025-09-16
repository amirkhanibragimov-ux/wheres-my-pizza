package ports

import (
	"context"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/workers"
)

// UnitOfWork wraps a function in a DB transaction.
type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Orders repository coordinates orders + items. The creation MUST also insert initial status log 'received'.
type OrderRepository interface {
	CreateOrder(ctx context.Context, o *orders.Order) error
	GetByNumber(ctx context.Context, number string) (*orders.Order, error)
	UpdateStatusCAS(ctx context.Context, number string, expected orders.OrderStatus, next orders.OrderStatus, changedBy string, notes *string) (applied bool, err error)
	SetProcessedBy(ctx context.Context, number string, worker string) error
	SetCompletedAt(ctx context.Context, number string, t time.Time) error
	ListHistory(ctx context.Context, number string) ([]orders.StatusLog, error)
}

// Workers repository controls registration, heartbeat, counters.
type WorkerRepository interface {
	RegisterOnline(ctx context.Context, name, typ string) (ok bool, err error)
	MarkOffline(ctx context.Context, name string) error
	Heartbeat(ctx context.Context, name string, when time.Time) error
	IncrementProcessed(ctx context.Context, name string) error
	ListAll(ctx context.Context) ([]workers.Worker, error)
}
