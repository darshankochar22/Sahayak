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
	FirebaseUID string    `json:"-"`
	PhoneNumber string    `json:"phoneNumber"`
	Role        Role      `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
