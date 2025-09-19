package kitchenworker

import (
	"context"
	"fmt"
	"time"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/kitchenworker"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	pg "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/rabbitmq"
)

func Run(ctx context.Context, workerName string, orderTypes *string, heartbeat, prefetch int) error {
	// set up a new logger for kitchen worker with a static request ID for startup logs
	logger := logger.NewLogger("kitchen-worker")
	ctx = logger.WithRequestID(ctx, "startup-001")

	// load a config from file
	cfg, err := config.LoadFromFile("config/config.yaml")
	if err != nil {
		logger.Error(ctx, "config_load_failed", "Failed to load configuration", err)
		return err
	}

	// set up a Postgres connection pool
	pool, err := pg.NewPool(ctx, cfg, logger)
	if err != nil {
		logger.Error(ctx, "db_connection_failed", "Failed to initialize Postgres pool", err)
		return err
	}
	defer pool.Close()

	rmq, err := rabbitmq.ConnectRabbitMQ(ctx, cfg, logger)
	if err != nil {
		logger.Error(ctx, "rabbitmq_connection_failed", "Failed to connect to RabbitMQ", err)
		return err
	}
	defer rmq.Close()

	// set up repositories, unit of work and service
	uow := pg.NewUnitOfWork(pool)
	workersRepo := pg.NewWorkersRepo() // import "git.platform.../internal/shared/postgres"
	workerSvc := service.NewWorkerService(uow, workersRepo, logger)

	// normalize worker type string (e.g., "dine_in, takeout, delivery")
	wtype := detectWorkerType(orderTypes)

	// register the worker as online (or exit if duplicate)
	ok, err := workerSvc.RegisterOrExit(ctx, workerName, wtype)
	if err != nil {
		return err
	}
	if !ok {
		// duplicate online worker; terminate with error to conform to spec
		return fmt.Errorf("worker '%s' is already online", workerName)
	}

	logger.Info(ctx, "service_started", "Kitchen worker started", map[string]any{
		"name":      workerName,
		"type":      wtype,
		"heartbeat": heartbeat,
		"prefetch":  prefetch,
	})

	// set up the ticker for heartbeats
	hb := time.NewTicker(time.Duration(heartbeat) * time.Second)
	defer hb.Stop()

	// run heartbeat loop until shutdown
	heartbeatErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-hb.C:
				if err := workerSvc.Heartbeat(ctx, workerName); err != nil {
					// Log inside Heartbeat as well; surface once to unblock callers if needed.
					heartbeatErrCh <- err
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// wait for shutdown signal or heartbeat failure
	select {
	case <-ctx.Done():
		// normal shutdown
	case err := <-heartbeatErrCh:
		// heartbeat died (likely DB outage) — proceed to graceful offline, return error
		logger.Error(ctx, "heartbeat_loop_stopped", "Heartbeat loop stopped", err)
	}

	// try to mark the worker as offline gracefully before exiting
	graceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := workerSvc.GracefulOffline(graceCtx, workerName); err != nil {
		logger.Error(ctx, "graceful_offline_failed", "Failed to mark offline during shutdown", err)
		// continue shutdown regardless
	} else {
		logger.Info(ctx, "graceful_shutdown", "Worker shutdown completed", map[string]any{
			"name": workerName,
		})
	}

	return nil
}
