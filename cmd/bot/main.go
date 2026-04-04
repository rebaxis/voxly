package main

import (
	"context"
	"fmt"
	"time"

	"github.com/voxly/voxly/internal/bot"
	"github.com/voxly/voxly/internal/config"
	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

func main() {
	app := fx.New(
		fx.Provide(
			config.Load,
			newLogger,
			newTelebotBot,
			bot.NewClient,
			bot.NewProcessor,
			bot.NewQueue,
			bot.NewHandler,
			bot.New,
		),
		fx.Invoke(registerBotLifecycle),
	)

	app.Run()
}

// newLogger constructs the application logger from configuration.
func newLogger(cfg *config.Config) (*logger.Logger, error) {
	return logger.New(logger.Config{
		Level:       cfg.LogLevel,
		Development: cfg.LogLevel == "debug",
	})
}

// newTelebotBot initialises the Telegram bot client.
func newTelebotBot(cfg *config.Config, log *logger.Logger) (*telebot.Bot, error) {
	tb, err := telebot.NewBot(telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	log.Info("telegram bot initialised", zap.String("username", tb.Me.Username))

	return tb, nil
}

// registerBotLifecycle attaches the bot start/stop hooks to the fx lifecycle.
func registerBotLifecycle(b *bot.Bot, lc fx.Lifecycle, log *logger.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting voxly bot")
			go b.Start(ctx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping voxly bot")
			b.Stop()
			return nil
		},
	})
}
