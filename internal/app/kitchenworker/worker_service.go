package kitchenworker

import (
	"context"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
)

// workerService coordinates lifecycle operations for a kitchen worker.
type workerService struct {
	uow    ports.UnitOfWork
	repo   ports.WorkerRepository
	logger *logger.Logger
}

// NewWorkerService constructs a WorkerService implementation.
func NewWorkerService(uow ports.UnitOfWork, repo ports.WorkerRepository, logger *logger.Logger) ports.WorkerService {
	return &workerService{
		uow:    uow,
		repo:   repo,
		logger: logger,
	}
}

// RegisterOrExit tries to register a worker as 'online'. Returns false if a duplicate online worker with the same name already exists.
func (service *workerService) RegisterOrExit(ctx context.Context, name, typ string) (bool, error) {
	var ok bool
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		ok, err = service.repo.RegisterOnline(txCtx, name, typ)
		return err
	})
	if err != nil {
		service.logger.Error(ctx, "worker_registration_failed", "Failed to register worker as online", err)
		return false, err
	}
	if !ok {
		// duplicate instance online with the same name
		service.logger.Error(ctx, "worker_duplicate", "Worker with this name is already online; terminating", nil)
		return false, nil
	}

	service.logger.Info(ctx, "worker_registered", "Worker registered as online", map[string]any{
		"name": name,
		"type": typ,
	})

	return true, nil
}

// Heartbeat updates last_seen and ensures status remains 'online'.
func (service *workerService) Heartbeat(ctx context.Context, name string) error {
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		return service.repo.Heartbeat(txCtx, name)
	})
	if err != nil {
		service.logger.Error(ctx, "heartbeat_failed", "Failed to send heartbeat", err)
		return err
	}

	service.logger.Debug(ctx, "heartbeat_sent", "Worker heartbeat sent", map[string]any{
		"name": name,
		"at":   time.Now().UTC().Format(time.RFC3339Nano),
	})

	return nil
}

// GracefulOffline marks the worker 'offline'.
func (service *workerService) GracefulOffline(ctx context.Context, name string) error {
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		return service.repo.MarkOffline(txCtx, name)
	})
	if err != nil {
		service.logger.Error(ctx, "graceful_offline_failed", "Failed to mark worker offline", err)
		return err
	}

	service.logger.Info(ctx, "graceful_offline", "Worker marked offline", map[string]any{
		"name": name,
	})

	return nil
}
