package bot

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/service"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

const (
	handlerDBTimeout = 30 * time.Second

	helpCommandsList = "/list — list all saved meetings\n" +
		"/get <id> — retrieve meeting transcript\n" +
		"/find <keyword> — search meetings by keyword\n" +
		"/chat <question> — ask the AI assistant (planned)"

	helpCommandsShort = "/start — register\n" +
		"/list — list meetings\n" +
		"/get <id> — retrieve meeting\n" +
		"/find <keyword> — search meetings\n" +
		"/chat <question> — ask AI (planned)"

	usageGet  = "Usage: /get <id> — retrieve meeting"
	usageFind = "Usage: /find <keyword> — search meetings"
	usageChat = "Usage: /chat <question> — ask AI (planned)"
)

// Handler contains the Telegram message handlers used by the bot.
// It depends only on the service layer — never on repositories directly.
type Handler struct {
	queue    *Queue
	meetings service.MeetingService
	log      *logger.Logger
}

// NewHandler constructs a Handler.
func NewHandler(
	queue *Queue,
	meetings service.MeetingService,
	log *logger.Logger,
) *Handler {
	return &Handler{
		queue:    queue,
		meetings: meetings,
		log:      log.WithComponent("handler"),
	}
}

// requestCtx returns a context with a timeout for DB-backed handler work.
func (h *Handler) requestCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), handlerDBTimeout)
}

// reply sends a Telegram reply and logs failures (API/network errors are otherwise easy to miss).
func (h *Handler) reply(c telebot.Context, what interface{}, opts ...interface{}) error {
	err := c.Reply(what, opts...)
	if err != nil {
		h.log.Error("telegram reply failed", zap.Error(err))
	}
	return err
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

	switch text {
	case "/start":
		ctx, cancel := h.requestCtx()
		defer cancel()
		if err := h.meetings.Register(ctx, userID); err != nil {
			h.log.Error("failed to register user", zap.Int64("user_id", userID), zap.Error(err))
			return h.reply(c, "Sorry, registration failed. Please try again later.")
		}
		return h.reply(c, fmt.Sprintf(
			"Hello, %s! I'm Voxly — your voice meeting assistant.\n\n"+
				"Send me a voice or audio file and I'll transcribe it and store it for search.\n\n"+
				"Available commands:\n%s",
			c.Sender().FirstName,
			helpCommandsList,
		))

	case "/list":
		ctx, cancel := h.requestCtx()
		defer cancel()
		meetings, err := h.meetings.List(ctx, userID)
		if err != nil {
			h.log.Error("list meetings failed", zap.Error(err))
			return h.reply(c, "Failed to retrieve meetings. Please try again later.")
		}
		if len(meetings) == 0 {
			return h.reply(c, "No meetings yet. Send me a voice or audio file to get started.")
		}
		return h.reply(c, formatMeetingList(meetings))
	}

	if id, ok := commandRest(text, "get"); ok {
		if id == "" {
			return h.reply(c, usageGet)
		}
		ctx, cancel := h.requestCtx()
		defer cancel()
		meeting, err := h.meetings.Get(ctx, userID, id)
		if err != nil {
			h.log.Error("get meeting failed", zap.String("id", id), zap.Error(err))
			return h.reply(c, "Failed to retrieve the meeting. Please try again later.")
		}
		if meeting == nil {
			return h.reply(c, fmt.Sprintf("Meeting %q not found.", id))
		}
		return h.reply(c, formatMeetingDetail(meeting))
	}

	if keyword, ok := commandRest(text, "find"); ok {
		if keyword == "" {
			return h.reply(c, usageFind)
		}
		ctx, cancel := h.requestCtx()
		defer cancel()
		meetings, err := h.meetings.Search(ctx, userID, keyword)
		if err != nil {
			h.log.Error("search meetings failed", zap.Error(err))
			return h.reply(c, "Search failed. Please try again later.")
		}
		if len(meetings) == 0 {
			return h.reply(c, fmt.Sprintf("No meetings found containing %q.", keyword))
		}
		return h.reply(c, formatSearchResult(keyword, meetings))
	}

	if question, ok := commandRest(text, "chat"); ok {
		if question == "" {
			return h.reply(c, usageChat)
		}
		return h.reply(c, fmt.Sprintf("AI chat is not available yet.\nYour question: %s", question))
	}

	return h.reply(c, "Unknown command. Available commands:\n"+helpCommandsShort)
}

// OnVoice — round voice notes (queue → worker downloads → TranscriptionService → DB → bot sends result).
func (h *Handler) OnVoice(c telebot.Context) error {
	voice := c.Message().Voice
	if voice == nil {
		return h.reply(c, "Could not read the voice message. Try again.")
	}
	h.log.Info("voice message received",
		zap.Int64("user_id", c.Sender().ID),
		zap.Int64("chat_id", c.Chat().ID),
		zap.String("file_id", voice.FileID),
		zap.Int("duration_sec", voice.Duration),
	)
	if err := h.enqueueTranscription(c, voice.FileID, voice.MIME, "", "voice"); err != nil {
		h.log.Error("enqueue voice failed", zap.Error(err))
		return h.reply(c, "Could not start transcription. Please try again.")
	}
	return h.reply(c, "Got it. Transcribing and saving… You'll get another message when it's done.")
}

// OnAudio — Telegram audio objects and audio sent as files (documents). Same handler is wired to OnAudio and OnDocument.
func (h *Handler) OnAudio(c telebot.Context) error {
	msg := c.Message()

	if msg.Audio != nil {
		a := msg.Audio
		h.log.Info("audio message received",
			zap.Int64("user_id", c.Sender().ID),
			zap.Int64("chat_id", c.Chat().ID),
			zap.String("file_id", a.FileID),
			zap.String("file_name", a.FileName),
		)
		if err := h.enqueueTranscription(c, a.FileID, a.MIME, a.FileName, "audio"); err != nil {
			h.log.Error("enqueue audio failed", zap.Error(err))
			return h.reply(c, "Could not start transcription. Please try again.")
		}
		return h.reply(c, "Got it. Transcribing and saving… You'll get another message when it's done.")
	}

	doc := msg.Document
	if doc == nil {
		return nil
	}
	if !isAudioDocument(doc.MIME, doc.FileName) {
		h.log.Debug("ignoring document: not treated as audio",
			zap.String("mime", doc.MIME),
			zap.String("file_name", doc.FileName),
		)
		return nil
	}
	h.log.Info("audio document received",
		zap.Int64("user_id", c.Sender().ID),
		zap.String("file_id", doc.FileID),
		zap.String("file_name", doc.FileName),
		zap.String("mime", doc.MIME),
	)
	if err := h.enqueueTranscription(c, doc.FileID, doc.MIME, doc.FileName, "audio"); err != nil {
		h.log.Error("enqueue audio document failed", zap.Error(err))
		return h.reply(c, "Could not start transcription. Please try again.")
	}
	return h.reply(c, "Got it. Transcribing and saving… You'll get another message when it's done.")
}

func isAudioDocument(mime, fileName string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	if strings.HasPrefix(m, "audio/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp3", ".wav", ".ogg", ".oga", ".m4a", ".aac", ".flac", ".opus", ".webm":
		return true
	default:
		return false
	}
}

// enqueueTranscription registers the user, queues a job. Worker runs Processor → TranscriptionService (speech + DB); Bot.dispatchResults sends the outcome to the user.
func (h *Handler) enqueueTranscription(c telebot.Context, fileID, mimeType, fileName, kind string) error {
	userID := c.Sender().ID
	chatID := c.Chat().ID

	ctx, cancel := h.requestCtx()
	defer cancel()
	if err := h.meetings.Register(ctx, userID); err != nil {
		h.log.Error("register user failed", zap.Int64("user_id", userID), zap.String("kind", kind), zap.Error(err))
		return fmt.Errorf("register user: %w", err)
	}

	h.queue.Submit(Job{
		UserID:   userID,
		ChatID:   chatID,
		Type:     JobTypeTranscribe,
		FileID:   fileID,
		FileName: fileName,
		MimeType: mimeType,
	})
	return nil
}

// commandRest returns the argument after a slash command (e.g. "/get", "/get id",
// "/get@BotName id" in groups). matched is false if text is not this command.
func commandRest(text, name string) (arg string, matched bool) {
	p := "/" + name
	switch {
	case text == p:
		return "", true
	case strings.HasPrefix(text, p+" "):
		return strings.TrimSpace(text[len(p)+1:]), true
	case strings.HasPrefix(text, p+"@"):
		tail := strings.TrimPrefix(text, p+"@")
		fields := strings.Fields(tail)
		if len(fields) <= 1 {
			return "", true
		}
		return strings.Join(fields[1:], " "), true
	default:
		return "", false
	}
}

// --- formatting helpers ---

func formatMeetingList(meetings []*model.Meeting) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Your meetings (%d total):\n\n", len(meetings))
	for _, m := range meetings {
		fmt.Fprintf(&sb, "• %s — %s\n", m.ID, m.CreatedAt.Format("02 Jan 2006 15:04"))
	}
	sb.WriteString("\nUse /get <id> to view the full transcript.")
	return sb.String()
}

func formatMeetingDetail(m *model.Meeting) string {
	return fmt.Sprintf("Meeting %s (%s):\n\n%s",
		m.ID, m.CreatedAt.Format("02 Jan 2006 15:04"), m.Transcript)
}

func formatSearchResult(keyword string, meetings []*model.Meeting) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d meeting(s) containing %q:\n\n", len(meetings), keyword)
	for _, m := range meetings {
		fmt.Fprintf(&sb, "• %s — %s\n", m.ID, m.CreatedAt.Format("02 Jan 2006 15:04"))
	}
	sb.WriteString("\nUse /get <id> to view the full transcript.")
	return sb.String()
}
