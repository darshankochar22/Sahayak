package domain

import "time"

type PaymentStatus string

const (
	PaymentPending     PaymentStatus = "PENDING"
	PaymentPaidOffline PaymentStatus = "PAID_OFFLINE"
)

type Payment struct {
	ID          string        `json:"id"`
	JobID       string        `json:"jobId"`
	AmountPaise int64         `json:"amountPaise"`
	Currency    string        `json:"currency"`
	Status      PaymentStatus `json:"status"`
	PaidAt      *time.Time    `json:"paidAt"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}
