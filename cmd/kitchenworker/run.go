package kitchenworker

import (
	"context"
	"fmt"
	"strings"

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
	workersRepo := pg.NewWorkersRepo() // import "git.platform.../internal/shared/postgres"

	// build worker service
	workerSvc := service.NewWorkerService(uow, workersRepo, logger)

	// register the worker as online (or exit if duplicate)
	ok, err := workerSvc.RegisterOrExit(ctx, workerName, detectWorkerType(orderTypes))
	if err != nil {
		return err
	}
	if !ok {
		// duplicate online worker; terminate with error to conform to spec
		return fmt.Errorf("worker '%s' is already online", workerName)
	}

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
