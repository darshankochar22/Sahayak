package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appauth "github.com/darshankochar22/sahayak/server/internal/auth"
	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
)

const testJobID = "11111111-1111-4111-8111-111111111111"

type fakeAuth struct {
	result appauth.Result
	err    error
	userID string
}

func (f fakeAuth) Register(context.Context, string, string, domain.Role) (appauth.Result, error) {
	return f.result, f.err
}
func (f fakeAuth) Login(context.Context, string, string) (appauth.Result, error) {
	return f.result, f.err
}
func (f fakeAuth) Refresh(context.Context, string) (appauth.Result, error) { return f.result, f.err }
func (f fakeAuth) Logout(context.Context, string) error                    { return f.err }
func (f fakeAuth) VerifyAccess(string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.userID, nil
}

type fakeUsers struct {
	user domain.User
	err  error
}

func (f *fakeUsers) Create(context.Context, string, string, domain.Role) (domain.User, error) {
	return f.user, f.err
}
func (f *fakeUsers) ByPhone(context.Context, string) (domain.UserCredentials, error) {
	return domain.UserCredentials{User: f.user}, f.err
}
func (f *fakeUsers) ByID(context.Context, string) (domain.User, error) { return f.user, f.err }
func (f *fakeUsers) RecordFailedLogin(context.Context, string, int, time.Duration) (*time.Time, error) {
	return nil, f.err
}
func (f *fakeUsers) ResetLoginFailures(context.Context, string) error { return f.err }

type fakePayments struct {
	payment   domain.Payment
	err       error
	gotUserID string
}

func (f *fakePayments) ByJob(_ context.Context, userID, _ string) (domain.Payment, error) {
	f.gotUserID = userID
	return f.payment, f.err
}
func (f *fakePayments) MarkPaidOffline(_ context.Context, userID, _ string) (domain.Payment, error) {
	f.gotUserID = userID
	return f.payment, f.err
}

func testHandler(auth fakeAuth, users *fakeUsers) http.Handler { return New(auth, users, "*").Routes() }

func TestHealth(t *testing.T) {
	r := httptest.NewRecorder()
	testHandler(fakeAuth{}, &fakeUsers{}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/health", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", r.Code)
	}
}

func TestRegisterIsPublicAndReturnsSession(t *testing.T) {
	result := appauth.Result{User: domain.User{ID: "user-1", PhoneNumber: "+919876543210", Role: domain.RoleHirer}, AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900}
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"phoneNumber":"9876543210","pin":"1234","role":"HIRER"}`))
	testHandler(fakeAuth{result: result}, &fakeUsers{}).ServeHTTP(r, req)
	if r.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", r.Code, r.Body.String())
	}
}

func TestRegisterRejectsUnexpectedFields(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"phoneNumber":"9876543210","pin":"1234","role":"HIRER","admin":true}`))
	testHandler(fakeAuth{}, &fakeUsers{}).ServeHTTP(r, req)
	assertError(t, r, http.StatusBadRequest, "INVALID_REQUEST")
}

func TestLoginInvalidCredentials(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"phoneNumber":"9876543210","pin":"9999"}`))
	testHandler(fakeAuth{err: appauth.ErrInvalidCredentials}, &fakeUsers{}).ServeHTTP(r, req)
	assertError(t, r, http.StatusUnauthorized, "INVALID_CREDENTIALS")
}

func TestLoginLocked(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"phoneNumber":"9876543210","pin":"9999"}`))
	testHandler(fakeAuth{err: &appauth.AccountLockedError{Until: time.Now().Add(time.Minute)}}, &fakeUsers{}).ServeHTTP(r, req)
	assertError(t, r, http.StatusTooManyRequests, "ACCOUNT_LOCKED")
}

func TestRefreshRejectsReusedSession(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refreshToken":"used"}`))
	testHandler(fakeAuth{err: store.ErrSessionInvalid}, &fakeUsers{}).ServeHTTP(r, req)
	assertError(t, r, http.StatusUnauthorized, "SESSION_INVALID")
}

func TestLogoutIsIdempotentForInvalidSession(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader(`{"refreshToken":"used"}`))
	testHandler(fakeAuth{err: store.ErrSessionInvalid}, &fakeUsers{}).ServeHTTP(r, req)
	if r.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", r.Code)
	}
}

func TestProtectedRouteRequiresToken(t *testing.T) {
	r := httptest.NewRecorder()
	testHandler(fakeAuth{userID: "user-1"}, &fakeUsers{}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	assertError(t, r, http.StatusUnauthorized, "TOKEN_REQUIRED")
}

func TestProtectedRouteRejectsInvalidToken(t *testing.T) {
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	testHandler(fakeAuth{err: errors.New("bad token")}, &fakeUsers{}).ServeHTTP(r, req)
	assertError(t, r, http.StatusUnauthorized, "TOKEN_INVALID")
}

func TestMeReturnsProfile(t *testing.T) {
	users := &fakeUsers{user: domain.User{ID: "user-1", PhoneNumber: "+919876543210", Role: domain.RoleLabourer}}
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	testHandler(fakeAuth{userID: "user-1"}, users).ServeHTTP(r, req)
	if r.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", r.Code, r.Body.String())
	}
}

func TestPaymentUsesAuthenticatedUserID(t *testing.T) {
	payments := &fakePayments{payment: domain.Payment{ID: "payment-1", JobID: testJobID, AmountPaise: 90000, Currency: "INR", Status: domain.PaymentPending}}
	h := NewWithPayments(fakeAuth{userID: "user-1"}, &fakeUsers{}, payments, "*").Routes()
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+testJobID+"/payment", nil)
	req.Header.Set("Authorization", "Bearer valid")
	h.ServeHTTP(r, req)
	if r.Code != http.StatusOK || payments.gotUserID != "user-1" {
		t.Fatalf("status=%d user=%s body=%s", r.Code, payments.gotUserID, r.Body.String())
	}
}

func TestPaymentRejectsInvalidJobID(t *testing.T) {
	h := NewWithPayments(fakeAuth{userID: "user-1"}, &fakeUsers{}, &fakePayments{}, "*").Routes()
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/bad/payment", nil)
	req.Header.Set("Authorization", "Bearer valid")
	h.ServeHTTP(r, req)
	assertError(t, r, http.StatusBadRequest, "INVALID_JOB_ID")
}

func assertError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, recorder.Code, recorder.Body.String())
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("expected %s, got %s", code, response.Error.Code)
	}
}
