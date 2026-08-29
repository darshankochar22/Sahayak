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

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
	"github.com/jackc/pgx/v5"
)

const testJobID = "11111111-1111-4111-8111-111111111111"

type fakeVerifier struct {
	token *firebaseauth.Token
	err   error
}

func (f fakeVerifier) VerifyIDToken(context.Context, string) (*firebaseauth.Token, error) {
	return f.token, f.err
}

type fakeUserStore struct {
	user       domain.User
	findErr    error
	upsertErr  error
	upsertRole domain.Role
}

type fakePaymentStore struct {
	payment domain.Payment
	getErr  error
	markErr error
}

func (f *fakePaymentStore) ByJob(context.Context, string, string) (domain.Payment, error) {
	return f.payment, f.getErr
}

func (f *fakePaymentStore) MarkPaidOffline(context.Context, string, string) (domain.Payment, error) {
	return f.payment, f.markErr
}

func (f *fakeUserStore) Upsert(_ context.Context, firebaseUID, phoneNumber string, role domain.Role) (domain.User, error) {
	f.upsertRole = role
	if f.upsertErr != nil {
		return domain.User{}, f.upsertErr
	}
	user := f.user
	user.FirebaseUID = firebaseUID
	user.PhoneNumber = phoneNumber
	user.Role = role
	return user, nil
}

func (f *fakeUserStore) ByFirebaseUID(context.Context, string) (domain.User, error) {
	return f.user, f.findErr
}

func authenticatedHandler(users *fakeUserStore) http.Handler {
	return New(fakeVerifier{token: &firebaseauth.Token{
		UID:    "firebase-user-1",
		Claims: map[string]interface{}{"phone_number": "+919876543210"},
	}}, users, "*").Routes()
}

func authenticatedPaymentHandler(payments *fakePaymentStore) http.Handler {
	return NewWithPayments(fakeVerifier{token: &firebaseauth.Token{
		UID:    "firebase-user-1",
		Claims: map[string]interface{}{"phone_number": "+919876543210"},
	}}, &fakeUserStore{}, payments, "*").Routes()
}

func TestHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	authenticatedHandler(&fakeUserStore{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestProtectedRouteRequiresToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	authenticatedHandler(&fakeUserStore{}).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusUnauthorized, "TOKEN_REQUIRED")
}

func TestProtectedRouteRejectsInvalidToken(t *testing.T) {
	handler := New(fakeVerifier{err: errors.New("invalid token")}, &fakeUserStore{}, "*").Routes()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	handler.ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusUnauthorized, "TOKEN_INVALID")
}

func TestProtectedRouteRequiresPhoneAuthentication(t *testing.T) {
	handler := New(fakeVerifier{token: &firebaseauth.Token{
		UID:    "firebase-user-1",
		Claims: map[string]interface{}{},
	}}, &fakeUserStore{}, "*").Routes()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	handler.ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusForbidden, "PHONE_REQUIRED")
}

func TestRegisterValidatesRole(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"role":"ADMIN"}`))
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(&fakeUserStore{}).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusBadRequest, "INVALID_ROLE")
}

func TestRegisterCreatesHirerProfile(t *testing.T) {
	users := &fakeUserStore{user: domain.User{ID: "user-1", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"role":"HIRER"}`))
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(users).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if users.upsertRole != domain.RoleHirer {
		t.Fatalf("expected HIRER to be saved, got %s", users.upsertRole)
	}
	var response domain.User
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.PhoneNumber != "+919876543210" {
		t.Fatalf("unexpected registered user: %+v", response)
	}
}

func TestRegisterRejectsUnexpectedFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"role":"HIRER","admin":true}`))
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(&fakeUserStore{}).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusBadRequest, "INVALID_ROLE")
}

func TestProtectedRouteRejectsTokenWithoutUID(t *testing.T) {
	handler := New(fakeVerifier{token: &firebaseauth.Token{
		Claims: map[string]interface{}{"phone_number": "+919876543210"},
	}}, &fakeUserStore{}, "*").Routes()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	handler.ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusUnauthorized, "TOKEN_INVALID")
}

func TestRegisterCreatesLabourerProfile(t *testing.T) {
	users := &fakeUserStore{user: domain.User{ID: "user-2"}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"role":"LABOURER"}`))
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(users).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if users.upsertRole != domain.RoleLabourer {
		t.Fatalf("expected LABOURER to be saved, got %s", users.upsertRole)
	}
}

func TestRegisterRejectsDifferentRoleForExistingAccount(t *testing.T) {
	users := &fakeUserStore{upsertErr: store.ErrRoleConflict}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(`{"role":"LABOURER"}`))
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(users).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusConflict, "ROLE_CONFLICT")
}

func TestMeReturnsProfile(t *testing.T) {
	users := &fakeUserStore{user: domain.User{ID: "user-1", FirebaseUID: "firebase-user-1", PhoneNumber: "+919876543210", Role: domain.RoleLabourer}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(users).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMeReturnsNotFoundBeforeRegistration(t *testing.T) {
	users := &fakeUserStore{findErr: pgx.ErrNoRows}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedHandler(users).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusNotFound, "PROFILE_NOT_FOUND")
}

func TestGetOfflinePayment(t *testing.T) {
	payments := &fakePaymentStore{payment: domain.Payment{
		ID:          "payment-1",
		JobID:       testJobID,
		AmountPaise: 90000,
		Currency:    "INR",
		Status:      domain.PaymentPending,
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+testJobID+"/payment", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedPaymentHandler(payments).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response domain.Payment
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != domain.PaymentPending || response.AmountPaise != 90000 {
		t.Fatalf("unexpected payment: %+v", response)
	}
}

func TestMarkOfflinePaymentPaid(t *testing.T) {
	now := time.Now()
	payments := &fakePaymentStore{payment: domain.Payment{
		ID:          "payment-1",
		JobID:       testJobID,
		AmountPaise: 90000,
		Currency:    "INR",
		Status:      domain.PaymentPaidOffline,
		PaidAt:      &now,
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+testJobID+"/payment/mark-paid", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedPaymentHandler(payments).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMarkOfflinePaymentHidesUnauthorizedJob(t *testing.T) {
	payments := &fakePaymentStore{markErr: store.ErrPaymentNotFound}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+testJobID+"/payment/mark-paid", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedPaymentHandler(payments).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusNotFound, "PAYMENT_NOT_FOUND")
}

func TestPaymentRejectsInvalidJobID(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/not-a-uuid/payment", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authenticatedPaymentHandler(&fakePaymentStore{}).ServeHTTP(recorder, request)

	assertError(t, recorder, http.StatusBadRequest, "INVALID_JOB_ID")
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
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("expected error %s, got %s", code, response.Error.Code)
	}
}
