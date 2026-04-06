package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/voxly/voxly/internal/model"
)

// MeetingRepository provides CRUD and search operations for meetings.
type MeetingRepository interface {
	Save(ctx context.Context, m *model.Meeting) error
	// GetForUser returns the meeting only if it belongs to userID; nil if missing or not owned.
	GetForUser(ctx context.Context, userID int64, id string) (*model.Meeting, error)
	ListByUser(ctx context.Context, userID int64) ([]*model.Meeting, error)
	SearchByKeyword(ctx context.Context, userID int64, keyword string) ([]*model.Meeting, error)
}

type meetingRepo struct {
	db *sql.DB
}

// NewMeetingRepository constructs a MeetingRepository backed by PostgreSQL.
// db is the shared connection pool; it must stay open until the repository is unused.
func NewMeetingRepository(db *sql.DB) MeetingRepository {
	return &meetingRepo{db: db}
}

// Save inserts a new meeting and fills in the generated ID and CreatedAt fields.
func (r *meetingRepo) Save(ctx context.Context, m *model.Meeting) error {
	const q = `
		INSERT INTO meetings (user_id, file_id, transcript)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, q, m.UserID, m.FileID, m.Transcript).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return fmt.Errorf("save meeting user_id=%d file_id=%q: %w", m.UserID, m.FileID, err)
	}
	return nil
}

// GetForUser returns the meeting with the given UUID if it belongs to userID; nil if not found or wrong owner.
func (r *meetingRepo) GetForUser(ctx context.Context, userID int64, id string) (*model.Meeting, error) {
	const q = `
		SELECT id, user_id, file_id, transcript, created_at
		FROM meetings WHERE id = $1 AND user_id = $2`

	m := &model.Meeting{}
	err := r.db.QueryRowContext(ctx, q, id, userID).
		Scan(&m.ID, &m.UserID, &m.FileID, &m.Transcript, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meeting %q for user %d: %w", id, userID, err)
	}

	return m, nil
}

// ListByUser returns all meetings for the given user, newest first.
func (r *meetingRepo) ListByUser(ctx context.Context, userID int64) ([]*model.Meeting, error) {
	const q = `
		SELECT id, user_id, file_id, transcript, created_at
		FROM meetings
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list meetings for user %d: %w", userID, err)
	}
	defer rows.Close()

	return scanMeetings(rows)
}

// SearchByKeyword performs a full-text search on the Russian-language transcript.
func (r *meetingRepo) SearchByKeyword(ctx context.Context, userID int64, keyword string) ([]*model.Meeting, error) {
	const q = `
		SELECT id, user_id, file_id, transcript, created_at
		FROM meetings
		WHERE user_id = $1
		  AND to_tsvector('russian', transcript) @@ plainto_tsquery('russian', $2)
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q, userID, keyword)
	if err != nil {
		return nil, fmt.Errorf("search meetings for user %d keyword %q: %w", userID, keyword, err)
	}
	defer rows.Close()

	return scanMeetings(rows)
}

func scanMeetings(rows *sql.Rows) ([]*model.Meeting, error) {
	var meetings []*model.Meeting
	for rows.Next() {
		m := &model.Meeting{}
		if err := rows.Scan(&m.ID, &m.UserID, &m.FileID, &m.Transcript, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan meeting row: %w", err)
		}
		meetings = append(meetings, m)
	}
	if err := rows.Err(); err != nil {
		return meetings, fmt.Errorf("iterate meetings: %w", err)
	}
	return meetings, nil
}
