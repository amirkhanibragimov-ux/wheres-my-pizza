// cmd/orderservice/run.go
package orderservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/orderservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	pg "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/rabbitmq"
)

// Run wires the order service and blocks until ctx is cancelled.
// It returns the first terminal error (server or startup failure).
func Run(ctx context.Context, port int, maxConcurrent int) error {
	// set up a new logger for order service with a static request ID for startup logs
	logger := logger.NewLogger("order-service")
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

	// set up the service with its dependencies
	pub := &rabbitmq.MQPublisher{Client: rmq}
	uow := pg.NewUnitOfWork(pool)
	repo := pg.NewOrdersRepo()
	svc := service.NewOrderService(uow, repo, pub, logger)

	// set up HTTP handler
	h := service.NewOrderHTTPHandler(svc, logger)
	mux := http.NewServeMux()
	h.Register(mux)

	// Concurrency limiter (global) — blocks when capacity is full.
	handler := withConcurrencyLimit(maxConcurrent, mux)

	// set up the server configurations
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	logger.Info(ctx, "service_started",
		fmt.Sprintf("Order Service started on port %d", port),
		map[string]any{"port": port, "max_concurrent": maxConcurrent},
	)

	// start the server in a background goroutine
	errCh := make(chan error, 1)
	go func() {
		// http.ErrServerClosed is returned on Shutdown; treat that as clean exit
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		// graceful HTTP shutdown on context cancel
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx) // err ignored; srv closed in any case
	case err := <-errCh:
		// server returned a terminal error at startup or during run
		return err
	}

	return nil
}

// withConcurrencyLimit wraps an http.Handler with a semaphore-based limiter.
// It blocks until capacity is available, which provides natural backpressure.
func withConcurrencyLimit(n int, next http.Handler) http.Handler {
	sem := make(chan struct{}, n)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sem <- struct{}{}        // acquire
		defer func() { <-sem }() // release
		next.ServeHTTP(w, r)
	})
}
