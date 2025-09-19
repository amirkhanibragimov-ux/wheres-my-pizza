package kitchenworker

import (
	"context"
	"strings"

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

	// set up repositories and unit of work
	uow := pg.NewUnitOfWork(pool)
	workersRepo := pg.NewWorkersRepo() // import "git.platform.../internal/shared/postgres"

	// register the worker as online (or exit if duplicate)
	ok := false
	err = uow.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		ok, err = workersRepo.RegisterOnline(ctx, workerName, detectWorkerType(orderTypes))
		return err
	})
	if err != nil {
		logger.Error(ctx, "worker_registration_failed", "Failed to register worker as online", err)
	}
	if !ok {
		logger.Error(ctx, "worker_duplicate", "Worker with this name is already online; terminating", nil)
	}
	logger.Info(ctx, "worker_registered", "Worker registered as online", map[string]any{"name": workerName, "type": detectWorkerType(orderTypes)})

	return nil
}

// detectWorkerType returns a canonical, comma+space separated list of supported order types.
func detectWorkerType(orderTypes *string) string {
	allowed := map[string]struct{}{
		"dine_in":  {},
		"takeout":  {},
		"delivery": {},
	}
	order := []string{"dine_in", "takeout", "delivery"}

	// Helper to render canonical list from a presence set.
	render := func(have map[string]bool) string {
		var out []string
		for _, k := range order {
			if have[k] {
				out = append(out, k)
			}
		}
		// Join with comma+space for readability/consistency.
		return strings.Join(out, ", ")
	}

	// Default: all supported.
	if orderTypes == nil || strings.TrimSpace(*orderTypes) == "" {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	parts := strings.Split(*orderTypes, ",")
	have := map[string]bool{}

	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; ok {
			have[p] = true
		}
	}

	// If user input yields nothing valid, treat as "all supported".
	if len(have) == 0 {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	return render(have)
}
