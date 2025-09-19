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

	// set up repositories and unit of work
	uow := pg.NewUnitOfWork(pool)
	workersRepo := pg.NewWorkersRepo()
	ordersRepo := pg.NewOrdersRepo()

	// set up the service with its dependencies
	pub := &rabbitmq.MQPublisher{Client: rmq}
	workerSvc := service.NewWorkerService(uow, workersRepo, logger)
	kitchenSvc := service.NewKitchenService(uow, ordersRepo, workersRepo, pub, logger)
	processor := service.NewProcessor(kitchenSvc)

	// normalize worker type string (e.g., "dine_in, takeout, delivery")
	wtype := detectWorkerType(orderTypes)

	// register (or exit if duplicate)
	ok, err := workerSvc.RegisterOrExit(ctx, workerName, wtype)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("worker %q is already online", workerName)
	}

	// log startup details
	logger.Info(ctx, "service_started", "Kitchen worker started", map[string]any{
		"name":      workerName,
		"type":      wtype,
		"heartbeat": heartbeat,
		"prefetch":  prefetch,
	})

	// define a ticker for heartbeats
	hb := time.NewTicker(time.Duration(heartbeat) * time.Second)
	defer hb.Stop()

	// set up a heartbeat loop in a background goroutine
	heartbeatErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-hb.C:
				if err := workerSvc.Heartbeat(ctx, workerName); err != nil {
					heartbeatErrCh <- err
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// set up a consumer loop in a background goroutine
	consumeErrCh := make(chan error, 1)
	go func() {
		// backoff parameters
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}

			ch, err := rmq.NewConsumerChannel(prefetch)
			if err != nil {
				logger.Error(ctx, "rabbitmq_channel_failed", "Failed to open consumer channel", err)
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}

			consumerTag := fmt.Sprintf("kitchen-%s", workerName)
			deliveries, err := ch.Consume(
				"kitchen_queue",
				consumerTag,
				false, // manual ack
				false,
				false,
				false,
				nil,
			)
			if err != nil {
				_ = ch.Close()
				logger.Error(ctx, "rabbitmq_consume_failed", "Failed to start consuming", err)
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}

			// reset backoff after a successful subscribe
			backoff = time.Second

			// read loop
			for {
				select {
				case <-ctx.Done():
					// stop consuming and let broker requeue any in-flight
					_ = ch.Cancel(consumerTag, false)
					_ = ch.Close()
					return
				case d, ok := <-deliveries:
					if !ok {
						// channel closed (connection lost or server-side cancel) → resubscribe
						_ = ch.Close()
						time.Sleep(backoff)
						if backoff < 30*time.Second {
							backoff *= 2
						}
						goto resubscribe
					}
					handleDelivery(ctx, logger, processor, workerName, wtype, d)
				}
			}
		resubscribe:
			continue
		}
	}()

	// wait for termination (context cancel, heartbeat failure, or consumer channel failure)
	var retErr error
	select {
	case <-ctx.Done():
	case err := <-heartbeatErrCh:
		logger.Error(ctx, "heartbeat_loop_stopped", "Heartbeat loop stopped", err)
		retErr = err
	case err := <-consumeErrCh:
		logger.Error(ctx, "consumer_stopped", "Consumer channel stopped", err)
		retErr = err
	}

	// attempt graceful offline mark
	graceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := workerSvc.GracefulOffline(graceCtx, workerName); err != nil {
		logger.Error(ctx, "graceful_offline_failed", "Failed to mark offline during shutdown", err)
	} else {
		logger.Info(ctx, "graceful_shutdown", "Worker shutdown completed", map[string]any{
			"name": workerName,
		})
	}

	return retErr
}
