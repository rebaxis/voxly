package service

import (
	"context"
	"errors"
	"testing"

	"github.com/voxly/voxly/internal/gigachat"
	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/repository"
	"go.uber.org/zap"
)

type noopMeetRepo struct{}

func (noopMeetRepo) Save(context.Context, *model.Meeting) error { return nil }
func (noopMeetRepo) GetForUser(context.Context, int64, string) (*model.Meeting, error) {
	return nil, nil
}
func (noopMeetRepo) ListByUser(context.Context, int64) ([]*model.Meeting, error) { return nil, nil }
func (noopMeetRepo) SearchByKeyword(context.Context, int64, string) ([]*model.Meeting, error) {
	return nil, nil
}
func (noopMeetRepo) UpdateSummary(context.Context, int64, string, string) error { return nil }

var _ repository.MeetingRepository = noopMeetRepo{}

type noopUserRepo struct{}

func (noopUserRepo) Upsert(context.Context, int64) error { return nil }

var _ repository.UserRepository = noopUserRepo{}

func TestChat_ErrNotConfigured(t *testing.T) {
	log := &logger.Logger{Logger: zap.NewNop()}
	svc := NewMeetingService(noopMeetRepo{}, noopUserRepo{}, gigachat.NewStub(), log)

	_, err := svc.Chat(context.Background(), 1, "hello")
	if !errors.Is(err, gigachat.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}
