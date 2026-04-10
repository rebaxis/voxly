package service

import (
	"context"
	"testing"

	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/repository"
	"github.com/voxly/voxly/internal/salutespeech"
	"go.uber.org/zap"
)

type transcribeMockRepo struct {
	lastSummary string
	lastID      string
	lastUser    int64
}

func (m *transcribeMockRepo) Save(_ context.Context, mtg *model.Meeting) error {
	mtg.ID = "meeting-uuid-1"
	mtg.CreatedAt = mtg.CreatedAt // unchanged
	return nil
}
func (m *transcribeMockRepo) GetForUser(context.Context, int64, string) (*model.Meeting, error) {
	return nil, nil
}
func (m *transcribeMockRepo) ListByUser(context.Context, int64) ([]*model.Meeting, error) {
	return nil, nil
}
func (m *transcribeMockRepo) SearchByKeyword(context.Context, int64, string) ([]*model.Meeting, error) {
	return nil, nil
}
func (m *transcribeMockRepo) UpdateSummary(_ context.Context, userID int64, meetingID, summary string) error {
	m.lastUser = userID
	m.lastID = meetingID
	m.lastSummary = summary
	return nil
}

var _ repository.MeetingRepository = (*transcribeMockRepo)(nil)

type summarizeMockGC struct {
	summary string
	err     error
}

func (s *summarizeMockGC) SummarizeTranscript(context.Context, string) (string, error) {
	return s.summary, s.err
}
func (s *summarizeMockGC) Answer(context.Context, string, string) (string, error) {
	return "", nil
}

type stubSpeech struct{}

func (stubSpeech) Transcribe(context.Context, []byte, string) (string, error) {
	return "транскрипт теста", nil
}

var _ salutespeech.Client = stubSpeech{}

func TestTranscribe_PersistsSummaryFromGigaChat(t *testing.T) {
	log := &logger.Logger{Logger: zap.NewNop()}
	repo := &transcribeMockRepo{}
	gc := &summarizeMockGC{summary: "Краткое резюме."}
	svc := NewTranscriptionService(stubSpeech{}, gc, repo, log)

	mtg, err := svc.Transcribe(context.Background(), 99, "file-1", []byte{1, 2}, "audio/ogg")
	if err != nil {
		t.Fatal(err)
	}
	if mtg.Summary != "Краткое резюме." {
		t.Errorf("meeting.Summary: want %q, got %q", "Краткое резюме.", mtg.Summary)
	}
	if repo.lastSummary != "Краткое резюме." || repo.lastID != mtg.ID || repo.lastUser != 99 {
		t.Errorf("UpdateSummary: summary=%q id=%q user=%d", repo.lastSummary, repo.lastID, repo.lastUser)
	}
}

func TestTranscribe_SummarizeErrorStillReturnsMeeting(t *testing.T) {
	log := &logger.Logger{Logger: zap.NewNop()}
	repo := &transcribeMockRepo{}
	gc := &summarizeMockGC{err: context.Canceled}
	svc := NewTranscriptionService(stubSpeech{}, gc, repo, log)

	mtg, err := svc.Transcribe(context.Background(), 1, "f", []byte{1}, "audio/ogg")
	if err != nil {
		t.Fatal(err)
	}
	if mtg.Transcript != "транскрипт теста" {
		t.Fatalf("unexpected transcript %q", mtg.Transcript)
	}
	if repo.lastSummary != "" {
		t.Error("UpdateSummary should not run on summarize error")
	}
}
