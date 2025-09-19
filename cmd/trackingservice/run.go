package trackingservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/trackingservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
	pg "git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/postgres"
)

func Run(ctx context.Context, port int) error {
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

	// set up the service with its dependencies
	uow := postgres.NewUnitOfWork(pool)
	ordersRepo := postgres.NewOrdersRepo()
	workersRepo := postgres.NewWorkersRepo()
	svc := service.NewService(uow, ordersRepo, workersRepo, logger)

	// routes
	mux := http.NewServeMux()
	service.NewHandler(logger, svc).Register(mux)

	// set up the server configurations
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,                                   // time to read headers
		WriteTimeout:      15 * time.Second,                                  // full response write timeout
		IdleTimeout:       60 * time.Second,                                  // keep-alive window
		BaseContext:       func(net.Listener) context.Context { return ctx }, // pass base ctx to all handlers
	}

	// log service start
	logger.Info(ctx, "service_started", "Tracking Service started", map[string]any{"port": port})

	// run server and wait for ctx cancellation
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	// wait for context cancellation or server error
	select {
	case <-ctx.Done():
		// graceful HTTP shutdown on context cancel
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	case err := <-errCh:
		// server returned a terminal error at startup or during run
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}

	return nil
}
