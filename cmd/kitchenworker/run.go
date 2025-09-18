package kitchenworker

import (
	"context"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	newlogger "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	pg "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/rabbitmq"
)

func Run(ctx context.Context, workerName string, orderTypes *string, heartbeat, prefetch int) error {
	// set up a new logger for kitchen worker with a static request ID for startup logs
	logger := newlogger.NewLogger("kitchen-worker")
	ctx = newlogger.WithRequestID(ctx, "startup-001")

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

	return nil
}
