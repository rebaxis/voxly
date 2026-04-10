package bot

import (
	"context"

	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/service"
	"go.uber.org/zap"
)

// newTestLogger returns a no-op logger suitable for tests.
func newTestLogger() *logger.Logger {
	return &logger.Logger{Logger: zap.NewNop()}
}

// mockProcessor is a Processor that returns a fixed result without doing any I/O.
type mockProcessor struct {
	result string
	err    error
}

func (m *mockProcessor) Process(_ context.Context, _ Job) (string, error) {
	return m.result, m.err
}

// newTestQueue creates a Queue with a buffered channel of the given size and a
// mockProcessor. No workers are started — callers control that explicitly.
func newTestQueue(size int, proc Processor) *Queue {
	return &Queue{
		jobs:      make(chan Job, size),
		results:   make(chan JobResult, size),
		processor: proc,
		log:       newTestLogger().WithComponent("queue"),
	}
}

// --- mock services ---

// mockMeetingService implements service.MeetingService in memory.
type mockMeetingService struct {
	meetings  []*model.Meeting
	chatReply string
	chatErr   error
}

var _ service.MeetingService = (*mockMeetingService)(nil)

func (m *mockMeetingService) Register(_ context.Context, _ int64) error { return nil }

func (m *mockMeetingService) List(_ context.Context, _ int64) ([]*model.Meeting, error) {
	return m.meetings, nil
}

func (m *mockMeetingService) Get(_ context.Context, userID int64, id string) (*model.Meeting, error) {
	for _, mtg := range m.meetings {
		if mtg.ID == id && mtg.UserID == userID {
			return mtg, nil
		}
	}
	return nil, nil
}

func (m *mockMeetingService) Search(_ context.Context, _ int64, _ string) ([]*model.Meeting, error) {
	return m.meetings, nil
}

func (m *mockMeetingService) Chat(_ context.Context, _ int64, _ string) (string, error) {
	if m.chatErr != nil {
		return "", m.chatErr
	}
	return m.chatReply, nil
}

// newTestHandler creates a Handler wired with a mock MeetingService.
func newTestHandler(q *Queue) *Handler {
	return NewHandler(q, &mockMeetingService{}, newTestLogger())
}
