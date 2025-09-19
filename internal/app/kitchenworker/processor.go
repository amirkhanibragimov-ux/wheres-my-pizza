package kitchenworker

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
)

// Sleeper is a function used to simulate cooking time (time.Sleep in prod, no-op in tests).
type Sleeper func(d time.Duration)

// Processor coordinates one end-to-end processing of an order in th kitchen.
type Processor struct {
	kitchen ports.KitchenService
}

// NewProcessor creates a new Processor instance.
func NewProcessor(kitchen ports.KitchenService) *Processor {
	return &Processor{kitchen: kitchen}
}

// Process handles a single order message end-to-end.
func (p *Processor) Process(
	ctx context.Context,
	workerName string,
	workerTypesCSV string,
	msg contracts.OrderMessage,
	now time.Time,
	sleep Sleeper,
) error {
	if !supports(workerTypesCSV, msg.OrderType) {
		return errors.New("unsupported order type")
	}

	// start cooking
	cookFor, err := p.kitchen.StartCooking(ctx, workerName, msg, now)
	if err != nil {
		return err
	}

	// simulate cooking
	if cookFor > 0 {
		sleep(cookFor)
	}

	// finish cooking
	return p.kitchen.FinishCooking(ctx, workerName, msg, now, time.Now().UTC())
}

// supports returns true if the worker types CSV contains the given orderType.
func supports(csv, orderType string) bool {
	orderType = strings.ToLower(strings.TrimSpace(orderType))
	if orderType == "" {
		return false
	}

	// supports both ", " and "," formats
	normalized := strings.ReplaceAll(csv, ", ", ",")
	for _, t := range strings.Split(normalized, ",") {
		if strings.ToLower(strings.TrimSpace(t)) == orderType {
			return true
		}
	}

	return false
}
