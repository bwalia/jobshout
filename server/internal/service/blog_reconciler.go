package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const blogReconcilerInterval = 60 * time.Second

// blogOrphanReaper is the slice of BlogService the reconciler needs: fail
// rows whose writer is gone, do not start new work.
type blogOrphanReaper interface {
	ReapOrphans(ctx context.Context) (int, error)
}

// BlogReconciler ticks ReapOrphans so a SIGKILL / OOM / node drain cannot
// leave blog_runs stuck at running. It does not re-dispatch generation —
// Retry is the user's action.
type BlogReconciler struct {
	reaper   blogOrphanReaper
	interval time.Duration
	logger   *zap.Logger
}

func NewBlogReconciler(reaper blogOrphanReaper, interval time.Duration, logger *zap.Logger) *BlogReconciler {
	if interval <= 0 {
		interval = blogReconcilerInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BlogReconciler{reaper: reaper, interval: interval, logger: logger}
}

// Start runs until ctx is cancelled. One tick immediately so a restart after
// a crash reaps orphans without waiting a full interval.
func (rc *BlogReconciler) Start(ctx context.Context) {
	if rc.reaper == nil {
		rc.logger.Info("blog reconciler not started: no reaper")
		return
	}
	rc.logger.Info("blog reconciler started", zap.Duration("interval", rc.interval))
	rc.tick(ctx)

	ticker := time.NewTicker(rc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			rc.logger.Info("blog reconciler stopped")
			return
		case <-ticker.C:
			rc.tick(ctx)
		}
	}
}

// Tick is exported so tests do not have to wait on the ticker.
func (rc *BlogReconciler) Tick(ctx context.Context) error {
	n, err := rc.reaper.ReapOrphans(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		rc.logger.Info("blog reconciler reaped orphaned runs", zap.Int("count", n))
	}
	return nil
}

func (rc *BlogReconciler) tick(ctx context.Context) {
	if err := rc.Tick(ctx); err != nil {
		rc.logger.Warn("blog reconciler tick failed", zap.Error(err))
	}
}
