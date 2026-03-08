package database

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// RunMigrations reads all *.up.sql files from the given directory and executes
// them in lexicographic order. Because the schema uses CREATE TABLE IF NOT EXISTS
// and CREATE INDEX IF NOT EXISTS, running the same migration multiple times is
// idempotent and safe.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, logger *zap.Logger) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return err
	}

	// Collect only *.up.sql files
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			// Only apply "up" migrations
			matched, _ := filepath.Match("*.up.sql", e.Name())
			if matched {
				files = append(files, e.Name())
			}
		}
	}

	sort.Strings(files)

	for _, f := range files {
		path := filepath.Join(migrationsDir, f)
		sql, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		logger.Info("applying migration", zap.String("file", f))

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			logger.Error("migration failed", zap.String("file", f), zap.Error(err))
			return err
		}

		logger.Info("migration applied", zap.String("file", f))
	}

	return nil
}
