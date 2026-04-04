package bot

import (
	"fmt"
	"strings"

	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

// Handler contains the Telegram message handlers used by the bot.
type Handler struct {
	queue *Queue
	log   *logger.Logger
}

// NewHandler constructs a Handler.
func NewHandler(queue *Queue, log *logger.Logger) *Handler {
	return &Handler{
		queue: queue,
		log:   log.WithComponent("handler"),
	}
}

// OnText handles all incoming text messages and dispatches bot commands.
// Supported commands: /start, /list, /get <id>, /find <keyword>, /chat <question>.
func (h *Handler) OnText(c telebot.Context) error {
	text := strings.TrimSpace(c.Text())
	userID := c.Sender().ID

	h.log.Info("text message received",
		zap.Int64("user_id", userID),
		zap.String("text", text),
	)

	switch {
	case text == "/start":
		return c.Reply(fmt.Sprintf(
			"Hello, %s! I'm Voxly — your voice meeting assistant.\n\n"+
				"Send me a voice or audio file and I'll transcribe it and save the summary.\n\n"+
				"Available commands:\n"+
				"/list — list all saved meetings\n"+
				"/get <id> — retrieve meeting transcript\n"+
				"/find <keyword> — search meetings by keyword\n"+
				"/chat <question> — ask the AI assistant",
			c.Sender().FirstName,
		))

	case text == "/list":
		// TODO: query PostgreSQL for meetings belonging to this user
		return c.Reply("No meetings found yet. Send me a voice or audio file to get started.")

	case strings.HasPrefix(text, "/get "):
		id := strings.TrimPrefix(text, "/get ")
		if id == "" {
			return c.Reply("Usage: /get <id>")
		}
		// TODO: retrieve meeting transcript from PostgreSQL
		return c.Reply(fmt.Sprintf("Meeting with ID %q not found.", id))

	case strings.HasPrefix(text, "/find "):
		keyword := strings.TrimPrefix(text, "/find ")
		if keyword == "" {
			return c.Reply("Usage: /find <keyword>")
		}
		// TODO: full-text search in PostgreSQL
		return c.Reply(fmt.Sprintf("No meetings found containing %q.", keyword))

	case strings.HasPrefix(text, "/chat "):
		question := strings.TrimPrefix(text, "/chat ")
		if question == "" {
			return c.Reply("Usage: /chat <question>")
		}
		// TODO: forward question to GigaChat API
		return c.Reply(fmt.Sprintf("GigaChat integration is not yet available.\nYour question: %s", question))

	default:
		return c.Reply(
			"Unknown command. Available commands:\n" +
				"/start — register\n" +
				"/list — list meetings\n" +
				"/get <id> — retrieve meeting\n" +
				"/find <keyword> — search meetings\n" +
				"/chat <question> — ask AI",
		)
	}
}

// OnVoice handles voice messages sent by the user.
// The voice file is queued for transcription and the user receives an acknowledgement.
func (h *Handler) OnVoice(c telebot.Context) error {
	voice := c.Message().Voice
	userID := c.Sender().ID
	chatID := c.Chat().ID

	h.log.Info("voice message received",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("file_id", voice.FileID),
		zap.Int("duration_sec", voice.Duration),
	)

	h.queue.Submit(Job{
		UserID:   userID,
		ChatID:   chatID,
		Type:     JobTypeTranscribe,
		FileID:   voice.FileID,
		MimeType: voice.MIME,
	})

	return c.Reply("Voice message received! I'm processing your recording…")
}

// OnAudio handles audio files uploaded by the user.
// The audio file is queued for transcription and the user receives an acknowledgement.
func (h *Handler) OnAudio(c telebot.Context) error {
	audio := c.Message().Audio
	userID := c.Sender().ID
	chatID := c.Chat().ID

	h.log.Info("audio file received",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("file_id", audio.FileID),
		zap.String("file_name", audio.FileName),
	)

	h.queue.Submit(Job{
		UserID:   userID,
		ChatID:   chatID,
		Type:     JobTypeTranscribe,
		FileID:   audio.FileID,
		FileName: audio.FileName,
		MimeType: audio.MIME,
	})

	return c.Reply("Audio file received! I'm processing your recording…")
}
