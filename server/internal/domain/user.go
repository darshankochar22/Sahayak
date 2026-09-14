package domain

import "time"

type Role string

const (
	RoleHirer    Role = "HIRER"
	RoleLabourer Role = "LABOURER"
)

func (r Role) Valid() bool {
	return r == RoleHirer || r == RoleLabourer
}

type User struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phoneNumber"`
	Role        Role      `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UserCredentials struct {
	User
	PINHash        string
	FailedAttempts int
	LockedUntil    *time.Time
}
