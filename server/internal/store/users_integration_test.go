package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEST_DATABASE_URL must point to a disposable database with all migrations applied.
func TestPostgresLockoutExpiryRestoresFiveAttempts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := NewPostgresUserStore(db)
	user, err := users.Create(ctx, "integration-"+uuid.NewString(), "unused-test-hash", domain.RoleHirer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID); err != nil {
			t.Error(err)
		}
	}()
	for cycle := 0; cycle < 2; cycle++ {
		for attempt := 1; attempt <= 5; attempt++ {
			until, err := users.RecordFailedLogin(ctx, user.ID, 5, 15*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if (until != nil) != (attempt == 5) {
				t.Fatalf("cycle %d attempt %d: unexpected lock %v", cycle, attempt, until)
			}
		}
		before, err := users.ByPhone(ctx, user.PhoneNumber)
		if err != nil {
			t.Fatal(err)
		}
		until, err := users.RecordFailedLogin(ctx, user.ID, 5, 15*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if until == nil || !until.Equal(*before.LockedUntil) {
			t.Fatal("active lock must not be extended")
		}
		if _, err := db.Exec(ctx, "UPDATE users SET locked_until = NOW() - INTERVAL '1 second' WHERE id = $1", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := users.ResetLoginFailures(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	credentials, err := users.ByPhone(ctx, user.PhoneNumber)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.FailedAttempts != 0 || credentials.LockedUntil != nil {
		t.Fatal("successful login must clear lockout state")
	}
}
