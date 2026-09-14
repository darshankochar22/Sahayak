package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

type memoryUsers struct {
	credentials domain.UserCredentials
	attempts    int
	lockedUntil *time.Time
}

func (m *memoryUsers) Create(_ context.Context, phone, hash string, role domain.Role) (domain.User, error) {
	m.credentials = domain.UserCredentials{User: domain.User{ID: "11111111-1111-4111-8111-111111111111", PhoneNumber: phone, Role: role}, PINHash: hash}
	return m.credentials.User, nil
}
func (m *memoryUsers) ByPhone(context.Context, string) (domain.UserCredentials, error) {
	m.credentials.FailedAttempts = m.attempts
	m.credentials.LockedUntil = m.lockedUntil
	return m.credentials, nil
}
func (m *memoryUsers) ByID(context.Context, string) (domain.User, error) {
	return m.credentials.User, nil
}
func (m *memoryUsers) RecordFailedLogin(_ context.Context, _ string, after int, duration time.Duration) (*time.Time, error) {
	m.attempts++
	if m.attempts >= after {
		value := time.Now().Add(duration)
		m.lockedUntil = &value
	}
	return m.lockedUntil, nil
}
func (m *memoryUsers) ResetLoginFailures(context.Context, string) error {
	m.attempts = 0
	m.lockedUntil = nil
	return nil
}

type memorySessions struct{ active map[string]string }

func (m *memorySessions) Create(_ context.Context, userID, hash string, _ time.Time) error {
	if m.active == nil {
		m.active = map[string]string{}
	}
	m.active[hash] = userID
	return nil
}
func (m *memorySessions) Rotate(_ context.Context, oldHash, newHash string, _ time.Time) (string, error) {
	userID, ok := m.active[oldHash]
	if !ok {
		return "", store.ErrSessionInvalid
	}
	delete(m.active, oldHash)
	m.active[newHash] = userID
	return userID, nil
}
func (m *memorySessions) Revoke(_ context.Context, hash string) error {
	if _, ok := m.active[hash]; !ok {
		return store.ErrSessionInvalid
	}
	delete(m.active, hash)
	return nil
}

func newTestService() (*Service, *memoryUsers, *memorySessions) {
	users := &memoryUsers{}
	sessions := &memorySessions{}
	return NewService(users, sessions, "pepper-that-is-at-least-thirty-two-bytes", "jwt-secret-that-is-at-least-thirty-two-bytes"), users, sessions
}

func TestNormalizeIndianPhone(t *testing.T) {
	for _, input := range []string{"9876543210", "919876543210", "+91 98765-43210"} {
		got, err := NormalizeIndianPhone(input)
		if err != nil || got != "+919876543210" {
			t.Fatalf("input=%q got=%q err=%v", input, got, err)
		}
	}
	for _, input := range []string{"1234567890", "+14155552671", "98765"} {
		if _, err := NormalizeIndianPhone(input); err == nil {
			t.Fatalf("expected %q to fail", input)
		}
	}
}

func TestPINHashesAreSaltedAndVerifiable(t *testing.T) {
	s, _, _ := newTestService()
	one, _ := s.hashPIN("1234")
	two, _ := s.hashPIN("1234")
	if one == "1234" || one == two {
		t.Fatal("PIN hashes must be salted and not plaintext")
	}
	ok, err := s.comparePIN(one, "1234")
	if err != nil || !ok {
		t.Fatalf("verify failed: %v", err)
	}
	ok, _ = s.comparePIN(one, "9999")
	if ok {
		t.Fatal("wrong PIN verified")
	}
}

func TestRegisterLoginRefreshAndReplayRejection(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	registered, err := s.Register(ctx, "9876543210", "1234", domain.RoleHirer)
	if err != nil {
		t.Fatal(err)
	}
	if registered.User.PhoneNumber != "+919876543210" || registered.ExpiresIn != 900 {
		t.Fatalf("unexpected result: %+v", registered)
	}
	loggedIn, err := s.Login(ctx, "+919876543210", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyAccess(loggedIn.AccessToken); err != nil {
		t.Fatalf("access token invalid: %v", err)
	}
	refreshed, err := s.Refresh(ctx, loggedIn.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == loggedIn.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if _, err := s.Refresh(ctx, loggedIn.RefreshToken); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("expected replay rejection, got %v", err)
	}
}

func TestFiveFailuresLockAccount(t *testing.T) {
	s, users, _ := newTestService()
	_, err := s.Register(context.Background(), "9876543210", "1234", domain.RoleLabourer)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err = s.Login(context.Background(), "9876543210", "9999"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if _, err = s.Login(context.Background(), "9876543210", "9999"); err == nil {
		t.Fatal("expected lockout")
	}
	var locked *AccountLockedError
	if !errors.As(err, &locked) || users.lockedUntil == nil {
		t.Fatalf("expected account lock, got %v", err)
	}
}

func TestSuccessfulLoginResetsFailedAttempts(t *testing.T) {
	s, users, _ := newTestService()
	if _, err := s.Register(context.Background(), "9876543210", "1234", domain.RoleHirer); err != nil {
		t.Fatal(err)
	}
	users.attempts = 3
	if _, err := s.Login(context.Background(), "9876543210", "1234"); err != nil {
		t.Fatal(err)
	}
	if users.attempts != 0 || users.lockedUntil != nil {
		t.Fatal("successful login did not reset failures")
	}
}

func TestRejectsExpiredAndMalformedAccessTokens(t *testing.T) {
	s, _, _ := newTestService()
	if _, err := s.VerifyAccess("not-a-token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("malformed token: %v", err)
	}
	claims := jwt.RegisteredClaims{Issuer: "sahayak-api", Subject: "user-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyAccess(expired); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token: %v", err)
	}
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	result, err := s.Register(ctx, "9876543210", "1234", domain.RoleHirer)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(ctx, result.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, result.RefreshToken); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("expected revoked session, got %v", err)
	}
}
