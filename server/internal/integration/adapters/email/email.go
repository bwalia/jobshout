// Package email provides a NotificationAdapter that sends alerts via SMTP.
package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
)

// Adapter sends notifications via SMTP email.
type Adapter struct {
	smtpHost string
	smtpPort string
	from     string
	password string
	to       []string
}

// NewAdapter constructs an email adapter from a NotificationConfig.
// Expected config keys: smtp_host, smtp_port, from, password, to (comma-separated).
func NewAdapter(cfg model.NotificationConfig) integration.NotificationAdapter {
	getStr := func(key, fallback string) string {
		if v, ok := cfg.Config[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
		return fallback
	}

	to := strings.Split(getStr("to", ""), ",")
	var trimmed []string
	for _, addr := range to {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			trimmed = append(trimmed, addr)
		}
	}

	return &Adapter{
		smtpHost: getStr("smtp_host", "localhost"),
		smtpPort: getStr("smtp_port", "587"),
		from:     getStr("from", "noreply@jobshout.io"),
		password: getStr("password", ""),
		to:       trimmed,
	}
}

func (a *Adapter) Name() string { return "email" }

func (a *Adapter) Send(_ context.Context, msg integration.NotificationMessage) error {
	if len(a.to) == 0 {
		return fmt.Errorf("email adapter: no recipients configured")
	}

	subject := fmt.Sprintf("[JobShout] %s: %s", msg.EventType, msg.TaskTitle)
	body := buildBody(msg)

	mime := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		a.from, strings.Join(a.to, ", "), subject, body,
	)

	addr := a.smtpHost + ":" + a.smtpPort
	var auth smtp.Auth
	if a.password != "" {
		auth = smtp.PlainAuth("", a.from, a.password, a.smtpHost)
	}

	return smtp.SendMail(addr, auth, a.from, a.to, []byte(mime))
}

func (a *Adapter) Test(ctx context.Context) error {
	return a.Send(ctx, integration.NotificationMessage{
		EventType: "test",
		TaskTitle: "JobShout Email Test",
		Status:    "This is a test notification from JobShout",
	})
}

func buildBody(msg integration.NotificationMessage) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Event: %s\n", msg.EventType))
	b.WriteString(fmt.Sprintf("Task:  %s\n", msg.TaskTitle))
	b.WriteString(fmt.Sprintf("Status: %s\n", msg.Status))
	if msg.AgentName != "" {
		b.WriteString(fmt.Sprintf("Agent: %s\n", msg.AgentName))
	}
	if msg.Duration != "" {
		b.WriteString(fmt.Sprintf("Duration: %s\n", msg.Duration))
	}
	if msg.URL != "" {
		b.WriteString(fmt.Sprintf("URL: %s\n", msg.URL))
	}
	for k, v := range msg.Extra {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	return b.String()
}
