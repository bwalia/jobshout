package metalead

// DMs, comments and @mentions on a connected Facebook Page and its linked
// Instagram professional account, arriving by webhook. Each becomes a
// meta_leads row — one per DM thread, comment or mention — that the LLM
// qualifies; real leads with an email or phone sync into OpsAPI CRM exactly as
// form leads do.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

// webhookPayload is one Meta webhook delivery: object "page" for a Facebook
// Page, "instagram" for an Instagram professional account.
type webhookPayload struct {
	Object string         `json:"object"`
	Entry  []webhookEntry `json:"entry"`
}

type webhookEntry struct {
	ID        string           `json:"id"` // the Page id, or the Instagram account id
	Messaging []messagingEvent `json:"messaging"`
	Changes   []struct {
		Field string `json:"field"`
		// Decoded per change, so one odd value cannot drop the whole delivery.
		Value json.RawMessage `json:"value"`
	} `json:"changes"`
}

type messagingEvent struct {
	Sender struct {
		ID string `json:"id"`
	} `json:"sender"`
	Message *struct {
		MID       string `json:"mid"`
		Text      string `json:"text"`
		IsEcho    bool   `json:"is_echo"`
		IsSelf    bool   `json:"is_self"`
		IsDeleted bool   `json:"is_deleted"`
	} `json:"message"`
}

// changeValue is the union of the change values handled here.
type changeValue struct {
	// leadgen
	LeadgenID string `json:"leadgen_id"`
	PageID    string `json:"page_id"`
	FormID    string `json:"form_id"`
	// Facebook feed and mention
	Item      string `json:"item"`
	Verb      string `json:"verb"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Message   string `json:"message"`
	// Instagram comments and mentions
	ID      string `json:"id"`
	Text    string `json:"text"`
	MediaID string `json:"media_id"`
	From    struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"from"`
}

// interaction is one DM, comment or mention, the same shape for both networks.
type interaction struct {
	AccountID  string // the Page or Instagram account the entry names
	Source     string // model.MetaLeadSource*
	Key        string // meta_leads.leadgen_id: the DM thread, or the comment/post/media id
	EventID    string // the message or comment, so a retried delivery is skipped
	AuthorID   string
	AuthorName string
	Text       string
	Permalink  string
	At         *time.Time // when it was sent; set by backfill only
	// Meta sends a mention without its text; these say where to read it.
	CommentID, MediaID string
}

// interactionsFrom lists the interactions in one webhook entry. The business's
// own messages and comments are skipped: they are replies, not leads.
func interactionsFrom(object string, e webhookEntry) []interaction {
	ig := object == "instagram"
	var out []interaction
	for _, m := range e.Messaging {
		msg := m.Message
		if msg == nil || msg.IsEcho || msg.IsSelf || msg.IsDeleted ||
			m.Sender.ID == "" || m.Sender.ID == e.ID || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		src := model.MetaLeadSourceFBDM
		if ig {
			src = model.MetaLeadSourceIGDM
		}
		out = append(out, interaction{
			AccountID: e.ID, Source: src, Key: "dm:" + e.ID + ":" + m.Sender.ID,
			EventID: msg.MID, AuthorID: m.Sender.ID, Text: msg.Text,
		})
	}
	for _, ch := range e.Changes {
		var v changeValue
		if json.Unmarshal(ch.Value, &v) != nil || (v.From.ID != "" && v.From.ID == e.ID) {
			continue
		}
		it := interaction{AccountID: e.ID, AuthorID: v.From.ID, AuthorName: firstNonEmpty(v.From.Username, v.From.Name)}
		switch {
		case !ig && ch.Field == "feed":
			// A comment, or a visitor's post on the Page.
			if v.Verb != "add" || (v.Item != "comment" && v.Item != "post") {
				continue
			}
			it.Source, it.Key, it.Text = model.MetaLeadSourceFBComment, firstNonEmpty(v.CommentID, v.PostID), v.Message
			if v.PostID != "" {
				it.Permalink = "https://www.facebook.com/" + v.PostID
			}
		case !ig && ch.Field == "mention":
			if v.Verb != "add" {
				continue
			}
			it.Source, it.Key, it.Text = model.MetaLeadSourceFBMention, firstNonEmpty(v.CommentID, v.PostID), v.Message
		case ig && (ch.Field == "comments" || ch.Field == "live_comments"):
			// Instagram Login sends the comment as id, Facebook Login as comment_id.
			it.Source, it.Key, it.Text = model.MetaLeadSourceIGComment, firstNonEmpty(v.ID, v.CommentID), v.Text
		case ig && ch.Field == "mentions":
			it.Source, it.Key = model.MetaLeadSourceIGMention, firstNonEmpty(v.CommentID, v.MediaID)
			it.CommentID, it.MediaID = v.CommentID, v.MediaID
		default:
			continue
		}
		if it.Key == "" {
			continue
		}
		it.EventID = it.Key
		out = append(out, it)
	}
	return out
}

// processInteraction stores, qualifies and (when it is a lead) syncs one
// interaction from a webhook.
func (s *Service) processInteraction(ctx context.Context, it interaction) error {
	conn, err := s.store.GetConnectionByAccount(ctx, it.AccountID)
	if err != nil {
		return err
	}
	token, err := s.decryptToken(conn.AccessTokenEnc)
	if err != nil {
		return err
	}
	lead, brief, err := s.qualifyInteraction(ctx, conn, token, it, nil, false)
	if err != nil || lead == nil {
		return err
	}
	return s.syncLeadCRM(ctx, conn.OrgID, lead.ID, contactFields(lead.Body, lead.AuthorName), brief, "", false)
}

// qualifyInteraction stores and qualifies one interaction. A nil lead means
// there was nothing new: a retried event, or a mention Graph would not show.
// replace makes it.Text the whole body, for a backfilled DM thread that
// already holds the messages a webhook may have stored.
func (s *Service) qualifyInteraction(ctx context.Context, conn *connRow, token string, it interaction, runID *uuid.UUID, replace bool) (*model.MetaLead, *model.MetaLeadBrief, error) {
	if strings.TrimSpace(it.Text) == "" {
		s.readMention(ctx, &it, token)
	}
	if strings.TrimSpace(it.Text) == "" {
		return nil, nil, nil
	}
	if it.AuthorName == "" && it.AuthorID != "" {
		it.AuthorName = s.authorName(ctx, conn.OrgID, it, token)
	}
	lead, err := s.store.UpsertInteraction(ctx, &model.MetaLead{
		OrgID: conn.OrgID, ConnectionID: conn.ID, RunID: runID, LeadgenID: it.Key, Source: it.Source,
		AuthorID: it.AuthorID, AuthorName: it.AuthorName, Body: it.Text, Permalink: it.Permalink,
		CreatedTime: it.At, Status: model.MetaLeadStatusAnalysing,
	}, it.EventID, replace)
	if errors.Is(err, ErrDuplicateEvent) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	brief, err := s.analyseInteraction(ctx, lead)
	if err != nil {
		_ = s.store.UpdateLeadAnalysis(ctx, conn.OrgID, lead.ID, nil, model.MetaLeadStatusFailed, err.Error())
		return nil, nil, err
	}
	raw, _ := json.Marshal(brief)
	if err := s.store.UpdateLeadAnalysis(ctx, conn.OrgID, lead.ID, raw, model.MetaLeadStatusAnalysed, ""); err != nil {
		return nil, nil, err
	}
	return lead, brief, nil
}

func (s *Service) analyseInteraction(ctx context.Context, lead *model.MetaLead) (*model.MetaLeadBrief, error) {
	b := &model.MetaLeadBrief{
		Intent: "unknown", FitScore: 50, NextAction: "Review manually", SpamRisk: "low",
		Flags: []string{}, Summary: truncate(lead.Body, 120),
	}
	if s.analyse != nil {
		var err error
		if b, err = s.analyse.AnalyseInteraction(ctx, lead.Source, lead.AuthorName, lead.Body); err != nil {
			return nil, err
		}
	}
	groundEvidence(b, lead.Body)
	return b, nil
}

// readMention fills in a mention's text, which its webhook leaves out. Graph
// may refuse a mention on a profile we cannot see; that one is skipped.
func (s *Service) readMention(ctx context.Context, it *interaction, token string) {
	var err error
	switch {
	case it.Source == model.MetaLeadSourceFBMention:
		var v struct {
			Message   string `json:"message"`
			Permalink string `json:"permalink_url"`
			From      struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"from"`
		}
		if err = s.graph.Get(ctx, it.Key, fields("message,from,permalink_url"), token, &v); err == nil {
			it.Text, it.Permalink = v.Message, v.Permalink
			if it.AuthorID == "" {
				it.AuthorID, it.AuthorName = v.From.ID, v.From.Name
			}
		}
	case it.Source == model.MetaLeadSourceIGMention && it.CommentID != "":
		var v struct {
			Comment struct {
				Text string `json:"text"`
			} `json:"mentioned_comment"`
		}
		if err = s.graph.Get(ctx, it.AccountID, fields("mentioned_comment.comment_id("+it.CommentID+"){text}"), token, &v); err == nil {
			it.Text = v.Comment.Text
		}
	case it.Source == model.MetaLeadSourceIGMention && it.MediaID != "":
		var v struct {
			Media struct {
				Caption  string `json:"caption"`
				Username string `json:"username"`
			} `json:"mentioned_media"`
		}
		if err = s.graph.Get(ctx, it.AccountID, fields("mentioned_media.media_id("+it.MediaID+"){caption,username}"), token, &v); err == nil {
			it.Text, it.AuthorName = v.Media.Caption, v.Media.Username
		}
	}
	if err != nil {
		s.log.Warn("meta mention read", zap.String("source", it.Source), zap.String("key", it.Key), zap.Error(err))
	}
}

// authorName names a sender the webhook identifies only by id (every DM),
// asking Graph once per thread.
func (s *Service) authorName(ctx context.Context, orgID uuid.UUID, it interaction, token string) string {
	if prev, err := s.store.GetLeadByLeadgenID(ctx, orgID, it.Key); err == nil && prev.AuthorName != "" {
		return prev.AuthorName
	}
	f := "name"
	if it.Source == model.MetaLeadSourceIGDM {
		f = "name,username"
	}
	var p struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := s.graph.Get(ctx, it.AuthorID, fields(f), token, &p); err != nil {
		s.log.Debug("meta profile read", zap.String("source", it.Source), zap.Error(err))
		return ""
	}
	return firstNonEmpty(p.Username, p.Name)
}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	phoneRe = regexp.MustCompile(`\+?\d[\d ().-]{8,}\d`)
)

// contactFields is how a message says its author can be reached, in the
// field_data shape the CRM push reads. Contact details come from the text by
// pattern, never from the model, so none can be invented.
func contactFields(body, author string) []FieldData {
	var out []FieldData
	if e := emailRe.FindString(body); e != "" {
		out = append(out, FieldData{Name: "email", Values: []string{e}})
	}
	// 10–15 digits keeps prices and order numbers out.
	// ponytail: an ISO date and time ("2026-11-02 10") also has 10; add a
	// libphonenumber check if those show up as phones.
	for _, p := range phoneRe.FindAllString(body, -1) {
		if n := countDigits(p); n >= 10 && n <= 15 {
			out = append(out, FieldData{Name: "phone", Values: []string{p}})
			break
		}
	}
	if author != "" {
		out = append(out, FieldData{Name: "full_name", Values: []string{author}})
	}
	return out
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// groundEvidence keeps the brief's evidence only when the message contains
// it. The quote is there so a reviewer can trust the verdict at a glance; one
// the model made up would do the opposite.
func groundEvidence(b *model.MetaLeadBrief, body string) {
	q := strings.Trim(strings.TrimSpace(b.Evidence), `"'“”‘’`)
	switch {
	case q == "":
		b.Evidence = ""
	case strings.Contains(squash(body), squash(q)):
		b.Evidence = q
	default:
		b.Evidence = ""
		b.Flags = append(b.Flags, "evidence_not_in_message")
	}
}

func squash(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
