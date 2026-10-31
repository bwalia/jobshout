package metalead

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

// Tasks is the board-task surface launches need (optional).
type Tasks interface {
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
}

// Service is the Meta Lead Analyst.
type Service struct {
	store  Store
	graph  Graph
	analyse *Analyser
	cfg    Config
	log    *zap.Logger
	tasks Tasks
}

// NewService wires the agent.
func NewService(store Store, graph Graph, analyse *Analyser, cfg Config, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{store: store, graph: graph, analyse: analyse, cfg: cfg, log: log}
}

// BindTasks enables board task status updates from a launch.
func (s *Service) BindTasks(t Tasks) { s.tasks = t }

// Configured reports webhook + sealing readiness.
func (s *Service) Configured() bool {
	return s != nil && s.cfg.Configured()
}

// Launch starts an async backfill+analyse run.
func (s *Service) Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error) {
	if s == nil || s.store == nil || s.graph == nil {
		return nil, fmt.Errorf("the Meta Lead Analyst is not configured")
	}
	if !s.cfg.CanSealTokens() {
		return nil, fmt.Errorf("META_TOKEN_KEY is not set — cannot use Page connections")
	}
	conn, err := s.store.GetConnectionByName(ctx, req.OrgID, req.Connection)
	if err != nil {
		return nil, err
	}
	token, err := s.decryptToken(conn.AccessTokenEnc)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > s.cfg.MaxLeadsPerLaunch {
		limit = s.cfg.MaxLeadsPerLaunch
	}
	run := &model.MetaLeadRun{
		OrgID:        req.OrgID,
		TaskID:       req.TaskID,
		ConnectionID: conn.ID,
		FormID:       strings.TrimSpace(req.Form),
		Focus:        req.Focus,
		Status:       model.MetaLeadRunStatusRunning,
		Source:       "launch",
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	go s.runLaunch(context.Background(), run.ID, req, conn, token, limit)
	msg := fmt.Sprintf("Fetching and analysing Meta leads for connection %q", req.Connection)
	return &LaunchResult{RunID: run.ID, Message: msg}, nil
}

func (s *Service) runLaunch(ctx context.Context, runID uuid.UUID, req LaunchRequest, conn *connRow, token string, limit int) {
	timeout := s.cfg.AnalyseTimeout
	if timeout <= 0 {
		timeout = 8 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run, err := s.store.GetRun(ctx, req.OrgID, runID)
	if err != nil {
		s.log.Error("meta lead run missing", zap.Error(err))
		return
	}
	defer func() {
		if req.TaskID != nil && s.tasks != nil {
			status := "done"
			if run.Status == model.MetaLeadRunStatusFailed {
				status = "failed"
			}
			_ = s.tasks.Transition(context.Background(), *req.TaskID, status, &req.UserID)
		}
	}()

	since, err := parseSince(req.Since)
	if err != nil {
		s.failRun(ctx, run, err.Error())
		return
	}

	formIDs, formName, err := s.resolveForms(ctx, conn.PageID, token, req.Form)
	if err != nil {
		s.failRun(ctx, run, err.Error())
		return
	}
	run.FormName = formName
	if len(formIDs) == 1 {
		run.FormID = formIDs[0]
	}

	type tagged struct {
		LeadSummary
		FormID string
	}
	var taggedLeads []tagged
	remaining := limit
	for _, fid := range formIDs {
		if remaining <= 0 {
			break
		}
		leads, err := s.graph.ListLeads(ctx, fid, token, since, remaining)
		if err != nil {
			s.failRun(ctx, run, err.Error())
			return
		}
		for _, L := range leads {
			taggedLeads = append(taggedLeads, tagged{LeadSummary: L, FormID: fid})
		}
		remaining -= len(leads)
	}

	briefs := make([]model.MetaLeadBrief, 0, len(taggedLeads))
	for _, tl := range taggedLeads {
		fields := tl.FieldData
		formID := tl.FormID
		if len(fields) == 0 {
			detail, err := s.graph.GetLead(ctx, tl.ID, token)
			if err != nil {
				s.log.Warn("meta get lead", zap.String("id", tl.ID), zap.Error(err))
				continue
			}
			fields = detail.FieldData
			if formID == "" {
				formID = detail.FormID
			}
		}
		lead := &model.MetaLead{
			OrgID:        req.OrgID,
			ConnectionID: conn.ID,
			RunID:        &run.ID,
			LeadgenID:    tl.ID,
			FormID:       formID,
			FieldData:    FieldDataJSON(fields),
			CreatedTime:  ParseCreatedTime(tl.CreatedTime),
			Status:       model.MetaLeadStatusAnalysing,
		}
		if err := s.store.UpsertLead(ctx, lead); err != nil {
			s.log.Warn("meta upsert lead", zap.Error(err))
			continue
		}
		brief, err := s.analyseLead(ctx, fields, req.Focus)
		if err != nil {
			_ = s.store.UpdateLeadAnalysis(ctx, req.OrgID, lead.ID, nil, model.MetaLeadStatusFailed, err.Error())
			continue
		}
		raw, _ := json.Marshal(brief)
		_ = s.store.UpdateLeadAnalysis(ctx, req.OrgID, lead.ID, raw, model.MetaLeadStatusAnalysed, "")
		briefs = append(briefs, *brief)
	}

	run.LeadCount = len(briefs)
	if s.analyse != nil && len(briefs) > 0 {
		rollup, err := s.analyse.AnalyseRollup(ctx, briefs, req.Focus)
		if err != nil {
			s.log.Warn("meta rollup", zap.Error(err))
		} else if rollup != nil {
			run.Rollup, _ = json.Marshal(rollup)
		}
	}
	run.Status = model.MetaLeadRunStatusDone
	if err := s.store.UpdateRun(ctx, run); err != nil {
		s.log.Error("meta update run", zap.Error(err))
	}
}

func (s *Service) analyseLead(ctx context.Context, fields []FieldData, focus string) (*model.MetaLeadBrief, error) {
	if s.analyse == nil {
		return heuristicBrief(fields), nil
	}
	return s.analyse.AnalyseLead(ctx, fields, focus)
}

func heuristicBrief(fields []FieldData) *model.MetaLeadBrief {
	summary := "Lead captured"
	for _, f := range fields {
		if strings.EqualFold(f.Name, "email") && len(f.Values) > 0 {
			summary = "Lead: " + f.Values[0]
			break
		}
	}
	return &model.MetaLeadBrief{
		Intent: "unknown", FitScore: 50, NextAction: "Review manually",
		SpamRisk: "low", Flags: []string{}, Summary: summary,
	}
}

func (s *Service) resolveForms(ctx context.Context, pageID, token, formSpec string) (ids []string, name string, err error) {
	formSpec = strings.TrimSpace(formSpec)
	forms, err := s.graph.ListForms(ctx, pageID, token)
	if err != nil {
		return nil, "", err
	}
	if formSpec == "" {
		for _, f := range forms {
			ids = append(ids, f.ID)
		}
		if len(ids) == 0 {
			return nil, "", fmt.Errorf("no Lead Ads forms on this Page")
		}
		return ids, "", nil
	}
	// numeric id?
	if _, err := strconv.ParseUint(formSpec, 10, 64); err == nil {
		return []string{formSpec}, "", nil
	}
	lower := strings.ToLower(formSpec)
	for _, f := range forms {
		if strings.EqualFold(f.Name, formSpec) || strings.Contains(strings.ToLower(f.Name), lower) {
			return []string{f.ID}, f.Name, nil
		}
	}
	return nil, "", fmt.Errorf("lead form %q not found on this Page", formSpec)
}

func (s *Service) failRun(ctx context.Context, run *model.MetaLeadRun, msg string) {
	run.Status = model.MetaLeadRunStatusFailed
	run.ErrorMessage = msg
	_ = s.store.UpdateRun(ctx, run)
}

func (s *Service) decryptToken(enc []byte) (string, error) {
	key, err := mail.KeyFromSecret(s.cfg.TokenKey)
	if err != nil {
		return "", err
	}
	plain, err := mail.Decrypt(key, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt page token: %w", err)
	}
	return string(plain), nil
}

func (s *Service) encryptToken(token string) ([]byte, error) {
	key, err := mail.KeyFromSecret(s.cfg.TokenKey)
	if err != nil {
		return nil, err
	}
	return mail.Encrypt(key, []byte(token))
}

// UpsertConnection stores a named Page connection (token sealed).
func (s *Service) UpsertConnection(ctx context.Context, orgID, userID uuid.UUID, name, pageID, pageName, accessToken string) (*model.MetaLeadConnection, error) {
	if !s.cfg.CanSealTokens() {
		return nil, fmt.Errorf("META_TOKEN_KEY is not set")
	}
	name = strings.TrimSpace(name)
	pageID = strings.TrimSpace(pageID)
	accessToken = strings.TrimSpace(accessToken)
	if name == "" || pageID == "" || accessToken == "" {
		return nil, fmt.Errorf("name, page_id and access_token are required")
	}
	enc, err := s.encryptToken(accessToken)
	if err != nil {
		return nil, err
	}
	uid := userID
	return s.store.UpsertConnection(ctx, orgID, name, pageID, strings.TrimSpace(pageName), enc, &uid)
}

func (s *Service) ListConnections(ctx context.Context, orgID uuid.UUID) ([]model.MetaLeadConnection, error) {
	return s.store.ListConnections(ctx, orgID)
}

func (s *Service) ListRuns(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLeadRun, error) {
	return s.store.ListRuns(ctx, orgID, limit)
}

func (s *Service) GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.MetaLeadRun, error) {
	return s.store.GetRun(ctx, orgID, runID)
}

func (s *Service) ListLeadsByRun(ctx context.Context, orgID, runID uuid.UUID) ([]model.MetaLead, error) {
	return s.store.ListLeadsByRun(ctx, orgID, runID)
}

func (s *Service) ListRecentLeads(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MetaLead, error) {
	return s.store.ListRecentLeads(ctx, orgID, limit)
}

func (s *Service) Status() map[string]any {
	return map[string]any{
		"configured":       s.Configured(),
		"can_seal_tokens":  s.cfg.CanSealTokens(),
		"graph_version":    s.cfg.GraphVersion,
		"webhook_verify":   s.cfg.WebhookVerify != "",
		"llm":              s.analyse != nil && s.analyse.LLM != nil,
	}
}

// VerifyWebhookChallenge handles Meta's GET subscription verification.
func (s *Service) VerifyWebhookChallenge(mode, token, challenge string) (string, bool) {
	if mode != "subscribe" {
		return "", false
	}
	if s.cfg.WebhookVerify == "" || token != s.cfg.WebhookVerify {
		return "", false
	}
	return challenge, true
}

// HandleWebhook processes a signed leadgen notification.
func (s *Service) HandleWebhook(ctx context.Context, raw []byte, signatureHeader string) error {
	if s.cfg.AppSecret == "" {
		return fmt.Errorf("META_APP_SECRET is not set")
	}
	if !validMetaSignature(s.cfg.AppSecret, raw, signatureHeader) {
		return fmt.Errorf("invalid Meta webhook signature")
	}
	var payload struct {
		Entry []struct {
			ID      string `json:"id"` // page id
			Changes []struct {
				Field string `json:"field"`
				Value struct {
					LeadgenID string `json:"leadgen_id"`
					PageID    string `json:"page_id"`
					FormID    string `json:"form_id"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("webhook JSON: %w", err)
	}
	for _, entry := range payload.Entry {
		pageID := entry.ID
		for _, ch := range entry.Changes {
			if ch.Field != "leadgen" {
				continue
			}
			if ch.Value.PageID != "" {
				pageID = ch.Value.PageID
			}
			if err := s.processLeadgen(ctx, pageID, ch.Value.LeadgenID, ch.Value.FormID); err != nil {
				s.log.Warn("meta webhook leadgen", zap.Error(err), zap.String("leadgen_id", ch.Value.LeadgenID))
			}
		}
	}
	return nil
}

func (s *Service) processLeadgen(ctx context.Context, pageID, leadgenID, formID string) error {
	if leadgenID == "" {
		return fmt.Errorf("empty leadgen_id")
	}
	conn, err := s.store.GetConnectionByPageID(ctx, pageID)
	if err != nil {
		return err
	}
	token, err := s.decryptToken(conn.AccessTokenEnc)
	if err != nil {
		return err
	}
	detail, err := s.graph.GetLead(ctx, leadgenID, token)
	if err != nil {
		return err
	}
	if formID == "" {
		formID = detail.FormID
	}
	run := &model.MetaLeadRun{
		OrgID:        conn.OrgID,
		ConnectionID: conn.ID,
		FormID:       formID,
		Status:       model.MetaLeadRunStatusRunning,
		Source:       "webhook",
		LeadCount:    1,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return err
	}
	lead := &model.MetaLead{
		OrgID:        conn.OrgID,
		ConnectionID: conn.ID,
		RunID:        &run.ID,
		LeadgenID:    detail.ID,
		FormID:       formID,
		FieldData:    FieldDataJSON(detail.FieldData),
		CreatedTime:  ParseCreatedTime(detail.CreatedTime),
		Status:       model.MetaLeadStatusAnalysing,
	}
	if err := s.store.UpsertLead(ctx, lead); err != nil {
		s.failRun(ctx, run, err.Error())
		return err
	}
	brief, err := s.analyseLead(ctx, detail.FieldData, "")
	if err != nil {
		_ = s.store.UpdateLeadAnalysis(ctx, conn.OrgID, lead.ID, nil, model.MetaLeadStatusFailed, err.Error())
		s.failRun(ctx, run, err.Error())
		return err
	}
	raw, _ := json.Marshal(brief)
	_ = s.store.UpdateLeadAnalysis(ctx, conn.OrgID, lead.ID, raw, model.MetaLeadStatusAnalysed, "")
	if s.analyse != nil {
		if rollup, err := s.analyse.AnalyseRollup(ctx, []model.MetaLeadBrief{*brief}, ""); err == nil && rollup != nil {
			run.Rollup, _ = json.Marshal(rollup)
		}
	}
	run.Status = model.MetaLeadRunStatusDone
	return s.store.UpdateRun(ctx, run)
}

func validMetaSignature(appSecret string, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

func parseSince(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if strings.HasSuffix(v, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid since %q", v)
		}
		t := time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour)
		return &t, nil
	}
	if strings.HasSuffix(v, "h") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "h"))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid since %q", v)
		}
		t := time.Now().UTC().Add(-time.Duration(n) * time.Hour)
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, fmt.Errorf("since must be RFC3339 or Nd/Nh: %w", err)
	}
	return &t, nil
}

// ReadBody is a small helper for handlers.
func ReadBody(r *http.Request, max int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, max))
}
