package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/voxly/voxly/internal/bot"
	"github.com/voxly/voxly/internal/config"
	"github.com/voxly/voxly/internal/db"
	"github.com/voxly/voxly/internal/gigachat"
	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/repository"
	"github.com/voxly/voxly/internal/salutespeech"
	"github.com/voxly/voxly/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

func main() {
	config.RegisterFlags()
	flag.Parse()

	app := fx.New(
		fx.Provide(
			config.Load,
			newLogger,
			newTelebotBot,
			newDatabase,
			// Repositories
			fx.Annotate(repository.NewMeetingRepository, fx.As(new(repository.MeetingRepository))),
			fx.Annotate(repository.NewUserRepository, fx.As(new(repository.UserRepository))),
			// SaluteSpeech client
			newSaluteSpeechClient,
			newGigaChatClient,
			// Service layer
			fx.Annotate(service.NewMeetingService, fx.As(new(service.MeetingService))),
			fx.Annotate(service.NewTranscriptionService, fx.As(new(service.TranscriptionService))),
			// Bot internals
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

func newLogger(cfg *config.Config) (*logger.Logger, error) {
	return logger.New(logger.Config{
		Level:       cfg.LogLevel,
		Development: cfg.LogLevel == "debug",
	})
}

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

func newDatabase(cfg *config.Config, lc fx.Lifecycle, log *logger.Logger) (*sql.DB, error) {
	database, err := db.New(cfg.DatabaseDSN, db.Config{
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
		MigrationsPath:  cfg.DBMigrationsPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Info("database connected and migrations applied")

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			log.Info("closing database connection")
			if err := database.Close(); err != nil {
				return fmt.Errorf("close database: %w", err)
			}
			return nil
		},
	})

	return database, nil
}

// newSaluteSpeechClient returns the real HTTP client when an Authorization Key is set; otherwise a stub.
func newSaluteSpeechClient(cfg *config.Config, log *logger.Logger) salutespeech.Client {
	if strings.TrimSpace(cfg.SaluteSpeechAuthorizationKey) == "" {
		log.Info("SaluteSpeech credentials not configured — using stub client")
		return salutespeech.NewStub()
	}
	return salutespeech.New(salutespeech.Config{
		AuthorizationKey: cfg.SaluteSpeechAuthorizationKey,
		Scope:            cfg.SaluteSpeechScope,
	}, log)
}

func newGigaChatClient(cfg *config.Config, log *logger.Logger) gigachat.Client {
	if strings.TrimSpace(cfg.GigaChatAuthorizationKey) == "" {
		log.Info("GigaChat credentials not configured — summaries and /chat use a stub")
		return gigachat.NewStub()
	}
	return gigachat.New(gigachat.Config{
		AuthorizationKey: cfg.GigaChatAuthorizationKey,
		Scope:            cfg.GigaChatScope,
		Model:            cfg.GigaChatModel,
	}, log)
}

func registerBotLifecycle(b *bot.Bot, lc fx.Lifecycle, log *logger.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("starting voxly bot")
			go b.Start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping voxly bot")
			b.Stop()
			return nil
		},
	})
}
