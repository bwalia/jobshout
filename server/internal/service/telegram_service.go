package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/telegram"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// TelegramService handles Telegram bot updates, user linking, and notifications.
type TelegramService interface {
	HandleUpdate(ctx context.Context, update *telegram.Update) error
	GenerateLinkToken(ctx context.Context, userID, orgID uuid.UUID) (*model.TelegramLinkToken, error)
	UnlinkUser(ctx context.Context, userID uuid.UUID) error
	GetLinkStatus(ctx context.Context, userID uuid.UUID) (*model.TelegramLinkStatusResponse, error)
	NotifyUser(ctx context.Context, telegramUserID int64, message string) error
}

// chatAPI is the session + turn surface Telegram needs. Implemented by
// chatsvc without this package importing the chat agent (avoids an import cycle).
type chatAPI interface {
	GetSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) (*model.ChatSession, error)
	CreateSession(ctx context.Context, session *model.ChatSession) error
	SendMessage(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source string) (*model.ChatMessage, *model.ChatMessage, error)
	SendWithConfirm(ctx context.Context, orgID, userID, sessionID uuid.UUID, content, source, confirmToken string) (*model.ChatTurnResult, error)
}

type telegramService struct {
	bot         *telegram.BotClient
	repo        repository.TelegramRepository
	chatSvc     chatAPI
	ratePerMin  int
	frontendURL string
	logger      *zap.Logger
}

func NewTelegramService(
	bot *telegram.BotClient,
	repo repository.TelegramRepository,
	chatSvc chatAPI,
	ratePerMin int,
	frontendURL string,
	logger *zap.Logger,
) TelegramService {
	return &telegramService{
		bot:         bot,
		repo:        repo,
		chatSvc:     chatSvc,
		ratePerMin:  ratePerMin,
		frontendURL: frontendURL,
		logger:      logger,
	}
}

func (s *telegramService) HandleUpdate(ctx context.Context, update *telegram.Update) error {
	// Handle callback queries (inline keyboard button presses).
	if update.CallbackQuery != nil {
		return s.handleCallback(ctx, update.CallbackQuery)
	}

	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}

	log := s.logger.With(
		zap.Int64("telegram_user_id", msg.From.ID),
		zap.String("telegram_username", msg.From.Username),
	)

	// Rate limit check.
	maxTokens := float64(s.ratePerMin)
	refillPerSec := maxTokens / 60.0
	allowed, err := s.repo.CheckRateLimit(ctx, msg.From.ID, maxTokens, refillPerSec)
	if err != nil {
		log.Warn("rate limit check failed", zap.Error(err))
	}
	if !allowed {
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Rate limit exceeded. Please try again in a moment.", nil)
		return nil
	}

	// Handle /start with linking token.
	if strings.HasPrefix(msg.Text, "/start ") {
		token := strings.TrimPrefix(msg.Text, "/start ")
		return s.handleStartLink(ctx, msg, token)
	}

	// Handle commands.
	switch msg.Text {
	case "/start":
		_ = s.bot.SendMessage(ctx, msg.Chat.ID,
			"Welcome to JobShout! Link your account by generating a token at "+
				s.frontendURL+"/settings and using /start <token>.", nil)
		return nil
	case "/help":
		return s.sendHelp(ctx, msg.Chat.ID)
	case "/unlink":
		return s.handleUnlink(ctx, msg)
	}

	// Look up mapping.
	mapping, err := s.repo.FindByTelegramID(ctx, msg.From.ID)
	if err != nil {
		_ = s.bot.SendMessage(ctx, msg.Chat.ID,
			"Your Telegram account is not linked to JobShout. "+
				"Generate a link token at "+s.frontendURL+"/settings then use /start <token>.", nil)
		return nil
	}

	// Send message through chat service.
	session, err := s.getOrCreateSession(ctx, mapping, msg.Chat.ID)
	if err != nil {
		log.Error("failed to get/create chat session", zap.Error(err))
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Internal error. Please try again.", nil)
		return err
	}

	_, agentMsg, err := s.chatSvc.SendMessage(ctx, mapping.OrgID, mapping.JobshoutUserID, session.ID, msg.Text, model.ChatSourceTelegram)
	if err != nil {
		log.Error("chat service error", zap.Error(err))
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Failed to process your message. Please try again.", nil)
		return err
	}

	response := agentMsg.Content
	if len(response) > 4000 {
		response = response[:4000] + "\n\n[truncated]"
	}

	var keyboard *telegram.InlineKeyboard
	if turn := envelopeFrom(agentMsg); turn != nil && turn.Confirmation != nil {
		keyboard = &telegram.InlineKeyboard{
			InlineKeyboard: [][]telegram.InlineButton{{
				{Text: "Approve", CallbackData: "chat_confirm:" + turn.Confirmation.Token},
				{Text: "Cancel", CallbackData: "chat_cancel:" + turn.Confirmation.Token},
			}},
		}
	}
	return s.bot.SendMessage(ctx, msg.Chat.ID, response, keyboard)
}

func (s *telegramService) GenerateLinkToken(ctx context.Context, userID, orgID uuid.UUID) (*model.TelegramLinkToken, error) {
	token := &model.TelegramLinkToken{
		UserID: userID,
		OrgID:  orgID,
	}
	if err := s.repo.StoreLinkToken(ctx, token); err != nil {
		return nil, fmt.Errorf("telegram_svc: generate_link_token: %w", err)
	}
	return token, nil
}

func (s *telegramService) UnlinkUser(ctx context.Context, userID uuid.UUID) error {
	mapping, err := s.repo.FindByJobshoutUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("telegram_svc: no linked account found")
	}
	return s.repo.DeleteMapping(ctx, mapping.ID)
}

func (s *telegramService) GetLinkStatus(ctx context.Context, userID uuid.UUID) (*model.TelegramLinkStatusResponse, error) {
	mapping, err := s.repo.FindByJobshoutUser(ctx, userID)
	if err != nil {
		return &model.TelegramLinkStatusResponse{Linked: false}, nil
	}
	return &model.TelegramLinkStatusResponse{
		Linked:           true,
		TelegramUsername: &mapping.TelegramUsername,
		TelegramUserID:   &mapping.TelegramUserID,
	}, nil
}

func (s *telegramService) NotifyUser(ctx context.Context, telegramUserID int64, message string) error {
	return s.bot.SendMessage(ctx, telegramUserID, message, nil)
}

func (s *telegramService) handleStartLink(ctx context.Context, msg *telegram.TelegramMessage, token string) error {
	linkToken, err := s.repo.ConsumeLinkToken(ctx, strings.TrimSpace(token))
	if err != nil {
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Invalid or expired link token. Please generate a new one.", nil)
		return nil
	}

	username := ""
	if msg.From != nil {
		username = msg.From.Username
	}

	mapping := &model.TelegramUserMapping{
		TelegramUserID:   msg.From.ID,
		TelegramUsername: username,
		JobshoutUserID:  linkToken.UserID,
		OrgID:           linkToken.OrgID,
		Verified:        true,
	}

	if err := s.repo.CreateMapping(ctx, mapping); err != nil {
		s.logger.Error("failed to create telegram mapping", zap.Error(err))
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Failed to link account. Please try again.", nil)
		return err
	}

	_ = s.bot.SendMessage(ctx, msg.Chat.ID,
		"Account linked successfully! You can now chat with your agents directly here.\n\nType /help to see available commands.", nil)
	return nil
}

func (s *telegramService) handleUnlink(ctx context.Context, msg *telegram.TelegramMessage) error {
	err := s.repo.DeleteByTelegramID(ctx, msg.From.ID)
	if err != nil {
		_ = s.bot.SendMessage(ctx, msg.Chat.ID, "No linked account found.", nil)
		return nil
	}
	_ = s.bot.SendMessage(ctx, msg.Chat.ID, "Account unlinked. Use /start <token> to link again.", nil)
	return nil
}

func (s *telegramService) handleCallback(ctx context.Context, cb *telegram.CallbackQuery) error {
	if cb == nil || cb.From == nil {
		return nil
	}
	data := cb.Data
	_ = s.bot.AnswerCallbackQuery(ctx, cb.ID, "Working…")

	mapping, err := s.repo.FindByTelegramID(ctx, cb.From.ID)
	if err != nil {
		return nil
	}
	chatID := cb.From.ID
	if cb.Message != nil && cb.Message.Chat != nil {
		chatID = cb.Message.Chat.ID
	}
	session, err := s.getOrCreateSession(ctx, mapping, chatID)
	if err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(data, "chat_confirm:"):
		token := strings.TrimPrefix(data, "chat_confirm:")
		turn, err := s.chatSvc.SendWithConfirm(ctx, mapping.OrgID, mapping.JobshoutUserID, session.ID, "yes", model.ChatSourceTelegram, token)
		if err != nil {
			_ = s.bot.SendMessage(ctx, chatID, "Couldn't complete that approval.", nil)
			return err
		}
		return s.bot.SendMessage(ctx, chatID, turn.Response.Message, nil)
	case strings.HasPrefix(data, "chat_cancel:"):
		turn, err := s.chatSvc.SendWithConfirm(ctx, mapping.OrgID, mapping.JobshoutUserID, session.ID, "cancel", model.ChatSourceTelegram, "")
		if err != nil {
			_ = s.bot.SendMessage(ctx, chatID, "Cancelled.", nil)
			return err
		}
		return s.bot.SendMessage(ctx, chatID, turn.Response.Message, nil)
	default:
		return nil
	}
}

func envelopeFrom(msg *model.ChatMessage) *model.ChatResponse {
	if msg == nil || msg.Metadata == nil {
		return nil
	}
	raw, ok := msg.Metadata["response"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case model.ChatResponse:
		return &v
	case *model.ChatResponse:
		return v
	case map[string]any:
		if c, ok := v["confirmation"].(map[string]any); ok {
			token, _ := c["token"].(string)
			if token == "" {
				return nil
			}
			sum, _ := c["summary"].(string)
			effect, _ := c["effect"].(string)
			tool, _ := c["tool"].(string)
			return &model.ChatResponse{
				Message: msg.Content,
				Confirmation: &model.ConfirmRequest{Token: token, Tool: tool, Summary: sum, Effect: effect},
			}
		}
	}
	return nil
}

func (s *telegramService) sendHelp(ctx context.Context, chatID int64) error {
	help := `JobShout Bot Commands:

/start <token> — Link your Telegram to JobShout
/unlink — Unlink your account
/help — Show this message

Or just type naturally:
- "list my agents"
- "create a task to fix the login timeout"
- "run the release check workflow"

Web chats and Telegram chats are separate sessions for the same account, so each keeps its own history.`

	return s.bot.SendMessage(ctx, chatID, help, nil)
}

func (s *telegramService) getOrCreateSession(ctx context.Context, mapping *model.TelegramUserMapping, chatID int64) (*model.ChatSession, error) {
	// Deterministic session per Telegram chat. Web and Telegram do not share
	// a transcript — that is intentional so a phone conversation cannot
	// silently overwrite a desktop one (eval M9).
	sessionID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("telegram-%d", chatID)))

	session, err := s.chatSvc.GetSession(ctx, mapping.OrgID, mapping.JobshoutUserID, sessionID)
	if err == nil {
		return session, nil
	}

	newSession := &model.ChatSession{
		ID:       sessionID,
		OrgID:    mapping.OrgID,
		UserID:   mapping.JobshoutUserID,
		Source:   model.ChatSourceTelegram,
		Metadata: map[string]any{"telegram_chat_id": chatID, "surface": "telegram"},
	}
	if err := s.chatSvc.CreateSession(ctx, newSession); err != nil {
		// Race: another update created it.
		if existing, getErr := s.chatSvc.GetSession(ctx, mapping.OrgID, mapping.JobshoutUserID, sessionID); getErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return newSession, nil
}
