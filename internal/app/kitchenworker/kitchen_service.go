package kitchenworker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
)

// kitchenService implements ports.KitchenService interface.
type kitchenService struct {
	uow         ports.UnitOfWork
	ordersRepo  ports.OrderRepository
	workersRepo ports.WorkerRepository
	publisher   ports.Publisher
	logger      *logger.Logger
}

// New creates a new OrderService with the required dependencies.
func NewKitchenService(
	uow ports.UnitOfWork,
	ordersRepo ports.OrderRepository,
	workersRepo ports.WorkerRepository,
	publisher ports.Publisher,
	logger *logger.Logger,
) ports.KitchenService {
	return &kitchenService{
		uow:         uow,
		ordersRepo:  ordersRepo,
		workersRepo: workersRepo,
		publisher:   publisher,
		logger:      logger,
	}
}

// StartCooking transitions an order into 'cooking' state and publishes a status update.
func (service *kitchenService) StartCooking(ctx context.Context, workerName string, msg contracts.OrderMessage, now time.Time) (time.Duration, error) {
	var oldStatus, newStatus orders.OrderStatus

	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		// optimistic CAS update: expected 'received' -> 'cooking'
		ok, err := service.ordersRepo.UpdateStatusCAS(txCtx, msg.OrderNumber, orders.StatusReceived, orders.StatusCooking, workerName, nil)
		if err != nil {
			return err
		}
		if !ok {
			// already cooking or beyond -> idempotent skip
			return nil
		}

		// set processed_by together with the transition
		if err := service.ordersRepo.SetProcessedBy(txCtx, msg.OrderNumber, workerName); err != nil {
			return err
		}

		oldStatus = orders.StatusReceived
		newStatus = orders.StatusCooking
		return nil
	})
	if err != nil {
		service.logger.Error(ctx, "db_transaction_failed", "failed to set status to cooking", err)
		return 0, err
	}
	if newStatus == "" {
		// no transition happened, idempotent return
		return service.cookingDuration(msg.OrderType), nil
	}

	// publish status update
	if err := service.publishStatusUpdate(ctx, msg.OrderNumber, oldStatus, newStatus, workerName, now, service.cookingDuration(msg.OrderType)); err != nil {
		service.logger.Error(ctx, "rabbitmq_publish_failed", "failed to publish status update", err)
		// continue anyway; DB commit already succeeded
	}

	service.logger.Debug(ctx, "order_processing_started", "Order set to cooking", map[string]any{
		"order_number": msg.OrderNumber,
		"by":           workerName,
	})

	return service.cookingDuration(msg.OrderType), nil
}

// FinishCooking transitions an order into 'ready' state, increments worker processed count, and publishes a status update.
func (service *kitchenService) FinishCooking(ctx context.Context, workerName string, msg contracts.OrderMessage, startedAt, now time.Time) error {
	var oldStatus, newStatus orders.OrderStatus

	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		// optimistic CAS update: expected 'cooking' -> 'ready'
		ok, err := service.ordersRepo.UpdateStatusCAS(txCtx, msg.OrderNumber, orders.StatusCooking, orders.StatusReady, workerName, nil)
		if err != nil {
			return err
		}
		if !ok {
			// already ready or beyond -> idempotent skip
			return nil
		}

		oldStatus = orders.StatusCooking
		newStatus = orders.StatusReady

		// mark completion + increment the number of orders_processed by worker
		if err := service.ordersRepo.SetCompletedAt(txCtx, msg.OrderNumber, now); err != nil {
			return err
		}
		if err := service.workersRepo.IncrementProcessed(txCtx, workerName); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		service.logger.Error(ctx, "db_transaction_failed", "failed to set status to ready", err)
		return err
	}
	if newStatus == "" {
		// no transition happened
		return nil
	}

	// publish status update
	if err = service.publishStatusUpdate(ctx, msg.OrderNumber, oldStatus, newStatus, workerName, now, 0); err != nil {
		service.logger.Error(ctx, "rabbitmq_publish_failed", "failed to publish status update", err)
	}

	service.logger.Debug(ctx, "order_completed", "Order set to ready", map[string]any{
		"order_number": msg.OrderNumber,
		"by":           workerName,
	})
	return nil
}

// cookingDuration maps order type to simulated duration.
func (service *kitchenService) cookingDuration(orderType string) time.Duration {
	switch orderType {
	case "dine_in":
		return 8 * time.Second
	case "takeout":
		return 10 * time.Second
	case "delivery":
		return 12 * time.Second
	default:
		return 10 * time.Second
	}
}

// publishStatusUpdate builds and publishes a JSON message to notifications_fanout.
func (s *kitchenService) publishStatusUpdate(
	ctx context.Context,
	orderNumber string,
	old, new orders.OrderStatus,
	by string,
	now time.Time,
	cookFor time.Duration,
) error {
	est := now
	if cookFor > 0 {
		est = now.Add(cookFor)
	}

	update := map[string]any{
		"order_number":         orderNumber,
		"old_status":           string(old),
		"new_status":           string(new),
		"changed_by":           by,
		"timestamp":            now.UTC().Format(time.RFC3339),
		"estimated_completion": est.UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("marshal status update: %w", err)
	}
	return s.publisher.Publish("notifications_fanout", "", body, 0)
}
