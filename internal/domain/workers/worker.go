package workers

import "time"

type WorkerStatus string

const (
	WorkerOnline  WorkerStatus = "online"
	WorkerOffline WorkerStatus = "offline"
)

type Worker struct {
	ID              int64
	Name            string
	Type            string
	Status          WorkerStatus
	LastSeen        time.Time
	OrdersProcessed int
}
