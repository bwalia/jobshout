package simpro

import (
	"time"
)

// Invoice is a normalised AR invoice (demo or live).
type Invoice struct {
	ID           string  `json:"id"`
	Number       string  `json:"number"`
	Customer     string  `json:"customer"`
	Site         string  `json:"site"`
	JobRef       string  `json:"job_ref"`
	IssuedOn     string  `json:"issued_on"`
	DueOn        string  `json:"due_on"`
	AmountGBP    float64 `json:"amount_gbp"`
	PaidGBP      float64 `json:"paid_gbp"`
	BalanceGBP   float64 `json:"balance_gbp"`
	Status       string  `json:"status"` // open | partial | paid | overdue
	AgingBucket  string  `json:"aging_bucket"`
	Source       string  `json:"source"` // demo | simpro
}

// Payment is a receipt / payment allocation.
type Payment struct {
	ID         string  `json:"id"`
	InvoiceID  string  `json:"invoice_id"`
	Customer   string  `json:"customer"`
	ReceivedOn string  `json:"received_on"`
	AmountGBP  float64 `json:"amount_gbp"`
	Method     string  `json:"method"`
	Reference  string  `json:"reference"`
	Source     string  `json:"source"`
}

// Job is a light work-order summary.
type Job struct {
	ID       string `json:"id"`
	Number   string `json:"number"`
	Customer string `json:"customer"`
	Site     string `json:"site"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Source   string `json:"source"`
}

// RefrigerantUsage is one F-Gas ledger row.
type RefrigerantUsage struct {
	ID            string  `json:"id"`
	Date          string  `json:"date"`
	JobRef        string  `json:"job_ref"`
	Customer      string  `json:"customer"`
	Site          string  `json:"site"`
	Technician    string  `json:"technician"`
	GasType       string  `json:"gas_type"`
	KgUsed        float64 `json:"kg_used"`
	KgRecovered   float64 `json:"kg_recovered"`
	KgToppedUp    float64 `json:"kg_topped_up"`
	CylinderRef   string  `json:"cylinder_ref"`
	Notes         string  `json:"notes"`
	HasGap        bool    `json:"has_gap"`
	Source        string  `json:"source"`
}

// Summary is the AR + F-Gas dashboard payload.
type Summary struct {
	Role           string             `json:"role"`
	Period         string             `json:"period"`
	Mode           string             `json:"mode"` // demo | live | live_fallback
	Company        string             `json:"company"`
	Mailbox        MailboxStats       `json:"mailbox"`
	Aging          map[string]float64 `json:"aging"`
	Payments       PaymentStats       `json:"payments"`
	FGas           FGasStats          `json:"fgas"`
	Playbook       []PlaybookStep     `json:"playbook"`
	WriteActions   WriteCapability    `json:"write_actions"`
	GeneratedAt    string             `json:"generated_at"`
}

type MailboxStats struct {
	TotalInvoices int     `json:"total_invoices"`
	Open          int     `json:"open"`
	Overdue       int     `json:"overdue"`
	TotalGBP      float64 `json:"total_gbp"`
	OutstandingGBP float64 `json:"outstanding_gbp"`
}

type PaymentStats struct {
	ReceivedThisPeriod int     `json:"received_this_period"`
	ReceivedGBP        float64 `json:"received_gbp"`
	UnallocatedGBP     float64 `json:"unallocated_gbp"`
}

type FGasStats struct {
	Events           int                `json:"events"`
	Gaps             int                `json:"gaps"`
	KgUsed           float64            `json:"kg_used"`
	KgRecovered      float64            `json:"kg_recovered"`
	ByGasType        map[string]float64 `json:"by_gas_type"`
	Narrative        string             `json:"narrative"`
}

type PlaybookStep struct {
	Step   int    `json:"step"`
	Task   string `json:"task"`
	Detail string `json:"detail"`
}

// WriteCapability documents that Simpro write-back is talk-track only.
type WriteCapability struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func demoWriteCapability() WriteCapability {
	return WriteCapability{
		Enabled: false,
		Status:  "discussed_not_shipped",
		Message: "Pushing invoices, receipts, or stock adjustments into Simpro is available with an API key and will be scoped with the customer — not shipped in this demo.",
	}
}

func demoCompany() string {
	return "Watford Climate Services Ltd (demo)"
}

func demoPeriod() string {
	return time.Now().UTC().Format("2006-01") + " (demo)"
}

func DemoInvoices() []Invoice {
	return []Invoice{
		{ID: "inv-10421", Number: "INV-10421", Customer: "Hemel Retail Park Ltd", Site: "Unit 4 Cold Store", JobRef: "J-8821", IssuedOn: "2026-08-12", DueOn: "2026-09-11", AmountGBP: 4280, PaidGBP: 0, BalanceGBP: 4280, Status: "overdue", AgingBucket: "1-30", Source: "demo"},
		{ID: "inv-10458", Number: "INV-10458", Customer: "St Albans Academy Trust", Site: "Science Block AHU", JobRef: "J-8890", IssuedOn: "2026-08-28", DueOn: "2026-09-27", AmountGBP: 1960, PaidGBP: 980, BalanceGBP: 980, Status: "partial", AgingBucket: "current", Source: "demo"},
		{ID: "inv-10490", Number: "INV-10490", Customer: "Watford General FM", Site: "Theatre Suite VRF", JobRef: "J-8912", IssuedOn: "2026-09-01", DueOn: "2026-10-01", AmountGBP: 8750, PaidGBP: 0, BalanceGBP: 8750, Status: "open", AgingBucket: "current", Source: "demo"},
		{ID: "inv-10310", Number: "INV-10310", Customer: "Maple Grove Care Home", Site: "Plant Room", JobRef: "J-8702", IssuedOn: "2026-07-05", DueOn: "2026-08-04", AmountGBP: 3120, PaidGBP: 0, BalanceGBP: 3120, Status: "overdue", AgingBucket: "31-60", Source: "demo"},
		{ID: "inv-10201", Number: "INV-10201", Customer: "Oxhey Leisure Centre", Site: "Pool Dehum", JobRef: "J-8511", IssuedOn: "2026-05-18", DueOn: "2026-06-17", AmountGBP: 5400, PaidGBP: 0, BalanceGBP: 5400, Status: "overdue", AgingBucket: "61-90", Source: "demo"},
		{ID: "inv-10155", Number: "INV-10155", Customer: "Cassiobury Estates", Site: "Block C Rooftop", JobRef: "J-8400", IssuedOn: "2026-03-02", DueOn: "2026-04-01", AmountGBP: 2100, PaidGBP: 0, BalanceGBP: 2100, Status: "overdue", AgingBucket: "90+", Source: "demo"},
		{ID: "inv-10499", Number: "INV-10499", Customer: "Leavesden Studios", Site: "Stage 7 Chiller", JobRef: "J-8930", IssuedOn: "2026-09-03", DueOn: "2026-10-03", AmountGBP: 12640, PaidGBP: 12640, BalanceGBP: 0, Status: "paid", AgingBucket: "paid", Source: "demo"},
		{ID: "inv-10405", Number: "INV-10405", Customer: "Bushey Dental Group", Site: "Surgery AC", JobRef: "J-8788", IssuedOn: "2026-08-01", DueOn: "2026-08-31", AmountGBP: 890, PaidGBP: 890, BalanceGBP: 0, Status: "paid", AgingBucket: "paid", Source: "demo"},
	}
}

func DemoPayments() []Payment {
	return []Payment{
		{ID: "pay-901", InvoiceID: "inv-10499", Customer: "Leavesden Studios", ReceivedOn: "2026-09-04", AmountGBP: 12640, Method: "BACS", Reference: "LS-8930", Source: "demo"},
		{ID: "pay-902", InvoiceID: "inv-10405", Customer: "Bushey Dental Group", ReceivedOn: "2026-09-01", AmountGBP: 890, Method: "Card", Reference: "BDG-8788", Source: "demo"},
		{ID: "pay-903", InvoiceID: "inv-10458", Customer: "St Albans Academy Trust", ReceivedOn: "2026-09-02", AmountGBP: 980, Method: "BACS", Reference: "SAAT-8890-PART", Source: "demo"},
		{ID: "pay-904", InvoiceID: "", Customer: "Unallocated remittance", ReceivedOn: "2026-09-05", AmountGBP: 450, Method: "BACS", Reference: "UNKNOWN-REF", Source: "demo"},
	}
}

func DemoJobs() []Job {
	return []Job{
		{ID: "job-8821", Number: "J-8821", Customer: "Hemel Retail Park Ltd", Site: "Unit 4 Cold Store", Status: "Complete", Type: "Reactive", Source: "demo"},
		{ID: "job-8890", Number: "J-8890", Customer: "St Albans Academy Trust", Site: "Science Block AHU", Status: "Invoiced", Type: "PPM", Source: "demo"},
		{ID: "job-8912", Number: "J-8912", Customer: "Watford General FM", Site: "Theatre Suite VRF", Status: "In Progress", Type: "Install", Source: "demo"},
		{ID: "job-8930", Number: "J-8930", Customer: "Leavesden Studios", Site: "Stage 7 Chiller", Status: "Complete", Type: "Reactive", Source: "demo"},
		{ID: "job-8944", Number: "J-8944", Customer: "Cassiobury Estates", Site: "Block C Rooftop", Status: "Scheduled", Type: "F-Gas Service", Source: "demo"},
	}
}

func DemoFGas() []RefrigerantUsage {
	return []RefrigerantUsage{
		{ID: "fg-1", Date: "2026-08-14", JobRef: "J-8821", Customer: "Hemel Retail Park Ltd", Site: "Unit 4 Cold Store", Technician: "A. Patel", GasType: "R404A", KgUsed: 2.4, KgRecovered: 1.1, KgToppedUp: 1.3, CylinderRef: "CYL-R404-12", Notes: "Leak repair + top-up", Source: "demo"},
		{ID: "fg-2", Date: "2026-08-22", JobRef: "J-8890", Customer: "St Albans Academy Trust", Site: "Science Block AHU", Technician: "M. Okonkwo", GasType: "R32", KgUsed: 0.8, KgRecovered: 0, KgToppedUp: 0.8, CylinderRef: "CYL-R32-04", Notes: "Commission top-up", Source: "demo"},
		{ID: "fg-3", Date: "2026-08-29", JobRef: "J-8912", Customer: "Watford General FM", Site: "Theatre Suite VRF", Technician: "A. Patel", GasType: "R410A", KgUsed: 4.2, KgRecovered: 3.9, KgToppedUp: 0.3, CylinderRef: "CYL-R410-09", Notes: "Recovery during board swap", Source: "demo"},
		{ID: "fg-4", Date: "2026-09-02", JobRef: "J-8930", Customer: "Leavesden Studios", Site: "Stage 7 Chiller", Technician: "S. Hughes", GasType: "R134a", KgUsed: 6.0, KgRecovered: 5.5, KgToppedUp: 0.5, CylinderRef: "CYL-R134-02", Notes: "Annual service", Source: "demo"},
		{ID: "fg-5", Date: "2026-09-03", JobRef: "J-8944", Customer: "Cassiobury Estates", Site: "Block C Rooftop", Technician: "M. Okonkwo", GasType: "R410A", KgUsed: 0, KgRecovered: 0, KgToppedUp: 0, CylinderRef: "", Notes: "Visit logged without cylinder / kg — spreadsheet gap", HasGap: true, Source: "demo"},
		{ID: "fg-6", Date: "2026-07-19", JobRef: "J-8702", Customer: "Maple Grove Care Home", Site: "Plant Room", Technician: "S. Hughes", GasType: "R32", KgUsed: 1.5, KgRecovered: 0.2, KgToppedUp: 1.3, CylinderRef: "CYL-R32-04", Notes: "Leak find", Source: "demo"},
		{ID: "fg-7", Date: "2026-06-08", JobRef: "J-8511", Customer: "Oxhey Leisure Centre", Site: "Pool Dehum", Technician: "A. Patel", GasType: "R410A", KgUsed: 0, KgRecovered: 0, KgToppedUp: 0, CylinderRef: "", Notes: "Technician notes only in Excel — no Simpro stock move", HasGap: true, Source: "demo"},
	}
}

func DemoPlaybook() []PlaybookStep {
	return []PlaybookStep{
		{Step: 1, Task: "Pull open invoices", Detail: "Age AR and flag overdue buckets from Simpro (or demo cache)."},
		{Step: 2, Task: "Match remittances", Detail: "Recommend allocations; write-back discussed with customer, not shipped."},
		{Step: 3, Task: "Chase overdue", Detail: "Draft chase notes by bucket — human sends until write APIs are scoped."},
		{Step: 4, Task: "F-Gas ledger", Detail: "Capture refrigerant usage by job/site/tech; highlight missing kg/cylinder rows."},
		{Step: 5, Task: "Period close", Detail: "Month-end checklist: aging, unallocated cash, F-Gas gaps, export pack."},
	}
}

func BuildSummary(mode, company, period string, invoices []Invoice, payments []Payment, fgas []RefrigerantUsage) Summary {
	mailbox := MailboxStats{}
	aging := map[string]float64{"current": 0, "1-30": 0, "31-60": 0, "61-90": 0, "90+": 0}
	for _, inv := range invoices {
		mailbox.TotalInvoices++
		mailbox.TotalGBP += inv.AmountGBP
		if inv.BalanceGBP > 0 {
			mailbox.OutstandingGBP += inv.BalanceGBP
			mailbox.Open++
			if inv.Status == "overdue" {
				mailbox.Overdue++
			}
			if _, ok := aging[inv.AgingBucket]; ok {
				aging[inv.AgingBucket] += inv.BalanceGBP
			}
		}
	}
	payStats := PaymentStats{}
	for _, p := range payments {
		payStats.ReceivedThisPeriod++
		payStats.ReceivedGBP += p.AmountGBP
		if p.InvoiceID == "" {
			payStats.UnallocatedGBP += p.AmountGBP
		}
	}
	fg := FGasStats{ByGasType: map[string]float64{}}
	for _, e := range fgas {
		fg.Events++
		fg.KgUsed += e.KgUsed
		fg.KgRecovered += e.KgRecovered
		fg.ByGasType[e.GasType] += e.KgUsed
		if e.HasGap {
			fg.Gaps++
		}
	}
	fg.Narrative = "UK F-Gas compliance is currently split across Simpro jobs and technician spreadsheets. This agent consolidates usage, recovery, and top-ups into an audit-ready ledger and flags missing records."
	if company == "" {
		company = demoCompany()
	}
	if period == "" {
		period = demoPeriod()
	}
	return Summary{
		Role:         "Simpro Payments & Invoicing",
		Period:       period,
		Mode:         mode,
		Company:      company,
		Mailbox:      mailbox,
		Aging:        aging,
		Payments:     payStats,
		FGas:         fg,
		Playbook:     DemoPlaybook(),
		WriteActions: demoWriteCapability(),
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}
