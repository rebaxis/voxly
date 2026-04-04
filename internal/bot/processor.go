package bot

import (
	"context"
	"fmt"

	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
)

// Processor handles the business logic for a single Job.
type Processor interface {
	Process(ctx context.Context, job Job) (string, error)
}

// FileProcessor downloads the file from Telegram and stubs the transcription pipeline.
// In future iterations it will call SaluteSpeech and store results in PostgreSQL.
type FileProcessor struct {
	client *Client
	log    *logger.Logger
}

// NewProcessor constructs a FileProcessor and returns it as the Processor interface.
func NewProcessor(client *Client, log *logger.Logger) Processor {
	return &FileProcessor{
		client: client,
		log:    log.WithComponent("processor"),
	}
}

// Process downloads the audio/voice file and returns a placeholder response.
func (p *FileProcessor) Process(ctx context.Context, job Job) (string, error) {
	p.log.Info("processing file",
		zap.String("file_id", job.FileID),
		zap.Int64("user_id", job.UserID),
	)

	data, err := p.client.DownloadFile(job.FileID)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	p.log.Info("file ready for transcription",
		zap.String("file_id", job.FileID),
		zap.Int("size_bytes", len(data)),
	)

	// TODO: send to SaluteSpeech API for transcription
	// TODO: generate summary with GigaChat API
	// TODO: store transcript and summary in PostgreSQL
	return fmt.Sprintf("File received (%d bytes). Transcription is not yet available.", len(data)), nil
}
