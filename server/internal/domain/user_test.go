package domain

import "testing"

func TestRoleValidation(t *testing.T) {
	tests := []struct {
		role  Role
		valid bool
	}{
		{RoleHirer, true},
		{RoleLabourer, true},
		{Role("ADMIN"), false},
		{Role(""), false},
	}

	for _, test := range tests {
		if got := test.role.Valid(); got != test.valid {
			t.Fatalf("role %q validity: expected %v, got %v", test.role, test.valid, got)
		}
	}
}
