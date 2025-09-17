// cmd/orderservice/run.go
package orderservice

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/orderservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	pg "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
)

// Run wires the order service and blocks until ctx is cancelled.
// It returns the first terminal error (server or startup failure).
func Run(ctx context.Context, port int, maxConcurrent int) {
	// set up a new logger for order service
	logger := logger.NewLogger("order-service")

	// load a config from file
	cfg, err := config.LoadFromFile("config/config.yaml")
	if err != nil {
		logger.Error(ctx, "config_load_failed", "Failed to load configuration", err)
		log.Fatal(err)
	}

	// set up a Postgres connection pool
	pool, err := pg.NewPool(ctx, cfg)
	if err != nil {
		logger.Error(ctx, "db_connection_failed", "Failed to initialize Postgres pool", err)
		log.Fatal(err)
	}
	defer pool.Close()
	logger.Info(ctx, "db_connected", "Connected to PostgreSQL database", nil)

	// set up repositories, unit of work, and application service
	uow := pg.NewUnitOfWork(pool)
	repo := pg.NewOrdersRepo()
	svc := service.New(uow, repo, logger)

	// set up HTTP handler
	h := service.NewOrderHTTPHandler(svc, logger)

	mux := http.NewServeMux()
	h.Register(mux)

	// Concurrency limiter (global) — blocks when capacity is full.
	handler := withConcurrencyLimit(maxConcurrent, mux)

	// Optional: add request timeouts, keep-alive tuning, etc.
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		// Tie server lifetime to incoming ctx (nice for tests / parent cancel).
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	logger.Info(ctx, "service_started",
		fmt.Sprintf("Order Service started on port %d", port),
		map[string]any{"port": port, "max_concurrent": maxConcurrent},
	)

	// ---- RabbitMQ (intentionally omitted now) --------------------------------
	// TODO: Wire RabbitMQ publisher here (exchange "orders_topic"), then inject
	// a publisher into your service. Keep it commented until you’re ready.

	// ---- Serve + graceful shutdown -------------------------------------------
	errCh := make(chan error, 1)
	go func() {
		// http.ErrServerClosed is returned on Shutdown; treat that as clean exit.
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		// Graceful HTTP shutdown (drain keep-alives / in-flight requests).
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx) // best-effort
	case err := <-errCh:
		// Server returned a terminal error at startup or during run.
		log.Fatal(err)
	}
}

// withConcurrencyLimit wraps an http.Handler with a semaphore-based limiter.
// It *blocks* until capacity is available, which provides natural backpressure.
// If you prefer a fast-fail (HTTP 429), switch the acquire to a non-blocking select.
func withConcurrencyLimit(n int, next http.Handler) http.Handler {
	sem := make(chan struct{}, n)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sem <- struct{}{}        // acquire
		defer func() { <-sem }() // release
		next.ServeHTTP(w, r)
	})
}
