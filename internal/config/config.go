package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/go-playground/validator/v10"
)

// Config holds all application settings.
type Config struct {
	TelegramToken string `json:"telegram_token" validate:"required"`
	LogLevel      string `json:"log_level"      validate:"required,oneof=debug info warn error"`
	WorkerCount   int    `json:"worker_count"   validate:"min=1"`
	QueueSize     int    `json:"queue_size"     validate:"min=1"`
}

// Load reads configuration applying sources in ascending priority order:
// defaults → config file → flags → environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		LogLevel:    "info",
		WorkerCount: 5,
		QueueSize:   100,
	}

	// Define flags (zero/empty values mean "not provided").
	token := flag.String("token", "", "Telegram bot token")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	workerCount := flag.Int("workers", 0, "Number of queue worker goroutines")
	queueSize := flag.Int("queue-size", 0, "Job queue buffer size")
	configFile := flag.String("config", "", "Path to JSON config file")
	flag.Parse()

	if err := loadFile(cfg, *configFile); err != nil {
		return nil, err
	}

	loadFlags(cfg, *token, *logLevel, *workerCount, *queueSize)
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

// loadFlags overrides config fields with non-zero flag values.
func loadFlags(cfg *Config, token, logLevel string, workerCount, queueSize int) {
	if token != "" {
		cfg.TelegramToken = token
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if workerCount != 0 {
		cfg.WorkerCount = workerCount
	}
	if queueSize != 0 {
		cfg.QueueSize = queueSize
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
}
