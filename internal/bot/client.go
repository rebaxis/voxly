package bot

import (
	"fmt"
	"io"

	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

// Client wraps the Telegram bot API and provides file download functionality.
type Client struct {
	bot *telebot.Bot
	log *logger.Logger
}

// NewClient constructs a Client using the provided telebot instance.
func NewClient(bot *telebot.Bot, log *logger.Logger) *Client {
	return &Client{
		bot: bot,
		log: log.WithComponent("client"),
	}
}

// DownloadFile fetches a file from Telegram by its FileID and returns the raw bytes.
func (c *Client) DownloadFile(fileID string) ([]byte, error) {
	c.log.Info("downloading file from Telegram", zap.String("file_id", fileID))

	file, err := c.bot.FileByID(fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata for %q: %w", fileID, err)
	}

	reader, err := c.bot.File(&file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file stream for %q: %w", fileID, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data for %q: %w", fileID, err)
	}

	c.log.Info("file downloaded",
		zap.String("file_id", fileID),
		zap.Int("size_bytes", len(data)),
	)

	return data, nil
}
