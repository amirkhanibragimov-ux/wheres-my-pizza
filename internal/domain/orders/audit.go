package orders

import (
	"time"
)

type StatusLog struct {
	ID        int64
	OrderID   int64
	Status    OrderStatus
	ChangedBy *string
	ChangedAt time.Time
	Notes     *string
}
