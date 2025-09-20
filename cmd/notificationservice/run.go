package notificationservice

import (
	"context"
	"fmt"
	"sync"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/notificationservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/rabbitmq"
)

func Run(ctx context.Context) error {
	// set up a new logger for notification service with a static request ID for startup logs
	logger := logger.NewLogger("notification-subscriber")
	ctx = logger.WithRequestID(ctx, "startup-001")

	// load a config from file
	cfg, err := config.LoadFromFile("config/config.yaml")
	if err != nil {
		logger.Error(ctx, "config_load_failed", "Failed to load configuration", err)
		return err
	}

	// connect to RabbitMQ
	rmq, err := rabbitmq.ConnectRabbitMQ(ctx, cfg, logger)
	if err != nil {
		logger.Error(ctx, "rabbitmq_connection_failed", "Failed to connect to RabbitMQ", err)
		return err
	}
	defer rmq.Close()

	// log service start
	logger.Info(ctx, "service_started", "Notification service started", nil)

	// start a single consumer loop (you can scale out with more goroutines if needed)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		service.ConsumeForever(ctx, rmq, logger)
	}()

	// create a channel to signal when the consumer goroutine is done
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		// normal shutdown path
	case <-done:
		// consumer exited unexpectedly -> return error to let main exit non-zero or restart
		return fmt.Errorf("notification consumer exited unexpectedly")
	}

	logger.Info(logger.WithRequestID(context.Background(), "shutdown-001"), "graceful_shutdown", "Shutting down notification subscriber", nil)

	// allow consumer goroutine to exit cleanly
	wg.Wait()
	return nil
}
