package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool builds a DSN from cfg, configures pgxpool, verifies connectivity, and returns the pool.
func NewPool(ctx context.Context, cfg *config.Config, logger *logger.Logger) (*pgxpool.Pool, error) {
	start := time.Now()

	// build a safe URL DSN
	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(cfg.Database.Host, strconv.Itoa(cfg.Database.Port)),
		Path:   cfg.Database.Name, // database name
		User:   url.UserPassword(cfg.Database.User, cfg.Database.Password),
	}

	// parse pgxpool config
	pcfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	// good hygiene defaults
	pcfg.HealthCheckPeriod = 30 * time.Second
	pcfg.MaxConnIdleTime = 5 * time.Minute

	// keep sessions on UTC
	pcfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET TIME ZONE 'UTC'`)
		return err
	}

	// create pool
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	// ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	logger.Info(ctx, "db_connected", "Connected to PostgreSQL database", map[string]any{"duration_ms": time.Since(start).Milliseconds()})

	return pool, nil
}
