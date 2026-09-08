package service

import (
	"context"

	"github.com/jobshout/server/internal/creditcontroller"
)

// CreditControllerService is the JobShout façade over the AIVC AP runtime.
type CreditControllerService interface {
	Enabled() bool
	Ping(ctx context.Context) (map[string]any, error)
	ListInvoices(ctx context.Context) (map[string]any, error)
	GetInvoice(ctx context.Context, invoiceID string) (map[string]any, error)
	Summary(ctx context.Context) (map[string]any, error)
	Generate(ctx context.Context, cadence string, count int) (map[string]any, error)
	Triage(ctx context.Context, invoiceID string) (map[string]any, error)
	TriageBatch(ctx context.Context, invoiceIDs []string) (map[string]any, error)
	Queue(ctx context.Context) (map[string]any, error)
	Approve(ctx context.Context, runID string, approved bool, approver, note string) (map[string]any, error)
}

type creditControllerService struct {
	client *creditcontroller.Client
}

// NewCreditControllerService wraps the AIVC HTTP client.
func NewCreditControllerService(client *creditcontroller.Client) CreditControllerService {
	return &creditControllerService{client: client}
}

func (s *creditControllerService) Enabled() bool {
	return s.client != nil && s.client.Enabled()
}

func (s *creditControllerService) Ping(ctx context.Context) (map[string]any, error) {
	return s.client.Ping(ctx)
}

func (s *creditControllerService) ListInvoices(ctx context.Context) (map[string]any, error) {
	return s.client.ListInvoices(ctx)
}

func (s *creditControllerService) GetInvoice(ctx context.Context, invoiceID string) (map[string]any, error) {
	return s.client.GetInvoice(ctx, invoiceID)
}

func (s *creditControllerService) Summary(ctx context.Context) (map[string]any, error) {
	return s.client.CreditControllerSummary(ctx)
}

func (s *creditControllerService) Generate(ctx context.Context, cadence string, count int) (map[string]any, error) {
	return s.client.GenerateInvoices(ctx, cadence, count)
}

func (s *creditControllerService) Triage(ctx context.Context, invoiceID string) (map[string]any, error) {
	return s.client.Triage(ctx, invoiceID)
}

func (s *creditControllerService) TriageBatch(ctx context.Context, invoiceIDs []string) (map[string]any, error) {
	return s.client.TriageBatch(ctx, invoiceIDs)
}

func (s *creditControllerService) Queue(ctx context.Context) (map[string]any, error) {
	return s.client.Queue(ctx)
}

func (s *creditControllerService) Approve(ctx context.Context, runID string, approved bool, approver, note string) (map[string]any, error) {
	return s.client.Approve(ctx, runID, approved, approver, note)
}
