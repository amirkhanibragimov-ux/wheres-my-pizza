package trackingservice

import (
	"context"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
)

// Service implements ports.TrackingService interface.
type Service struct {
	uow     ports.UnitOfWork
	orders  ports.OrderRepository
	workers ports.WorkerRepository
	logger  *logger.Logger
}

// NewService creates a new TrackingService instance with the required dependencies.
func NewService(uow ports.UnitOfWork, orders ports.OrderRepository, workers ports.WorkerRepository, logger *logger.Logger) ports.TrackingService {
	return &Service{
		uow:     uow,
		orders:  orders,
		workers: workers,
		logger:  logger,
	}
}

// GetOrderStatus returns the current status of the order, along with estimated completion if applicable.
func (service *Service) GetOrderStatus(ctx context.Context, number string) (*ports.OrderStatusView, error) {
	var out *ports.OrderStatusView
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		order, err := service.orders.GetByNumber(txCtx, number)
		if err != nil {
			service.logger.Error(ctx, "db_query_failed", "Failed to get order by number", err)
			return err
		}

		// compute estimated completion only when in "cooking" status
		var est *time.Time
		if order.Status == orders.StatusCooking {
			d := cookDuration(order.Type)
			t := order.UpdatedAt.Add(d)
			est = &t
		}

		// build the response view
		out = &ports.OrderStatusView{
			OrderNumber:         order.Number,
			CurrentStatus:       order.Status,
			UpdatedAt:           &order.UpdatedAt,
			EstimatedCompletion: est,
			ProcessedBy:         order.ProcessedBy,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetOrderHistory returns a list of status changes for the given order.
func (service *Service) GetOrderHistory(ctx context.Context, number string) ([]orders.StatusLog, error) {
	var hist []orders.StatusLog
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {

		var err error
		hist, err = service.orders.ListHistory(txCtx, number)
		if err != nil {
			service.logger.Error(ctx, "db_query_failed", "Failed to list order history", err)
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return hist, nil
}

// ListWorkers returns a list of all workers, marking them offline if their last seen is older than the given duration.
func (service *Service) ListWorkers(ctx context.Context, offlineIfOlderThan time.Duration, now time.Time) ([]ports.WorkerView, error) {
	var views []ports.WorkerView
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		workers, err := service.workers.ListAll(txCtx)
		if err != nil {
			service.logger.Error(ctx, "db_query_failed", "Failed to list all workers", err)
			return err
		}

		// derive offline status based on last seen and the provided threshold
		for i := range workers {
			worker := workers[i]
			derived := worker.Status

			if now.Sub(worker.LastSeen) > offlineIfOlderThan {
				derived = "offline"
			}

			views = append(views, ports.WorkerView{
				WorkerName:      worker.Name,
				Status:          derived,
				OrdersProcessed: worker.OrdersProcessed,
				LastSeen:        &worker.LastSeen,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return views, nil
}

// cookDuration returns how long it takes to cook an order of the given type.
func cookDuration(t orders.OrderType) time.Duration {
	switch t {
	case orders.OrderTypeDineIn:
		return 8 * time.Second
	case orders.OrderTypeTakeout:
		return 10 * time.Second
	case orders.OrderTypeDelivery:
		return 12 * time.Second
	default:
		return 0 // unknown: no estimate
	}
}
