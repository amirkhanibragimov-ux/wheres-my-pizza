package ports

import (
	"context"
	"time"
	"your/module/internal/domain"
)

// UnitOfWork wraps a function in a DB transaction.
type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Sequencer ensures ORD_YYYYMMDD_NNN is unique and resets daily (transactional).
type OrderNumberSequencer interface {
	NextOrderNumber(ctx context.Context, dayUTC time.Time) (string, error)
}

// Orders repository coordinates orders + items. The creation MUST also insert initial status log 'received'.
type OrderRepository interface {
	CreateOrder(ctx context.Context, o *domain.Order) error
	GetByNumber(ctx context.Context, number string) (*domain.Order, error)
	// Compare-and-set status to enforce idempotency and legal transitions.
	UpdateStatusCAS(ctx context.Context, number string, expected domain.OrderStatus, next domain.OrderStatus, changedBy string, notes *string) (applied bool, err error)
	SetProcessedBy(ctx context.Context, number string, worker string) error
	SetCompletedAt(ctx context.Context, number string, t time.Time) error
	ListHistory(ctx context.Context, number string) ([]domain.StatusLog, error)
}

// Workers repository controls registration, heartbeat, counters.
type WorkerRepository interface {
	// RegisterOnline inserts or revives an offline record; returns (ok=false) if duplicate online exists.
	RegisterOnline(ctx context.Context, name, typ string) (ok bool, err error)
	MarkOffline(ctx context.Context, name string) error
	Heartbeat(ctx context.Context, name string, when time.Time) error
	IncrementProcessed(ctx context.Context, name string) error
	ListAll(ctx context.Context) ([]domain.Worker, error)
}
