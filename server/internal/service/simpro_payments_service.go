package service

import (
	"context"

	"github.com/jobshout/server/internal/simpro"
)

// SimproPaymentsService is the JobShout façade over Simpro reads + demo fixtures.
type SimproPaymentsService interface {
	Enabled() bool
	Status(ctx context.Context) map[string]any
	Summary(ctx context.Context) (simpro.Summary, error)
	ListInvoices(ctx context.Context) ([]simpro.Invoice, string, error)
	ListPayments(ctx context.Context) ([]simpro.Payment, string, error)
	ListJobs(ctx context.Context) ([]simpro.Job, string, error)
	ListFGas(ctx context.Context) ([]simpro.RefrigerantUsage, string, error)
	AgingReport(ctx context.Context) (map[string]any, error)
	ReconcilePreview(ctx context.Context) (map[string]any, error)
	MonthEndChecklist(ctx context.Context) (map[string]any, error)
	FGasReport(ctx context.Context) (map[string]any, error)
}

type simproPaymentsService struct {
	client *simpro.Client
}

// NewSimproPaymentsService wraps the Simpro client.
func NewSimproPaymentsService(client *simpro.Client) SimproPaymentsService {
	return &simproPaymentsService{client: client}
}

func (s *simproPaymentsService) Enabled() bool {
	return s.client != nil && s.client.Enabled()
}

func (s *simproPaymentsService) Status(ctx context.Context) map[string]any {
	return s.client.Status(ctx)
}

func (s *simproPaymentsService) Summary(ctx context.Context) (simpro.Summary, error) {
	return s.client.Summary(ctx)
}

func (s *simproPaymentsService) ListInvoices(ctx context.Context) ([]simpro.Invoice, string, error) {
	return s.client.ListInvoices(ctx)
}

func (s *simproPaymentsService) ListPayments(ctx context.Context) ([]simpro.Payment, string, error) {
	return s.client.ListPayments(ctx)
}

func (s *simproPaymentsService) ListJobs(ctx context.Context) ([]simpro.Job, string, error) {
	return s.client.ListJobs(ctx)
}

func (s *simproPaymentsService) ListFGas(ctx context.Context) ([]simpro.RefrigerantUsage, string, error) {
	return s.client.ListFGas(ctx)
}

func (s *simproPaymentsService) AgingReport(ctx context.Context) (map[string]any, error) {
	return s.client.AgingReport(ctx)
}

func (s *simproPaymentsService) ReconcilePreview(ctx context.Context) (map[string]any, error) {
	return s.client.ReconcilePreview(ctx)
}

func (s *simproPaymentsService) MonthEndChecklist(ctx context.Context) (map[string]any, error) {
	return s.client.MonthEndChecklist(ctx)
}

func (s *simproPaymentsService) FGasReport(ctx context.Context) (map[string]any, error) {
	return s.client.FGasReport(ctx)
}
