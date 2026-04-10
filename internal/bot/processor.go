package bot

import (
	"context"
	"fmt"

	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/service"
	"go.uber.org/zap"
)

// Processor handles the business logic for a single Job.
type Processor interface {
	Process(ctx context.Context, job Job) (string, error)
}

// FileProcessor downloads the audio file from Telegram and delegates the
// transcription and persistence pipeline to TranscriptionService.
type FileProcessor struct {
	client        *Client
	transcription service.TranscriptionService
	log           *logger.Logger
}

// NewProcessor constructs a FileProcessor and returns it as the Processor interface.
func NewProcessor(
	client *Client,
	transcription service.TranscriptionService,
	log *logger.Logger,
) Processor {
	return &FileProcessor{
		client:        client,
		transcription: transcription,
		log:           log.WithComponent("processor"),
	}
}

// Process downloads the audio file then calls TranscriptionService to transcribe and save it.
func (p *FileProcessor) Process(ctx context.Context, job Job) (string, error) {
	p.log.Info("processing job",
		zap.String("file_id", job.FileID),
		zap.Int64("user_id", job.UserID),
	)

	data, err := p.client.DownloadFile(job.FileID)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	p.log.Info("file downloaded, handing off to transcription service",
		zap.String("file_id", job.FileID),
		zap.Int("size_bytes", len(data)),
	)

	meeting, err := p.transcription.Transcribe(ctx, job.UserID, job.FileID, data, job.MimeType)
	if err != nil {
		return "", fmt.Errorf("transcription: %w", err)
	}

	msg := fmt.Sprintf("Transcription complete.\n\nMeeting ID: %s", meeting.ID)
	if meeting.Summary != "" {
		msg += "\n\nSummary:\n" + meeting.Summary
	}
	return msg, nil
}
