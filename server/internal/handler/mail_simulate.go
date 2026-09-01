package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jobshout/server/internal/mail"
)

type simulateInboxBody struct {
	Messages []simulateMessage `json:"messages"`
}

type simulateMessage struct {
	ThreadID string `json:"thread_id"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// SimulateConnect POST /api/v1/mail/simulate/connect — local MAIL_SIMULATE only.
func (h *MailHandler) SimulateConnect(w http.ResponseWriter, r *http.Request) {
	if !h.svc.SimulateEnabled() {
		RespondError(w, http.StatusNotFound, "mail simulate is off")
		return
	}
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	st, err := h.svc.ConnectSimulated(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, st)
}

// SimulateInbox POST /api/v1/mail/simulate/inbox — inject dummy emails.
func (h *MailHandler) SimulateInbox(w http.ResponseWriter, r *http.Request) {
	if !h.svc.SimulateEnabled() {
		RespondError(w, http.StatusNotFound, "mail simulate is off")
		return
	}
	if _, ok := h.orgID(w, r); !ok {
		return
	}
	var body simulateInboxBody
	if !DecodeJSON(w, r, &body) {
		return
	}
	if len(body.Messages) == 0 {
		RespondError(w, http.StatusBadRequest, "messages is required")
		return
	}
	now := time.Now()
	msgs := make([]mail.InboxMessage, 0, len(body.Messages))
	for i, m := range body.Messages {
		tid := m.ThreadID
		if tid == "" {
			tid = "sim-th-" + time.Now().Format("150405") + "-" + strconv.Itoa(i)
		}
		to := m.To
		if to == "" {
			to = "sim@jobshout.local"
		}
		from := m.From
		if from == "" {
			from = "sender@example.com"
		}
		msgs = append(msgs, mail.InboxMessage{
			GmailThreadID:  tid,
			GmailMessageID: tid + "-m1",
			FromEmail:      from,
			FromName:       m.FromName,
			ToEmail:        to,
			Subject:        m.Subject,
			Snippet:        clip(m.Body, 120),
			Body:           m.Body,
			ReceivedAt:     now,
		})
	}
	if err := h.svc.PushSimulatedInbox(msgs); err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusAccepted, map[string]any{"injected": len(msgs)})
}

// SimulateSync POST /api/v1/mail/simulate/sync — process the fake inbox now.
func (h *MailHandler) SimulateSync(w http.ResponseWriter, r *http.Request) {
	if !h.svc.SimulateEnabled() {
		RespondError(w, http.StatusNotFound, "mail simulate is off")
		return
	}
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	if err := h.svc.SyncNow(r.Context(), orgID); err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
