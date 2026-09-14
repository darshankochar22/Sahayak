package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidInput       = errors.New("invalid authentication input")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTokenInvalid       = errors.New("token invalid")
)

type AccountLockedError struct{ Until time.Time }

func (e *AccountLockedError) Error() string { return "account locked" }

type Result struct {
	User         domain.User `json:"user"`
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	ExpiresIn    int         `json:"expiresIn"`
}

type Service struct {
	users     store.UserStore
	sessions  store.SessionStore
	pepper    []byte
	jwtSecret []byte
	now       func() time.Time
}

const (
	accessTTL    = 15 * time.Minute
	refreshTTL   = 30 * 24 * time.Hour
	lockDuration = 15 * time.Minute
	lockAfter    = 5
)

func NewService(users store.UserStore, sessions store.SessionStore, pepper, jwtSecret string) *Service {
	return &Service{users: users, sessions: sessions, pepper: []byte(pepper), jwtSecret: []byte(jwtSecret), now: time.Now}
}

func (s *Service) Register(ctx context.Context, phone, pin string, role domain.Role) (Result, error) {
	phone, err := NormalizeIndianPhone(phone)
	if err != nil || !validPIN(pin) || !role.Valid() {
		return Result{}, ErrInvalidInput
	}
	hash, err := s.hashPIN(pin)
	if err != nil {
		return Result{}, err
	}
	user, err := s.users.Create(ctx, phone, hash, role)
	if err != nil {
		return Result{}, err
	}
	return s.issue(ctx, user)
}

func (s *Service) Login(ctx context.Context, phone, pin string) (Result, error) {
	phone, err := NormalizeIndianPhone(phone)
	if err != nil || !validPIN(pin) {
		return Result{}, ErrInvalidCredentials
	}
	credentials, err := s.users.ByPhone(ctx, phone)
	if errors.Is(err, pgx.ErrNoRows) {
		argon2.IDKey(append([]byte(pin), s.pepper...), []byte("unknown-user-salt"), 2, 64*1024, 1, 32)
		return Result{}, ErrInvalidCredentials
	}
	if err != nil {
		return Result{}, err
	}
	now := s.now()
	if credentials.LockedUntil != nil && now.Before(*credentials.LockedUntil) {
		return Result{}, &AccountLockedError{Until: *credentials.LockedUntil}
	}
	ok, err := s.comparePIN(credentials.PINHash, pin)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		until, updateErr := s.users.RecordFailedLogin(ctx, credentials.ID, lockAfter, lockDuration)
		if updateErr != nil {
			return Result{}, updateErr
		}
		if until != nil && now.Before(*until) {
			return Result{}, &AccountLockedError{Until: *until}
		}
		return Result{}, ErrInvalidCredentials
	}
	if err := s.users.ResetLoginFailures(ctx, credentials.ID); err != nil {
		return Result{}, err
	}
	return s.issue(ctx, credentials.User)
}

func (s *Service) Refresh(ctx context.Context, oldToken string) (Result, error) {
	if oldToken == "" {
		return Result{}, store.ErrSessionInvalid
	}
	newToken, err := randomToken()
	if err != nil {
		return Result{}, err
	}
	userID, err := s.sessions.Rotate(ctx, tokenHash(oldToken), tokenHash(newToken), s.now().Add(refreshTTL))
	if err != nil {
		return Result{}, err
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return Result{}, err
	}
	access, err := s.accessToken(user.ID)
	if err != nil {
		return Result{}, err
	}
	return Result{User: user, AccessToken: access, RefreshToken: newToken, ExpiresIn: int(accessTTL.Seconds())}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return store.ErrSessionInvalid
	}
	return s.sessions.Revoke(ctx, tokenHash(refreshToken))
}

func (s *Service) VerifyAccess(tokenString string) (string, error) {
	claims := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrTokenInvalid
		}
		return s.jwtSecret, nil
	}, jwt.WithIssuer("sahayak-api"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", ErrTokenInvalid
	}
	return claims.Subject, nil
}

func (s *Service) issue(ctx context.Context, user domain.User) (Result, error) {
	access, err := s.accessToken(user.ID)
	if err != nil {
		return Result{}, err
	}
	refresh, err := randomToken()
	if err != nil {
		return Result{}, err
	}
	if err := s.sessions.Create(ctx, user.ID, tokenHash(refresh), s.now().Add(refreshTTL)); err != nil {
		return Result{}, err
	}
	return Result{User: user, AccessToken: access, RefreshToken: refresh, ExpiresIn: int(accessTTL.Seconds())}, nil
}

func (s *Service) accessToken(userID string) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{Issuer: "sahayak-api", Subject: userID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func validPIN(pin string) bool { matched, _ := regexp.MatchString(`^[0-9]{4}$`, pin); return matched }

func NormalizeIndianPhone(value string) (string, error) {
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	digits := replacer.Replace(strings.TrimSpace(value))
	digits = strings.TrimPrefix(digits, "+")
	if len(digits) == 12 && strings.HasPrefix(digits, "91") {
		digits = digits[2:]
	}
	if len(digits) != 10 || digits[0] < '6' || digits[0] > '9' {
		return "", ErrInvalidInput
	}
	for _, char := range digits {
		if char < '0' || char > '9' {
			return "", ErrInvalidInput
		}
	}
	return "+91" + digits, nil
}

func (s *Service) hashPIN(pin string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey(append([]byte(pin), s.pepper...), salt, 2, 64*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func (s *Service) comparePIN(encoded, pin string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid PIN hash")
	}
	var memory uint64
	var iterations, parallelism uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, err
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false, errors.New("unsupported PIN hash")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey(append([]byte(pin), s.pepper...), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
