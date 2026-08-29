package store

import (
	"context"
	"errors"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRoleConflict = errors.New("account is already registered with a different role")

type UserStore interface {
	Upsert(ctx context.Context, firebaseUID, phoneNumber string, role domain.Role) (domain.User, error)
	ByFirebaseUID(ctx context.Context, firebaseUID string) (domain.User, error)
}

type PostgresUserStore struct{ db *pgxpool.Pool }

func NewPostgresUserStore(db *pgxpool.Pool) *PostgresUserStore {
	return &PostgresUserStore{db: db}
}

func (s *PostgresUserStore) Upsert(ctx context.Context, firebaseUID, phoneNumber string, role domain.Role) (domain.User, error) {
	const query = `
		INSERT INTO users (firebase_uid, phone_number, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (firebase_uid) DO UPDATE
		SET phone_number = EXCLUDED.phone_number, updated_at = NOW()
		WHERE users.role = EXCLUDED.role
		RETURNING id, firebase_uid, phone_number, role, created_at, updated_at`
	user, err := scanUser(s.db.QueryRow(ctx, query, firebaseUID, phoneNumber, role))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrRoleConflict
	}
	return user, err
}

func (s *PostgresUserStore) ByFirebaseUID(ctx context.Context, firebaseUID string) (domain.User, error) {
	const query = `SELECT id, firebase_uid, phone_number, role, created_at, updated_at FROM users WHERE firebase_uid = $1`
	return scanUser(s.db.QueryRow(ctx, query, firebaseUID))
}

func scanUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.FirebaseUID, &user.PhoneNumber, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}
