package store

import (
	"context"
	"errors"
	"time"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPhoneExists = errors.New("phone number already registered")

type UserStore interface {
	Create(ctx context.Context, phoneNumber, pinHash string, role domain.Role) (domain.User, error)
	ByPhone(ctx context.Context, phoneNumber string) (domain.UserCredentials, error)
	ByID(ctx context.Context, userID string) (domain.User, error)
	RecordFailedLogin(ctx context.Context, userID string, lockAfter int, lockDuration time.Duration) (*time.Time, error)
	ResetLoginFailures(ctx context.Context, userID string) error
}

type PostgresUserStore struct{ db *pgxpool.Pool }

func NewPostgresUserStore(db *pgxpool.Pool) *PostgresUserStore { return &PostgresUserStore{db: db} }

func (s *PostgresUserStore) Create(ctx context.Context, phoneNumber, pinHash string, role domain.Role) (domain.User, error) {
	const query = `INSERT INTO users (phone_number, pin_hash, role) VALUES ($1, $2, $3)
		RETURNING id, phone_number, role, created_at, updated_at`
	user, err := scanUser(s.db.QueryRow(ctx, query, phoneNumber, pinHash, role))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.User{}, ErrPhoneExists
	}
	return user, err
}

func (s *PostgresUserStore) ByPhone(ctx context.Context, phoneNumber string) (domain.UserCredentials, error) {
	const query = `SELECT id, phone_number, role, created_at, updated_at, pin_hash, failed_pin_attempts, locked_until
		FROM users WHERE phone_number = $1`
	var credentials domain.UserCredentials
	err := s.db.QueryRow(ctx, query, phoneNumber).Scan(&credentials.ID, &credentials.PhoneNumber, &credentials.Role,
		&credentials.CreatedAt, &credentials.UpdatedAt, &credentials.PINHash, &credentials.FailedAttempts, &credentials.LockedUntil)
	return credentials, err
}

func (s *PostgresUserStore) ByID(ctx context.Context, userID string) (domain.User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT id, phone_number, role, created_at, updated_at FROM users WHERE id = $1`, userID))
}

func (s *PostgresUserStore) RecordFailedLogin(ctx context.Context, userID string, lockAfter int, lockDuration time.Duration) (*time.Time, error) {
	const query = `UPDATE users SET failed_pin_attempts = CASE
			WHEN locked_until > NOW() THEN failed_pin_attempts
			WHEN locked_until IS NOT NULL THEN 1
			ELSE failed_pin_attempts + 1 END,
		locked_until = CASE
			WHEN locked_until > NOW() THEN locked_until
			WHEN locked_until IS NOT NULL THEN NULL
			WHEN failed_pin_attempts + 1 >= $2 THEN NOW() + $3::interval
			ELSE NULL END,
		updated_at = NOW() WHERE id = $1 RETURNING locked_until`
	var lockedUntil *time.Time
	err := s.db.QueryRow(ctx, query, userID, lockAfter, lockDuration.String()).Scan(&lockedUntil)
	return lockedUntil, err
}

func (s *PostgresUserStore) ResetLoginFailures(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET failed_pin_attempts = 0, locked_until = NULL, updated_at = NOW() WHERE id = $1`, userID)
	return err
}

func scanUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.PhoneNumber, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}
