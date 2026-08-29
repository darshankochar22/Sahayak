package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/darshankochar22/sahayak/server/internal/domain"
	"github.com/darshankochar22/sahayak/server/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*firebaseauth.Token, error)
}

type Handler struct {
	auth          TokenVerifier
	users         store.UserStore
	payments      store.PaymentStore
	allowedOrigin string
}

type principal struct {
	UID   string
	Phone string
}

type contextKey string

const principalKey contextKey = "principal"

func New(auth TokenVerifier, users store.UserStore, allowedOrigin string) *Handler {
	return &Handler{auth: auth, users: users, allowedOrigin: allowedOrigin}
}

func NewWithPayments(auth TokenVerifier, users store.UserStore, payments store.PaymentStore, allowedOrigin string) *Handler {
	return &Handler{auth: auth, users: users, payments: payments, allowedOrigin: allowedOrigin}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("POST /v1/auth/register", h.requireFirebaseToken(http.HandlerFunc(h.register)))
	mux.Handle("GET /v1/me", h.requireFirebaseToken(http.HandlerFunc(h.me)))
	if h.payments != nil {
		mux.Handle("GET /v1/jobs/{jobID}/payment", h.requireFirebaseToken(http.HandlerFunc(h.getPayment)))
		mux.Handle("POST /v1/jobs/{jobID}/payment/mark-paid", h.requireFirebaseToken(http.HandlerFunc(h.markPaymentPaid)))
	}
	return h.cors(mux)
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request) {
	jobID, ok := validJobID(w, r)
	if !ok {
		return
	}
	p := r.Context().Value(principalKey).(principal)
	payment, err := h.payments.ByJob(r.Context(), p.UID, jobID)
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
	p := r.Context().Value(principalKey).(principal)
	payment, err := h.payments.MarkPaidOffline(r.Context(), p.UID, jobID)
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

func validJobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	jobID := r.PathValue("jobID")
	if _, err := uuid.Parse(jobID); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JOB_ID", "job ID must be a valid UUID")
		return "", false
	}
	return jobID, true
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var body struct {
		Role domain.Role `json:"role"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !body.Role.Valid() {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "role must be HIRER or LABOURER")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "role must be HIRER or LABOURER")
		return
	}

	p := r.Context().Value(principalKey).(principal)
	user, err := h.users.Upsert(r.Context(), p.UID, p.Phone, body.Role)
	if errors.Is(err, store.ErrRoleConflict) {
		writeError(w, http.StatusConflict, "ROLE_CONFLICT", "this account is already registered with a different role")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USER_SAVE_FAILED", "could not save user profile")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	user, err := h.users.ByFirebaseUID(r.Context(), p.UID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", "choose a role to finish registration")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USER_READ_FAILED", "could not load user profile")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) requireFirebaseToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "TOKEN_REQUIRED", "Firebase ID token is required")
			return
		}
		token, err := h.auth.VerifyIDToken(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil || token == nil || token.UID == "" {
			writeError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Firebase ID token is invalid or expired")
			return
		}
		phone, _ := token.Claims["phone_number"].(string)
		if phone == "" {
			writeError(w, http.StatusForbidden, "PHONE_REQUIRED", "account must be authenticated by phone number")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal{UID: token.UID, Phone: phone})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
