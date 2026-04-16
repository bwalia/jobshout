package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/metrics"
	"github.com/jobshout/server/internal/model"
)

// BudgetAlertDispatcher sends budget alerts through the notification system.
type BudgetAlertDispatcher struct {
	notifSvc NotificationService
	logger   *zap.Logger
}

// NewBudgetAlertDispatcher creates a BudgetAlertDispatcher.
// notifSvc may be nil if notifications are not configured.
func NewBudgetAlertDispatcher(notifSvc NotificationService, logger *zap.Logger) *BudgetAlertDispatcher {
	return &BudgetAlertDispatcher{notifSvc: notifSvc, logger: logger}
}

// DispatchBudgetAlert sends a budget alert notification to all configured channels.
func (d *BudgetAlertDispatcher) DispatchBudgetAlert(ctx context.Context, orgID uuid.UUID, alert *model.BudgetAlert) {
	if d.notifSvc == nil {
		return
	}

	// Record Prometheus metric.
	metrics.BudgetAlertTotal.WithLabelValues(orgID.String(), alert.AlertType).Inc()

	title := fmt.Sprintf("Budget Alert: %s limit", alert.AlertType)
	status := fmt.Sprintf("Spend $%.2f / Limit $%.2f", alert.SpendUSD, alert.LimitUSD)

	event := integration.TaskEvent{
		Type:   integration.EventBudgetAlert,
		OrgID:  orgID,
		Title:  title,
		Status: status,
	}

	if err := d.notifSvc.DispatchEvent(ctx, orgID, event); err != nil {
		d.logger.Warn("failed to dispatch budget alert notification",
			zap.String("alert_type", alert.AlertType),
			zap.Error(err),
		)
	}
}
