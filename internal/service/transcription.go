package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/voxly/voxly/internal/gigachat"
	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/repository"
	"github.com/voxly/voxly/internal/salutespeech"
	"go.uber.org/zap"
)

// TranscriptionService encapsulates the transcribe-and-persist pipeline.
// It receives raw audio bytes, sends them to SaluteSpeech, and saves the
// resulting transcript as a Meeting record in the database.
type TranscriptionService interface {
	// Transcribe sends audio to SaluteSpeech and persists the result.
	// Returns the saved Meeting with its generated ID and timestamp.
	Transcribe(ctx context.Context, userID int64, fileID string, audio []byte, mimeType string) (*model.Meeting, error)
}

type transcriptionService struct {
	ss       salutespeech.Client
	gc       gigachat.Client
	meetings repository.MeetingRepository
	log      *logger.Logger
}

// NewTranscriptionService constructs a TranscriptionService.
func NewTranscriptionService(
	ss salutespeech.Client,
	gc gigachat.Client,
	meetings repository.MeetingRepository,
	log *logger.Logger,
) TranscriptionService {
	return &transcriptionService{
		ss:       ss,
		gc:       gc,
		meetings: meetings,
		log:      log.WithComponent("transcription-service"),
	}
}

// Transcribe calls SaluteSpeech with the raw audio and saves the transcript to PostgreSQL.
func (s *transcriptionService) Transcribe(
	ctx context.Context,
	userID int64,
	fileID string,
	audio []byte,
	mimeType string,
) (*model.Meeting, error) {
	s.log.Info("starting transcription",
		zap.Int64("user_id", userID),
		zap.String("file_id", fileID),
		zap.Int("audio_bytes", len(audio)),
	)

	transcript, err := s.ss.Transcribe(ctx, audio, mimeType)
	if err != nil {
		return nil, fmt.Errorf("salutespeech: %w", err)
	}

	meeting := &model.Meeting{
		UserID:     userID,
		FileID:     fileID,
		Transcript: transcript,
	}

	const saveAttempts = 3
	var saveErr error
	for attempt := 1; attempt <= saveAttempts; attempt++ {
		saveErr = s.meetings.Save(ctx, meeting)
		if saveErr == nil {
			break
		}
		s.log.Warn("save meeting failed, retrying",
			zap.Int("attempt", attempt),
			zap.Int("max", saveAttempts),
			zap.Error(saveErr),
		)
		if attempt < saveAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("save meeting: %w", ctx.Err())
			case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
			}
		}
	}
	if saveErr != nil {
		return nil, fmt.Errorf("save meeting after %d attempts: %w", saveAttempts, saveErr)
	}

	s.log.Info("meeting saved",
		zap.String("meeting_id", meeting.ID),
		zap.Int64("user_id", userID),
		zap.Int("transcript_chars", len(transcript)),
	)

	summary, sumErr := s.gc.SummarizeTranscript(ctx, transcript)
	if sumErr != nil {
		s.log.Warn("gigachat summarize failed", zap.Error(sumErr))
	} else if strings.TrimSpace(summary) != "" {
		if err := s.meetings.UpdateSummary(ctx, userID, meeting.ID, summary); err != nil {
			s.log.Warn("update meeting summary failed", zap.Error(err))
		} else {
			meeting.Summary = summary
			s.log.Info("meeting summary saved", zap.String("meeting_id", meeting.ID))
		}
	}

	return meeting, nil
}
