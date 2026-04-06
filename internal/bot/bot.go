package bot

import (
	"github.com/voxly/voxly/internal/config"
	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

// Bot orchestrates the Telegram bot lifecycle: handler registration, worker
// management, and dispatching job results back to users.
type Bot struct {
	bot     *telebot.Bot
	handler *Handler
	queue   *Queue
	cfg     *config.Config
	log     *logger.Logger
}

// New constructs a Bot and registers all Telegram handlers.
func New(tb *telebot.Bot, handler *Handler, queue *Queue, cfg *config.Config, log *logger.Logger) *Bot {
	b := &Bot{
		bot:     tb,
		handler: handler,
		queue:   queue,
		cfg:     cfg,
		log:     log.WithComponent("bot"),
	}
	b.registerHandlers()
	return b
}

// registerHandlers binds message handlers to the telebot instance.
func (b *Bot) registerHandlers() {
	b.bot.Handle(telebot.OnText, b.handler.OnText)
	b.bot.Handle(telebot.OnVoice, b.handler.OnVoice)
	b.bot.Handle(telebot.OnAudio, b.handler.OnAudio)
	b.bot.Handle(telebot.OnDocument, b.handler.OnAudio)
	b.log.Info("telegram handlers registered")
}

// Start runs workers, result delivery, and Telegram polling until Stop.
// Do not pass fx OnStart context here: fx cancels it as soon as hooks return.
func (b *Bot) Start() {
	b.log.Info("starting queue workers", zap.Int("count", b.cfg.WorkerCount))
	b.queue.StartWorkers(b.cfg.WorkerCount, nil)

	go b.dispatchResults()

	b.log.Info("starting telegram long-polling")
	b.bot.Start()
}

// Stop gracefully shuts down the Telegram poller and the work queue.
func (b *Bot) Stop() {
	b.log.Info("stopping telegram poller")
	b.bot.Stop()

	b.log.Info("stopping work queue")
	b.queue.Stop()
}

func (b *Bot) dispatchResults() {
	b.log.Info("result dispatcher started")

	for result := range b.queue.Results() {
		b.sendResult(result)
	}

	b.log.Info("result dispatcher stopped")
}

func (b *Bot) sendResult(result JobResult) {
	chat := &telebot.Chat{ID: result.ChatID}

	var text string
	if result.Err != nil {
		text = "Sorry, something went wrong while processing your file. Please try again later."
		b.log.Error("job result contains error",
			zap.Int64("chat_id", result.ChatID),
			zap.Error(result.Err),
		)
	} else {
		text = result.Text
	}

	if _, err := b.bot.Send(chat, text); err != nil {
		b.log.Error("failed to send result to user",
			zap.Int64("chat_id", result.ChatID),
			zap.Error(err),
		)
	}
}
