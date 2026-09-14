package store

import (
	"context"
	"errors"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPaymentNotFound = errors.New("payment not found")

type PaymentStore interface {
	ByJob(ctx context.Context, userID, jobID string) (domain.Payment, error)
	MarkPaidOffline(ctx context.Context, userID, jobID string) (domain.Payment, error)
}

type PostgresPaymentStore struct{ db *pgxpool.Pool }

func NewPostgresPaymentStore(db *pgxpool.Pool) *PostgresPaymentStore {
	return &PostgresPaymentStore{db: db}
}

func (s *PostgresPaymentStore) ByJob(ctx context.Context, userID, jobID string) (domain.Payment, error) {
	const query = `
		SELECT p.id, p.job_id, p.amount_paise, p.currency, p.status, p.paid_at, p.created_at, p.updated_at
		FROM job_payments p
		JOIN jobs j ON j.id = p.job_id
		WHERE p.job_id = $1 AND j.hirer_id = $2`
	payment, err := scanPayment(s.db.QueryRow(ctx, query, jobID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrPaymentNotFound
	}
	return payment, err
}

func (s *PostgresPaymentStore) MarkPaidOffline(ctx context.Context, userID, jobID string) (domain.Payment, error) {
	const query = `
		UPDATE job_payments p
		SET status = 'PAID_OFFLINE',
			paid_at = COALESCE(paid_at, NOW()),
			updated_at = NOW()
		FROM jobs j
		JOIN users hirer ON hirer.id = j.hirer_id
		WHERE p.job_id = $1
			AND p.job_id = j.id
			AND hirer.id = $2
			AND hirer.role = 'HIRER'
		RETURNING p.id, p.job_id, p.amount_paise, p.currency, p.status, p.paid_at, p.created_at, p.updated_at`
	payment, err := scanPayment(s.db.QueryRow(ctx, query, jobID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrPaymentNotFound
	}
	return payment, err
}

func scanPayment(row pgx.Row) (domain.Payment, error) {
	var payment domain.Payment
	err := row.Scan(
		&payment.ID,
		&payment.JobID,
		&payment.AmountPaise,
		&payment.Currency,
		&payment.Status,
		&payment.PaidAt,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	return payment, err
}
