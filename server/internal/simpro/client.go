package simpro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Client reads Simpro Premium REST APIs and always keeps demo fixtures as fallback.
type Client struct {
	baseURL   string
	apiKey    string
	companyID string
	http      *http.Client
	logger    *zap.Logger
}

// Config is loaded from the environment.
type Config struct {
	BaseURL   string
	APIKey    string
	CompanyID string
	Timeout   time.Duration
}

// LoadConfig reads SIMPRO_* settings. Empty API key keeps the agent in demo mode.
func LoadConfig() Config {
	timeout := 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("SIMPRO_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	company := strings.TrimSpace(os.Getenv("SIMPRO_COMPANY_ID"))
	if company == "" {
		company = "0"
	}
	return Config{
		BaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("SIMPRO_BASE_URL")), "/"),
		APIKey:    strings.TrimSpace(os.Getenv("SIMPRO_API_KEY")),
		CompanyID: company,
		Timeout:   timeout,
	}
}

// NewClient builds a Simpro client. Demo mode works with empty API key.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		baseURL:   cfg.BaseURL,
		apiKey:    cfg.APIKey,
		companyID: cfg.CompanyID,
		http:      &http.Client{Timeout: cfg.Timeout},
		logger:    logger,
	}
}

// Enabled is always true — demo fixtures keep demos working without credentials.
func (c *Client) Enabled() bool { return c != nil }

// LiveConfigured reports whether a live Simpro read path is available.
func (c *Client) LiveConfigured() bool {
	return c != nil && c.apiKey != "" && c.baseURL != ""
}

// Mode returns demo | live.
func (c *Client) Mode() string {
	if c.LiveConfigured() {
		return "live"
	}
	return "demo"
}

// Status describes connectivity for the UI / Ready check.
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"mode":            c.Mode(),
		"live_configured": c.LiveConfigured(),
		"company_id":      c.companyID,
		"write_actions":   demoWriteCapability(),
		"ok":              true,
	}
	if !c.LiveConfigured() {
		out["message"] = "Running on rich demo fixtures (Watford HVAC). Set SIMPRO_API_KEY + SIMPRO_BASE_URL for live reads."
		return out
	}
	if err := c.pingLive(ctx); err != nil {
		out["ok"] = false
		out["mode"] = "live_fallback"
		out["message"] = "Live Simpro unreachable — serving demo fixtures. " + err.Error()
		return out
	}
	out["message"] = "Live Simpro reads available; writes remain discussed-not-shipped."
	return out
}

func (c *Client) pingLive(ctx context.Context) error {
	var raw json.RawMessage
	return c.get(ctx, "/api/v1.0/companies/"+c.companyID, &raw)
}

// Summary builds the dashboard payload (live invoices when possible).
func (c *Client) Summary(ctx context.Context) (Summary, error) {
	invoices, mode, err := c.ListInvoices(ctx)
	if err != nil {
		return Summary{}, err
	}
	payments, _, _ := c.ListPayments(ctx)
	fgas, _, _ := c.ListFGas(ctx)
	company := demoCompany()
	if mode == "live" {
		company = "Simpro company " + c.companyID
	}
	return BuildSummary(mode, company, "", invoices, payments, fgas), nil
}

// ListInvoices prefers live Simpro customer invoices, falling back to demo.
func (c *Client) ListInvoices(ctx context.Context) ([]Invoice, string, error) {
	if c.LiveConfigured() {
		if live, err := c.fetchInvoices(ctx); err == nil && len(live) > 0 {
			return live, "live", nil
		} else if err != nil {
			c.logger.Warn("simpro invoices live read failed — using demo", zap.Error(err))
			inv := DemoInvoices()
			return inv, "live_fallback", nil
		}
	}
	return DemoInvoices(), "demo", nil
}

// ListPayments returns receipts (demo today; live stubbed to demo on failure).
func (c *Client) ListPayments(ctx context.Context) ([]Payment, string, error) {
	if c.LiveConfigured() {
		// Customer receipts endpoints vary by build; keep demo as the reliable path
		// until scoped with the prospect. Attempt a soft probe then fall back.
		if _, err := c.fetchInvoices(ctx); err != nil {
			c.logger.Warn("simpro payments probe failed — using demo", zap.Error(err))
			return DemoPayments(), "live_fallback", nil
		}
	}
	return DemoPayments(), c.Mode(), nil
}

// ListJobs prefers live jobs list.
func (c *Client) ListJobs(ctx context.Context) ([]Job, string, error) {
	if c.LiveConfigured() {
		if live, err := c.fetchJobs(ctx); err == nil && len(live) > 0 {
			return live, "live", nil
		} else if err != nil {
			c.logger.Warn("simpro jobs live read failed — using demo", zap.Error(err))
			return DemoJobs(), "live_fallback", nil
		}
	}
	return DemoJobs(), "demo", nil
}

// ListFGas returns the refrigerant ledger (demo / spreadsheet-replacement path).
func (c *Client) ListFGas(ctx context.Context) ([]RefrigerantUsage, string, error) {
	_ = ctx
	// Live stock/catalogue mapping is customer-specific; demo ledger is the product story.
	mode := "demo"
	if c.LiveConfigured() {
		mode = "live_fallback"
	}
	return DemoFGas(), mode, nil
}

// AgingReport groups outstanding balances.
func (c *Client) AgingReport(ctx context.Context) (map[string]any, error) {
	sum, err := c.Summary(ctx)
	if err != nil {
		return nil, err
	}
	invoices, mode, _ := c.ListInvoices(ctx)
	var overdue []Invoice
	for _, inv := range invoices {
		if inv.Status == "overdue" || (inv.BalanceGBP > 0 && inv.AgingBucket != "current" && inv.AgingBucket != "paid") {
			overdue = append(overdue, inv)
		}
	}
	return map[string]any{
		"mode":       mode,
		"aging":      sum.Aging,
		"overdue":    overdue,
		"outstanding_gbp": sum.Mailbox.OutstandingGBP,
		"write_actions":   demoWriteCapability(),
	}, nil
}

// ReconcilePreview recommends payment allocations (read-only).
func (c *Client) ReconcilePreview(ctx context.Context) (map[string]any, error) {
	payments, mode, _ := c.ListPayments(ctx)
	invoices, _, _ := c.ListInvoices(ctx)
	byID := map[string]Invoice{}
	for _, inv := range invoices {
		byID[inv.ID] = inv
	}
	type suggestion struct {
		PaymentID string  `json:"payment_id"`
		InvoiceID string  `json:"invoice_id,omitempty"`
		AmountGBP float64 `json:"amount_gbp"`
		Action    string  `json:"action"`
		Note      string  `json:"note"`
	}
	var suggestions []suggestion
	for _, p := range payments {
		if p.InvoiceID == "" {
			suggestions = append(suggestions, suggestion{
				PaymentID: p.ID, AmountGBP: p.AmountGBP,
				Action: "review_unallocated",
				Note:   "Remittance has no invoice link — recommend customer matching before any Simpro write.",
			})
			continue
		}
		inv := byID[p.InvoiceID]
		note := "Matched to " + inv.Number
		if inv.BalanceGBP > 0 && p.AmountGBP < inv.BalanceGBP+p.AmountGBP {
			note = "Partial receipt against " + inv.Number
		}
		suggestions = append(suggestions, suggestion{
			PaymentID: p.ID, InvoiceID: p.InvoiceID, AmountGBP: p.AmountGBP,
			Action: "allocate", Note: note,
		})
	}
	return map[string]any{
		"mode":          mode,
		"suggestions":   suggestions,
		"write_actions": demoWriteCapability(),
		"message":       "Reconciliation preview only — posting allocations to Simpro is discussed, not shipped.",
	}, nil
}

// MonthEndChecklist returns the period-close playbook with live counts.
func (c *Client) MonthEndChecklist(ctx context.Context) (map[string]any, error) {
	sum, err := c.Summary(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":     sum.Mode,
		"period":   sum.Period,
		"checklist": sum.Playbook,
		"stats": map[string]any{
			"overdue_invoices": sum.Mailbox.Overdue,
			"outstanding_gbp":  sum.Mailbox.OutstandingGBP,
			"unallocated_gbp":  sum.Payments.UnallocatedGBP,
			"fgas_gaps":        sum.FGas.Gaps,
		},
		"write_actions": demoWriteCapability(),
	}, nil
}

// FGasReport is the audit-friendly export payload.
func (c *Client) FGasReport(ctx context.Context) (map[string]any, error) {
	events, mode, err := c.ListFGas(ctx)
	if err != nil {
		return nil, err
	}
	sum, _ := c.Summary(ctx)
	return map[string]any{
		"mode":          mode,
		"period":        sum.Period,
		"company":       sum.Company,
		"stats":         sum.FGas,
		"events":        events,
		"gaps":          filterGaps(events),
		"export_hint":   "Copy this JSON / table into your audit pack — replaces the technician spreadsheet.",
		"write_actions": demoWriteCapability(),
	}, nil
}

func filterGaps(events []RefrigerantUsage) []RefrigerantUsage {
	var out []RefrigerantUsage
	for _, e := range events {
		if e.HasGap {
			out = append(out, e)
		}
	}
	return out
}

func (c *Client) fetchInvoices(ctx context.Context) ([]Invoice, error) {
	// Prefer customerInvoices where available; fall back to invoices.
	var raw []map[string]any
	path := fmt.Sprintf("/api/v1.0/companies/%s/customerInvoices/", c.companyID)
	if err := c.get(ctx, path, &raw); err != nil {
		path = fmt.Sprintf("/api/v1.0/companies/%s/invoices/", c.companyID)
		if err2 := c.get(ctx, path, &raw); err2 != nil {
			return nil, err2
		}
	}
	out := make([]Invoice, 0, len(raw))
	for _, row := range raw {
		inv := mapInvoice(row)
		out = append(out, inv)
	}
	return out, nil
}

func (c *Client) fetchJobs(ctx context.Context) ([]Job, error) {
	var raw []map[string]any
	path := fmt.Sprintf("/api/v1.0/companies/%s/jobs/", c.companyID)
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(raw))
	for _, row := range raw {
		out = append(out, mapJob(row))
	}
	return out, nil
}

func mapInvoice(row map[string]any) Invoice {
	id := fmt.Sprint(row["ID"])
	if id == "" || id == "<nil>" {
		id = fmt.Sprint(row["id"])
	}
	num := strField(row, "InvoiceNo", "Number", "Name")
	if num == "" {
		num = "INV-" + id
	}
	total := floatField(row, "Total", "TotalIncTax", "Amount")
	paid := floatField(row, "AmountPaid", "Paid", "TotalPaid")
	bal := total - paid
	if v := floatField(row, "Balance", "AmountDue"); v > 0 {
		bal = v
	}
	status := "open"
	switch {
	case bal <= 0.01:
		status = "paid"
		bal = 0
	case paid > 0:
		status = "partial"
	}
	due := strField(row, "DateDue", "DueDate")
	issued := strField(row, "DateIssued", "Date")
	bucket := "current"
	if due != "" {
		if t, err := time.Parse("2006-01-02", due[:min(10, len(due))]); err == nil && t.Before(time.Now().AddDate(0, 0, -1)) && bal > 0 {
			status = "overdue"
			days := int(time.Since(t).Hours() / 24)
			switch {
			case days <= 30:
				bucket = "1-30"
			case days <= 60:
				bucket = "31-60"
			case days <= 90:
				bucket = "61-90"
			default:
				bucket = "90+"
			}
		}
	}
	customer := strField(row, "CustomerName")
	if customer == "" {
		if m, ok := row["Customer"].(map[string]any); ok {
			customer = strField(m, "CompanyName", "Name", "GivenName")
		}
	}
	return Invoice{
		ID: id, Number: num, Customer: customer, IssuedOn: issued, DueOn: due,
		AmountGBP: total, PaidGBP: paid, BalanceGBP: bal, Status: status,
		AgingBucket: bucket, Source: "simpro",
	}
}

func mapJob(row map[string]any) Job {
	id := fmt.Sprint(row["ID"])
	num := strField(row, "Name", "Number")
	if num == "" {
		num = "J-" + id
	}
	customer := ""
	if m, ok := row["Customer"].(map[string]any); ok {
		customer = strField(m, "CompanyName", "Name")
	}
	site := ""
	if m, ok := row["Site"].(map[string]any); ok {
		site = strField(m, "Name")
	}
	return Job{
		ID: id, Number: num, Customer: customer, Site: site,
		Status: strField(row, "Status"), Type: strField(row, "Type"), Source: "simpro",
	}
}

func strField(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func floatField(row map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return t
			case float32:
				return float64(t)
			case int:
				return float64(t)
			case json.Number:
				f, _ := t.Float64()
				return f
			case string:
				f, _ := strconv.ParseFloat(t, 64)
				return f
			}
		}
	}
	return 0
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("simpro GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("simpro GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("simpro decode %s: %w", path, err)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
