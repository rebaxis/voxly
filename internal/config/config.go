package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
)

// cli holds pointers registered by [RegisterFlags] on [flag.CommandLine].
// [Load] reads them after the process has called [flag.Parse].
var cli struct {
	configFile        *string
	token             *string
	logLevel          *string
	workerCount       *int
	queueSize         *int
	dbDSN             *string
	dbMigrationsPath  *string
	dbMaxOpenConns    *int
	dbMaxIdleConns    *int
	dbConnMaxLifetime *time.Duration
	ssAuthKey         *string
	ssScope           *string
}

// RegisterFlags defines all CLI flags on the default [flag.CommandLine].
// Call [flag.Parse] (e.g. from main) before [Load].
func RegisterFlags() {
	if cli.configFile != nil {
		return
	}
	cli.configFile = flag.String("config", "", "Path to JSON config file")
	cli.token = flag.String("token", "", "Telegram bot token")
	cli.logLevel = flag.String("log-level", "", "Log level: debug, info, warn, error")
	cli.workerCount = flag.Int("workers", 0, "Number of queue worker goroutines")
	cli.queueSize = flag.Int("queue-size", 0, "Job queue buffer size")
	cli.dbDSN = flag.String("db-dsn", "", "PostgreSQL DSN")
	cli.dbMigrationsPath = flag.String("db-migrations", "", "Path to migration files (e.g. file://migrations)")
	cli.dbMaxOpenConns = flag.Int("db-max-open-conns", 0, "Max open DB connections")
	cli.dbMaxIdleConns = flag.Int("db-max-idle-conns", 0, "Max idle DB connections")
	cli.dbConnMaxLifetime = flag.Duration("db-conn-max-lifetime", 0, "Max DB connection lifetime (e.g. 5m)")
	cli.ssAuthKey = flag.String("ss-auth-key", "", "SaluteSpeech Authorization Key from Studio (Basic credential)")
	cli.ssScope = flag.String("ss-scope", "", "SaluteSpeech OAuth scope (e.g. SALUTE_SPEECH_CORP, SALUTE_SPEECH_PERS)")
}

// Config holds all application settings.
type Config struct {
	// Telegram
	TelegramToken string `json:"telegram_token" validate:"required"`

	// Logging
	LogLevel string `json:"log_level" validate:"required,oneof=debug info warn error"`

	// Worker queue
	WorkerCount int `json:"worker_count" validate:"min=1"`
	QueueSize   int `json:"queue_size"   validate:"min=1"`

	// PostgreSQL
	DatabaseDSN       string        `json:"database_dsn"      validate:"required"`
	DBMigrationsPath  string        `json:"db_migrations_path"`
	DBMaxOpenConns    int           `json:"db_max_open_conns" validate:"min=1"`
	DBMaxIdleConns    int           `json:"db_max_idle_conns" validate:"min=1"`
	DBConnMaxLifetime time.Duration `json:"db_conn_max_lifetime"`

	// SaluteSpeech — use authorization_key and scope
	SaluteSpeechAuthorizationKey string `json:"salutespeech_authorization_key"`
	SaluteSpeechScope            string `json:"salutespeech_scope"`
}

// Load reads configuration applying sources in ascending priority order:
// defaults → config file → flags → environment variables.
// Requires [RegisterFlags] then [flag.Parse] to have run before this (see cmd/bot).
func Load() (*Config, error) {
	if cli.configFile == nil {
		return nil, fmt.Errorf("config: RegisterFlags must be called before Load")
	}

	cfg := &Config{
		LogLevel:          "info",
		WorkerCount:       5,
		QueueSize:         100,
		DBMigrationsPath:  "file://migrations",
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 5 * time.Minute,
		SaluteSpeechScope: "SALUTE_SPEECH_PERS",
	}

	if err := loadFile(cfg, *cli.configFile); err != nil {
		return nil, err
	}

	loadFlags(cfg, flagValues{
		token:             *cli.token,
		logLevel:          *cli.logLevel,
		workerCount:       *cli.workerCount,
		queueSize:         *cli.queueSize,
		dbDSN:             *cli.dbDSN,
		dbMigrationsPath:  *cli.dbMigrationsPath,
		dbMaxOpenConns:    *cli.dbMaxOpenConns,
		dbMaxIdleConns:    *cli.dbMaxIdleConns,
		dbConnMaxLifetime: *cli.dbConnMaxLifetime,
		ssAuthKey:         *cli.ssAuthKey,
		ssScope:           *cli.ssScope,
	})
	loadEnv(cfg)

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// loadFile reads a JSON config file if it exists.
// The path is resolved in order: flagPath → CONFIG_FILE env → "config.json".
func loadFile(cfg *Config, flagPath string) error {
	path := flagPath
	if path == "" {
		path = os.Getenv("CONFIG_FILE")
	}
	if path == "" {
		path = "config.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return nil
}

// flagValues groups all parsed flag values passed to loadFlags.
type flagValues struct {
	token             string
	logLevel          string
	workerCount       int
	queueSize         int
	dbDSN             string
	dbMigrationsPath  string
	dbMaxOpenConns    int
	dbMaxIdleConns    int
	dbConnMaxLifetime time.Duration
	ssAuthKey         string
	ssScope           string
}

// loadFlags overrides config fields with non-zero flag values.
func loadFlags(cfg *Config, f flagValues) {
	if f.token != "" {
		cfg.TelegramToken = f.token
	}
	if f.logLevel != "" {
		cfg.LogLevel = f.logLevel
	}
	if f.workerCount != 0 {
		cfg.WorkerCount = f.workerCount
	}
	if f.queueSize != 0 {
		cfg.QueueSize = f.queueSize
	}
	if f.dbDSN != "" {
		cfg.DatabaseDSN = f.dbDSN
	}
	if f.dbMigrationsPath != "" {
		cfg.DBMigrationsPath = f.dbMigrationsPath
	}
	if f.dbMaxOpenConns != 0 {
		cfg.DBMaxOpenConns = f.dbMaxOpenConns
	}
	if f.dbMaxIdleConns != 0 {
		cfg.DBMaxIdleConns = f.dbMaxIdleConns
	}
	if f.dbConnMaxLifetime != 0 {
		cfg.DBConnMaxLifetime = f.dbConnMaxLifetime
	}
	if f.ssAuthKey != "" {
		cfg.SaluteSpeechAuthorizationKey = f.ssAuthKey
	}
	if f.ssScope != "" {
		cfg.SaluteSpeechScope = f.ssScope
	}
}

// loadEnv overrides config fields with environment variable values (highest priority).
func loadEnv(cfg *Config) {
	if v := os.Getenv("TELEGRAM_TOKEN"); v != "" {
		cfg.TelegramToken = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("WORKER_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WorkerCount = n
		}
	}
	if v := os.Getenv("QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.QueueSize = n
		}
	}
	if v := os.Getenv("DATABASE_DSN"); v != "" {
		cfg.DatabaseDSN = v
	}
	if v := os.Getenv("DB_MIGRATIONS_PATH"); v != "" {
		cfg.DBMigrationsPath = v
	}
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DBMaxOpenConns = n
		}
	}
	if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DBMaxIdleConns = n
		}
	}
	if v := os.Getenv("DB_CONN_MAX_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.DBConnMaxLifetime = d
		}
	}
	if v := os.Getenv("SALUTESPEECH_AUTHORIZATION_KEY"); v != "" {
		cfg.SaluteSpeechAuthorizationKey = v
	}
	if v := os.Getenv("SALUTESPEECH_SCOPE"); v != "" {
		cfg.SaluteSpeechScope = v
	}
}
