package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSessionInvalid = errors.New("session invalid")

type SessionStore interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) (string, error)
	Revoke(ctx context.Context, tokenHash string) error
}

type PostgresSessionStore struct{ db *pgxpool.Pool }

func NewPostgresSessionStore(db *pgxpool.Pool) *PostgresSessionStore {
	return &PostgresSessionStore{db: db}
}

func (s *PostgresSessionStore) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO refresh_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
	return err
}

func (s *PostgresSessionStore) Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(ctx, `UPDATE refresh_sessions SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW() RETURNING user_id`, oldHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSessionInvalid
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO refresh_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, newHash, expiresAt); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *PostgresSessionStore) Revoke(ctx context.Context, tokenHash string) error {
	tag, err := s.db.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionInvalid
	}
	return nil
}
