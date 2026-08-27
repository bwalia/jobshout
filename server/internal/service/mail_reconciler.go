package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/mail"
)

// MailReconciler claims due mail_connections and runs a sync. Postgres is the
// queue so a pod restart does not drop inbox polling.
type MailReconciler struct {
	svc      MailService
	interval time.Duration
	logger   *zap.Logger
}

func NewMailReconciler(svc MailService, interval time.Duration, logger *zap.Logger) *MailReconciler {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MailReconciler{svc: svc, interval: interval, logger: logger}
}

func (r *MailReconciler) Start(ctx context.Context) {
	if r.svc == nil {
		return
	}
	r.logger.Info("mail reconciler started", zap.Duration("interval", r.interval))
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("mail reconciler stopped")
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *MailReconciler) tick(ctx context.Context) {
	if err := r.svc.ProcessDueSyncs(ctx, 10); err != nil {
		r.logger.Warn("mail reconciler tick failed", zap.Error(mail.RedactErr(err)))
	}
}
