package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// UserRepository handles user registration.
type UserRepository interface {
	Upsert(ctx context.Context, userID int64) error
}

type userRepo struct {
	db *sql.DB
}

// NewUserRepository constructs a UserRepository backed by PostgreSQL.
// db is the shared connection pool; it must stay open until the repository is unused.
func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepo{db: db}
}

// Upsert registers a user if they do not already exist (idempotent).
func (r *userRepo) Upsert(ctx context.Context, userID int64) error {
	const q = `INSERT INTO users (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`

	if _, err := r.db.ExecContext(ctx, q, userID); err != nil {
		return fmt.Errorf("upsert user %d: %w", userID, err)
	}

	return nil
}
