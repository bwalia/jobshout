package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// usage_records is RANGE-partitioned by created_at, one partition per month.
// Migration 000008 creates the current month plus three ahead, and because
// RunMigrations replays every .up.sql on boot, a restart rolls that horizon
// forward. A server that stays up longer than the horizon runs off the end of
// it — and that failure is silent twice over: the insert error surfaces on a
// goroutine that only logs it (execution_service.go), so requests keep
// succeeding while usage stops being recorded; and budget enforcement reads
// current-period spend from this same table, so spend reads as zero and every
// hard limit quietly stops firing.
//
// Keeping the horizon rolling without a restart is what this file is for.

const (
	// How many months beyond the current one to keep partitioned. Three means
	// the table survives a quarter of uptime even if this loop dies entirely.
	partitionMonthsAhead = 3

	// Daily is ample for a horizon measured in months, and cheap: the work is
	// four CREATE TABLE IF NOT EXISTS statements that almost always no-op.
	partitionCheckInterval = 24 * time.Hour
)

type monthPartition struct {
	Name string
	From time.Time
	To   time.Time
}

// usagePartitions returns the partitions that should exist for the month
// containing now, plus the next monthsAhead months.
//
// Kept separate from the SQL so the month arithmetic — where a December start
// has to roll the year, which is exactly the case that breaks in production —
// is testable without a database.
func usagePartitions(now time.Time, monthsAhead int) []monthPartition {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	out := make([]monthPartition, 0, monthsAhead+1)
	for i := 0; i <= monthsAhead; i++ {
		next := start.AddDate(0, 1, 0)
		out = append(out, monthPartition{
			// Matches the naming 000008 already uses, so this creates the same
			// partitions that migration would rather than a parallel set.
			Name: fmt.Sprintf("usage_records_%04d_%02d", start.Year(), int(start.Month())),
			From: start,
			To:   next,
		})
		start = next
	}
	return out
}

// EnsureUsagePartitions creates any missing monthly partition of usage_records
// for the current month and the next partitionMonthsAhead months.
func EnsureUsagePartitions(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	for _, p := range usagePartitions(now, partitionMonthsAhead) {
		// Partition names and bounds are identifiers and literals in DDL, which
		// cannot be bound as parameters. Both are derived from a time.Time here,
		// never from user input.
		stmt := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF usage_records
			 FOR VALUES FROM ('%s') TO ('%s')`,
			p.Name,
			p.From.Format("2006-01-02"),
			p.To.Format("2006-01-02"),
		)
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create partition %s: %w", p.Name, err)
		}
	}
	return nil
}

// StartUsagePartitionMaintainer runs until ctx is cancelled. One tick
// immediately, so a restart repairs a horizon that has already lapsed without
// waiting a full day.
func StartUsagePartitionMaintainer(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) {
	if pool == nil {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Info("usage partition maintainer started",
		zap.Int("months_ahead", partitionMonthsAhead),
		zap.Duration("interval", partitionCheckInterval))

	tick := func() {
		if err := EnsureUsagePartitions(ctx, pool, time.Now().UTC()); err != nil {
			// Worth alerting on: from here the meter and the budget brake are
			// both running on borrowed time.
			logger.Error("failed to ensure usage partitions", zap.Error(err))
		}
	}
	tick()

	ticker := time.NewTicker(partitionCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("usage partition maintainer stopped")
			return
		case <-ticker.C:
			tick()
		}
	}
}
