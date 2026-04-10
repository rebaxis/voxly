package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/voxly/voxly/internal/gigachat"
	"github.com/voxly/voxly/internal/lib/logger"
	"github.com/voxly/voxly/internal/model"
	"github.com/voxly/voxly/internal/repository"
	"go.uber.org/zap"
)

// MeetingService is the business-logic facade for all meeting-related bot commands.
// It abstracts the underlying repositories so that handlers have no direct
// dependency on the persistence layer.
type MeetingService interface {
	// Register idempotently creates a user account.
	Register(ctx context.Context, userID int64) error

	// List returns all meetings for the user, newest first.
	List(ctx context.Context, userID int64) ([]*model.Meeting, error)

	// Get returns a single meeting by UUID for the given user, or nil if not found or not owned.
	Get(ctx context.Context, userID int64, id string) (*model.Meeting, error)

	// Search performs a full-text search on meeting transcripts.
	Search(ctx context.Context, userID int64, keyword string) ([]*model.Meeting, error)

	// Chat sends a free-form question to GigaChat (no meeting context).
	Chat(ctx context.Context, userID int64, question string) (string, error)
}

type meetingService struct {
	meetings repository.MeetingRepository
	users    repository.UserRepository
	gc       gigachat.Client
	log      *logger.Logger
}

// NewMeetingService constructs a MeetingService.
func NewMeetingService(
	meetings repository.MeetingRepository,
	users repository.UserRepository,
	gc gigachat.Client,
	log *logger.Logger,
) MeetingService {
	return &meetingService{
		meetings: meetings,
		users:    users,
		gc:       gc,
		log:      log.WithComponent("meeting-service"),
	}
}

func (s *meetingService) Register(ctx context.Context, userID int64) error {
	if err := s.users.Upsert(ctx, userID); err != nil {
		return fmt.Errorf("register user %d: %w", userID, err)
	}
	s.log.Info("user registered", zap.Int64("user_id", userID))
	return nil
}

func (s *meetingService) List(ctx context.Context, userID int64) ([]*model.Meeting, error) {
	meetings, err := s.meetings.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list meetings for user %d: %w", userID, err)
	}
	return meetings, nil
}

func (s *meetingService) Get(ctx context.Context, userID int64, id string) (*model.Meeting, error) {
	meeting, err := s.meetings.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, fmt.Errorf("get meeting %q: %w", id, err)
	}
	return meeting, nil
}

func (s *meetingService) Search(ctx context.Context, userID int64, keyword string) ([]*model.Meeting, error) {
	meetings, err := s.meetings.SearchByKeyword(ctx, userID, keyword)
	if err != nil {
		return nil, fmt.Errorf("search meetings for user %d keyword %q: %w", userID, keyword, err)
	}
	return meetings, nil
}

func (s *meetingService) Chat(ctx context.Context, userID int64, question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", fmt.Errorf("empty question")
	}
	s.log.Info("gigachat chat", zap.Int64("user_id", userID))
	ans, err := s.gc.Answer(ctx, "", question)
	if err != nil {
		return "", fmt.Errorf("gigachat: %w", err)
	}
	return ans, nil
}
