package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	appauth "github.com/darshankochar22/sahayak/server/internal/auth"
	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Authenticator interface {
	Register(context.Context, string, string, domain.Role) (appauth.Result, error)
	Login(context.Context, string, string) (appauth.Result, error)
	Refresh(context.Context, string) (appauth.Result, error)
	Logout(context.Context, string) error
	VerifyAccess(string) (string, error)
}

type Handler struct {
	auth          Authenticator
	users         store.UserStore
	payments      store.PaymentStore
	allowedOrigin string
}
type principal struct{ UserID string }
type contextKey string

const principalKey contextKey = "principal"

func New(auth Authenticator, users store.UserStore, allowedOrigin string) *Handler {
	return &Handler{auth: auth, users: users, allowedOrigin: allowedOrigin}
}
func NewWithPayments(auth Authenticator, users store.UserStore, payments store.PaymentStore, allowedOrigin string) *Handler {
	return &Handler{auth: auth, users: users, payments: payments, allowedOrigin: allowedOrigin}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /v1/auth/register", h.register)
	mux.HandleFunc("POST /v1/auth/login", h.login)
	mux.HandleFunc("POST /v1/auth/refresh", h.refresh)
	mux.HandleFunc("POST /v1/auth/logout", h.logout)
	mux.Handle("GET /v1/me", h.requireAccessToken(http.HandlerFunc(h.me)))
	if h.payments != nil {
		mux.Handle("GET /v1/jobs/{jobID}/payment", h.requireAccessToken(http.HandlerFunc(h.getPayment)))
		mux.Handle("POST /v1/jobs/{jobID}/payment/mark-paid", h.requireAccessToken(http.HandlerFunc(h.markPaymentPaid)))
	}
	return h.cors(mux)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PhoneNumber string      `json:"phoneNumber"`
		PIN         string      `json:"pin"`
		Role        domain.Role `json:"role"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := h.auth.Register(r.Context(), body.PhoneNumber, body.PIN, body.Role)
	if errors.Is(err, appauth.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "INVALID_REGISTRATION", "enter a valid Indian phone number, four-digit PIN, and role")
		return
	}
	if errors.Is(err, store.ErrPhoneExists) {
		writeError(w, http.StatusConflict, "PHONE_ALREADY_REGISTERED", "phone number is already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "could not create account")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PhoneNumber string `json:"phoneNumber"`
		PIN         string `json:"pin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := h.auth.Login(r.Context(), body.PhoneNumber, body.PIN)
	var locked *appauth.AccountLockedError
	if errors.As(err, &locked) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"code": "ACCOUNT_LOCKED", "message": "too many incorrect attempts; try again later", "retryAt": locked.Until}})
		return
	}
	if errors.Is(err, appauth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "phone number or PIN is incorrect")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LOGIN_FAILED", "could not log in")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := h.auth.Refresh(r.Context(), body.RefreshToken)
	if errors.Is(err, store.ErrSessionInvalid) {
		writeError(w, http.StatusUnauthorized, "SESSION_INVALID", "session is invalid or expired")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_REFRESH_FAILED", "could not refresh session")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := h.auth.Logout(r.Context(), body.RefreshToken); err != nil && !errors.Is(err, store.ErrSessionInvalid) {
		writeError(w, http.StatusInternalServerError, "LOGOUT_FAILED", "could not log out")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, err := h.users.ByID(r.Context(), currentUserID(r))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", "account was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USER_READ_FAILED", "could not load user profile")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request) {
	jobID, ok := validJobID(w, r)
	if !ok {
		return
	}
	payment, err := h.payments.ByJob(r.Context(), currentUserID(r), jobID)
	if errors.Is(err, store.ErrPaymentNotFound) {
		writeError(w, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYMENT_READ_FAILED", "could not load payment")
		return
	}
	writeJSON(w, http.StatusOK, payment)
}

func (h *Handler) markPaymentPaid(w http.ResponseWriter, r *http.Request) {
	jobID, ok := validJobID(w, r)
	if !ok {
		return
	}
	payment, err := h.payments.MarkPaidOffline(r.Context(), currentUserID(r), jobID)
	if errors.Is(err, store.ErrPaymentNotFound) {
		writeError(w, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment was not found or does not belong to this Hirer")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PAYMENT_UPDATE_FAILED", "could not update payment")
		return
	}
	writeJSON(w, http.StatusOK, payment)
}

func currentUserID(r *http.Request) string { return r.Context().Value(principalKey).(principal).UserID }

func validJobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	jobID := r.PathValue("jobID")
	if _, err := uuid.Parse(jobID); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JOB_ID", "job ID must be a valid UUID")
		return "", false
	}
	return jobID, true
}

func (h *Handler) requireAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "TOKEN_REQUIRED", "access token is required")
			return
		}
		userID, err := h.auth.VerifyAccess(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil || userID == "" {
			writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "access token is invalid or expired")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal{UserID: userID})))
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return false
	}
	return true
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", h.allowedOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
