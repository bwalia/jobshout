package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// NewPool creates a new PostgreSQL connection pool with sensible defaults.
func NewPool(ctx context.Context, databaseURL string, logger *zap.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	cfg.MaxConns = 25
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Info("database connection established",
		zap.Int32("max_conns", cfg.MaxConns),
		zap.Int32("min_conns", cfg.MinConns),
	)

	return pool, nil
}

// NewPoolWithRetry calls NewPool repeatedly, backing off between attempts,
// until it succeeds or timeout elapses.
//
// Without this, any blip that makes Postgres briefly unreachable — a DB
// restart, a DNS hiccup, a rescheduled StatefulSet — is fatal at startup. The
// pod then enters CrashLoopBackOff, and because the backoff grows to minutes,
// the API stays down long after the database has come back. Retrying here
// turns a short outage into a slow start instead of an outage of our own.
//
// A timeout of 0 disables retrying and behaves exactly like NewPool.
func NewPoolWithRetry(ctx context.Context, databaseURL string, logger *zap.Logger, timeout time.Duration) (*pgxpool.Pool, error) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 15 * time.Second
	)

	if timeout <= 0 {
		return NewPool(ctx, databaseURL, logger)
	}

	deadline := time.Now().Add(timeout)
	backoff := initialBackoff

	for attempt := 1; ; attempt++ {
		pool, err := NewPool(ctx, databaseURL, logger)
		if err == nil {
			if attempt > 1 {
				logger.Info("database reachable after retrying",
					zap.Int("attempts", attempt),
				)
			}
			return pool, nil
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("database connect cancelled after %d attempt(s): %w", attempt, err)
		}

		// Stop if sleeping again would carry us past the deadline; there is no
		// point burning the remaining window on a wait we know we cannot honour.
		if !time.Now().Add(backoff).Before(deadline) {
			return nil, fmt.Errorf("database unreachable after %d attempt(s) over %s: %w", attempt, timeout, err)
		}

		logger.Warn("database not reachable, retrying",
			zap.Int("attempt", attempt),
			zap.Duration("retry_in", backoff),
			zap.Error(err),
		)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("database connect cancelled after %d attempt(s): %w", attempt, err)
		case <-time.After(backoff):
		}

		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
