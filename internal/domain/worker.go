package domain

import "time"

type WorkerStatus string

const (
	WorkerOnline  WorkerStatus = "online"
	WorkerOffline WorkerStatus = "offline"
)

type Worker struct {
	ID              int64
	Name            string
	Type            string // freeform specialization label
	Status          WorkerStatus
	LastSeen        time.Time
	OrdersProcessed int
}
