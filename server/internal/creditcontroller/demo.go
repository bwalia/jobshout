package creditcontroller

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Demo fixtures mirror the aivc-agents AP responses the Credit Controller tab
// parses, so the tab and agent_execute work without the runtime. Every builder
// returns fresh maps; callers may mutate what they get.

const demoStatusMessage = "Running on demo fixtures (Northgate AP mailbox). Set AIVC_BASE_URL to the aivc-agents API for live invoices, AI generation, triage and approvals."

const demoCompany = "Northgate Facilities Ltd (demo)"

type demoInv struct {
	ID       string
	Supplier string
	AgeDays  int // days before today the invoice arrived
	PO       string
	Scenario string
	Source   string // mailbox | generated
	RunID    string
	Status   string // "" (untriaged) | awaiting_approval | posted | failed
	Reason   string // why it is awaiting approval / failed
	Lines    []demoLine
	Note     string // extra text on the document (e.g. bank-change letter)
}

type demoLine struct {
	Desc  string
	Qty   float64
	Price float64
}

func demoInvoices() []demoInv {
	return []demoInv{
		{ID: "INV-1001", Supplier: "Hertford Electrical Supplies Ltd", AgeDays: 18, PO: "PO-44810", Scenario: "STRAIGHT_THROUGH",
			Source: "mailbox", RunID: "run-7a1c02", Status: "posted",
			Lines: []demoLine{{"LED panel 600x600 4000K", 40, 38.50}, {"Emergency bulkhead fitting", 12, 64.00}}},
		{ID: "INV-1002", Supplier: "Chiltern Cleaning Services", AgeDays: 16, PO: "PO-44795", Scenario: "STRAIGHT_THROUGH",
			Source: "mailbox", RunID: "run-7a1c07", Status: "posted",
			Lines: []demoLine{{"Contract cleaning — Watford office (August)", 1, 2860.00}}},
		{ID: "INV-1003", Supplier: "Apex HVAC Parts", AgeDays: 14, PO: "PO-44822", Scenario: "PRICE_VARIANCE",
			Source: "mailbox", RunID: "run-7a1c11", Status: "awaiting_approval",
			Reason: "Unit price £212.00 vs PO £189.00 (+12.2%) exceeds the 5% tolerance.",
			Lines:  []demoLine{{"Condensate pump CP-240", 6, 212.00}, {"Delivery", 1, 45.00}}},
		{ID: "INV-1004", Supplier: "Stortford Timber & Fixings", AgeDays: 12, PO: "PO-44830", Scenario: "QUANTITY_VARIANCE",
			Source: "mailbox", RunID: "run-7a1c15", Status: "awaiting_approval",
			Reason: "Invoiced 120 units; goods receipt GRN-9912 shows 100 received.",
			Lines:  []demoLine{{"Treated batten 47x50 3.6m", 120, 6.40}, {"Screws A2 5x60 (box 200)", 8, 18.90}}},
		{ID: "INV-1005", Supplier: "Brightside Office Interiors", AgeDays: 10, PO: "", Scenario: "NO_PO",
			Source: "mailbox",
			Lines:  []demoLine{{"Meeting room refurbishment — stage payment 1", 1, 7450.00}}},
		{ID: "INV-1006", Supplier: "Apex HVAC Parts", AgeDays: 9, PO: "PO-44822", Scenario: "DUPLICATE_SUSPECT",
			Source: "mailbox",
			Note:   "Supplier reference AHP-30918 matches INV-1003 (same amount, same PO).",
			Lines:  []demoLine{{"Condensate pump CP-240", 6, 212.00}, {"Delivery", 1, 45.00}}},
		{ID: "INV-1007", Supplier: "Lea Valley Waste Management", AgeDays: 8, PO: "PO-44841", Scenario: "BANK_DETAIL_CHANGE",
			Source: "mailbox", RunID: "run-7a1c21", Status: "awaiting_approval",
			Reason: "Remittance bank details differ from the supplier master record — call-back verification required.",
			Note:   "IMPORTANT: our bank details have changed. Please pay to Sort code 20-45-77, Account 83190442 from this invoice onward.",
			Lines:  []demoLine{{"Mixed recycling collection (August)", 1, 1180.00}, {"Confidential waste consoles", 4, 36.00}}},
		{ID: "INV-1008", Supplier: "Hemel Security Systems", AgeDays: 6, PO: "PO-44856", Scenario: "STRAIGHT_THROUGH",
			Source: "mailbox",
			Lines:  []demoLine{{"CCTV maintenance visit — quarterly", 1, 640.00}, {"Replacement dome camera", 2, 189.00}}},
		{ID: "INV-1009", Supplier: "Tring Catering Equipment", AgeDays: 5, PO: "PO-44860", Scenario: "PRICE_VARIANCE",
			Source: "mailbox", RunID: "run-7a1c29", Status: "failed",
			Reason: "PO-44860 is closed in the ledger; posting was rejected.",
			Lines:  []demoLine{{"Combi oven service kit", 1, 1325.00}}},
		{ID: "INV-1010", Supplier: "Chiltern Cleaning Services", AgeDays: 3, PO: "PO-44871", Scenario: "STRAIGHT_THROUGH",
			Source: "generated",
			Lines:  []demoLine{{"Window cleaning — external, 3 floors", 1, 520.00}}},
		{ID: "INV-1011", Supplier: "Berkhamsted Print Works", AgeDays: 2, PO: "", Scenario: "NO_PO",
			Source: "generated",
			Lines:  []demoLine{{"A4 letterheads 120gsm", 5000, 0.09}, {"Business cards (per 250)", 6, 24.00}}},
		{ID: "INV-1012", Supplier: "Hertford Electrical Supplies Ltd", AgeDays: 1, PO: "PO-44880", Scenario: "QUANTITY_VARIANCE",
			Source: "generated",
			Lines:  []demoLine{{"Cat6 cable 305m drum", 4, 118.00}, {"RJ45 keystone jack", 150, 3.20}}},
	}
}

func (d demoInv) amount() float64 {
	var sub float64
	for _, l := range d.Lines {
		sub += l.Qty * l.Price
	}
	return roundPence(sub * 1.2) // incl. 20% VAT
}

func (d demoInv) received(now time.Time) string {
	return now.AddDate(0, 0, -d.AgeDays).Format("2006-01-02")
}

func (d demoInv) summaryMap(now time.Time) map[string]any {
	m := map[string]any{
		"invoice_id":    d.ID,
		"supplier_name": d.Supplier,
		"received_at":   d.received(now),
		"amount_gbp":    d.amount(),
		"po_reference":  nil,
		"scenario_hint": d.Scenario,
		"source":        d.Source,
		"workflow":      nil,
	}
	if d.PO != "" {
		m["po_reference"] = d.PO
	}
	if d.Status != "" {
		m["workflow"] = map[string]any{
			"run_id":     d.RunID,
			"status":     d.Status,
			"updated_at": now.AddDate(0, 0, -d.AgeDays+1).Format(time.RFC3339),
		}
	}
	return m
}

func (d demoInv) document(now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nVAT reg. GB %s\n\n", strings.ToUpper(d.Supplier), vatNumber(d.Supplier))
	fmt.Fprintf(&b, "TAX INVOICE %s\nDate: %s\nBill to: %s\nAccounts Payable, 1 Clarendon Road, Watford WD17 1HP\n", d.ID, d.received(now), strings.TrimSuffix(demoCompany, " (demo)"))
	if d.PO != "" {
		fmt.Fprintf(&b, "Your PO: %s\n", d.PO)
	} else {
		b.WriteString("Your PO: (none quoted)\n")
	}
	b.WriteString("\nQTY      DESCRIPTION                                   UNIT       TOTAL\n")
	var sub float64
	for _, l := range d.Lines {
		line := l.Qty * l.Price
		sub += line
		fmt.Fprintf(&b, "%-8s %-45s £%-9.2f £%.2f\n", trimQty(l.Qty), l.Desc, l.Price, line)
	}
	fmt.Fprintf(&b, "\nSubtotal  £%.2f\nVAT 20%%   £%.2f\nTOTAL     £%.2f\n", sub, roundPence(sub*0.2), d.amount())
	b.WriteString("\nPayment terms: 30 days from invoice date.\n")
	if d.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", d.Note)
	}
	b.WriteString("\n— Demo fixture: not a real supplier document —\n")
	return b.String()
}

func demoInvoiceList() map[string]any {
	now := time.Now().UTC()
	rows := demoInvoices()
	list := make([]map[string]any, 0, len(rows))
	var total float64
	for _, d := range rows {
		list = append(list, d.summaryMap(now))
		total += d.amount()
	}
	return map[string]any{
		"mode":      "demo",
		"count":     len(list),
		"total_gbp": roundPence(total),
		"invoices":  list,
	}
}

func demoInvoice(id string) (map[string]any, bool) {
	now := time.Now().UTC()
	for _, d := range demoInvoices() {
		if strings.EqualFold(d.ID, strings.TrimSpace(id)) {
			m := d.summaryMap(now)
			m["mode"] = "demo"
			m["document_text"] = d.document(now)
			return m, true
		}
	}
	return nil, false
}

func demoQueue() map[string]any {
	now := time.Now().UTC()
	items := []map[string]any{}
	for _, d := range demoInvoices() {
		if d.Status != "awaiting_approval" {
			continue
		}
		items = append(items, map[string]any{
			"run_id":        d.RunID,
			"invoice_id":    d.ID,
			"supplier_name": d.Supplier,
			"amount_gbp":    d.amount(),
			"scenario_hint": d.Scenario,
			"reason":        d.Reason,
			"received_at":   d.received(now),
		})
	}
	return map[string]any{
		"mode":              "demo",
		"count":             len(items),
		"awaiting_approval": items,
	}
}

func demoSummary() map[string]any {
	rows := demoInvoices()
	var total float64
	untriaged, posted, failed := 0, 0, 0
	byScenario := map[string]any{}
	counts := map[string]int{}
	queue := []map[string]any{}
	for _, d := range rows {
		total += d.amount()
		counts[d.Scenario]++
		switch d.Status {
		case "":
			untriaged++
		case "posted":
			posted++
		case "failed":
			failed++
		case "awaiting_approval":
			queue = append(queue, map[string]any{"run_id": d.RunID, "invoice_id": d.ID})
		}
	}
	for k, v := range counts {
		byScenario[k] = v
	}
	return map[string]any{
		"mode":    "demo",
		"company": demoCompany,
		"role":    "Credit Controller",
		"period":  time.Now().UTC().Format("2006-01") + " (demo)",
		"message": demoStatusMessage,
		"mailbox": map[string]any{
			"total_invoices": len(rows),
			"untriaged":      untriaged,
			"total_gbp":      roundPence(total),
		},
		"queue": map[string]any{
			"awaiting_approval": len(queue),
			"items":             queue,
		},
		"outcomes":      map[string]any{"posted": posted, "failed": failed},
		"by_scenario":   byScenario,
		"write_actions": writeCapability(false),
		"playbook": []map[string]any{
			{"step": 1, "task": "Intake the AP mailbox", "detail": "Pull supplier invoices from the shared mailbox; skip anything already carrying a workflow run."},
			{"step": 2, "task": "Extract and 3-way match", "detail": "Read supplier, PO, lines and totals; match against the PO and goods receipt within a 5% price tolerance."},
			{"step": 3, "task": "Classify exceptions", "detail": "Tag price and quantity variances, missing POs, suspected duplicates and bank-detail changes."},
			{"step": 4, "task": "Apply policy", "detail": "Bank-detail changes need call-back verification; duplicates are held; no-PO invoices go to the budget holder."},
			{"step": 5, "task": "Approve with segregation of duties", "detail": "The clerk who triaged cannot approve — the controller signs off exceptions in the queue."},
			{"step": 6, "task": "Post and audit", "detail": "Post approved invoices to the ledger and keep the full decision trail for audit."},
		},
	}
}

// writeCapability tells the UI whether mutating actions reach aivc-agents.
func writeCapability(live bool) map[string]any {
	if live {
		return map[string]any{
			"enabled": true,
			"status":  "live",
			"message": "AI generation, triage and approvals run on aivc-agents.",
		}
	}
	return map[string]any{
		"enabled": false,
		"status":  "needs_runtime",
		"message": "AI invoice generation, triage, month-end and approvals need the aivc-agents runtime (set AIVC_BASE_URL). The mailbox below is read-only demo data.",
	}
}

func demoWriteError(action string) error {
	return &Error{
		Status: http.StatusConflict,
		Msg:    action + " needs the Credit Controller runtime (aivc-agents). This environment is running on demo fixtures — set AIVC_BASE_URL to enable it. Nothing was changed.",
	}
}

func vatNumber(supplier string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(supplier); i++ {
		h = (h ^ uint32(supplier[i])) * 16777619
	}
	n := h % 1000000000
	return fmt.Sprintf("%03d %04d %02d", n/1000000, (n/100)%10000, n%100)
}

func trimQty(q float64) string {
	if q == float64(int64(q)) {
		return fmt.Sprintf("%d", int64(q))
	}
	return fmt.Sprintf("%.2f", q)
}

func roundPence(v float64) float64 {
	if v < 0 {
		return float64(int64(v*100-0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}
