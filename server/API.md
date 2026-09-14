# Sahayak API reference

Base URL for local development: `http://localhost:8080`.

All request and response bodies use JSON. Unknown request fields and multiple JSON values are rejected. Protected endpoints require `Authorization: Bearer <access-token>`.

## Authentication model

Users register and log in with an Indian mobile number and exactly four numeric PIN digits. Numbers are normalized to `+91XXXXXXXXXX`. Phone ownership is not verified. Access tokens last 15 minutes; refresh tokens last 30 days and are replaced each time they are used.

The common authenticated user shape is:

```json
{"id":"uuid","phoneNumber":"+919876543210","role":"HIRER","createdAt":"2026-09-14T10:00:00Z","updatedAt":"2026-09-14T10:00:00Z"}
```

Authentication responses contain:

```json
{"user":{},"accessToken":"...","refreshToken":"...","expiresIn":900}
```

## `GET /health`

Public process-health check. Returns `200` with `{"status":"ok"}`.

## `POST /v1/auth/register`

Public. Creates a new account and session.

```json
{"phoneNumber":"9876543210","pin":"1234","role":"HIRER"}
```

Returns `201` with an authentication response. Errors include `400 INVALID_REQUEST`, `400 INVALID_REGISTRATION`, `409 PHONE_ALREADY_REGISTERED`, and `500 REGISTRATION_FAILED`.

Roles are `HIRER` and `LABOURER` and cannot be changed after registration.

## `POST /v1/auth/login`

Public. Creates a session for valid credentials.

```json
{"phoneNumber":"+919876543210","pin":"1234"}
```

Returns `200` with an authentication response. An unknown phone and incorrect PIN both return `401 INVALID_CREDENTIALS`. Five failed attempts lock the account for 15 minutes; `429 ACCOUNT_LOCKED` includes `retryAt` in the error object.

## `POST /v1/auth/refresh`

Public. Atomically consumes one refresh token and returns a new authentication response.

```json
{"refreshToken":"..."}
```

Returns `200`. Expired, revoked, malformed, or previously used tokens return `401 SESSION_INVALID`.

## `POST /v1/auth/logout`

Public. Revokes the supplied refresh token. Repeating logout is safe.

```json
{"refreshToken":"..."}
```

Returns `204 No Content`.

## `GET /v1/me`

Protected. Returns `200` with the authenticated user. Errors include `401 TOKEN_REQUIRED`, `401 TOKEN_INVALID`, `404 PROFILE_NOT_FOUND`, and `500 USER_READ_FAILED`.

## `GET /v1/jobs/{jobId}/payment`

Protected. Returns the offline-payment record only when the authenticated user owns the job.

```json
{"id":"uuid","jobId":"uuid","amountPaise":90000,"currency":"INR","status":"PENDING","paidAt":null,"createdAt":"2026-09-14T10:00:00Z","updatedAt":"2026-09-14T10:00:00Z"}
```

Errors include `400 INVALID_JOB_ID`, `404 PAYMENT_NOT_FOUND`, and `500 PAYMENT_READ_FAILED`.

## `POST /v1/jobs/{jobId}/payment/mark-paid`

Protected. The owning hirer marks the record `PAID_OFFLINE`. No money is processed by Sahayak. Returns the updated payment. Errors include `400 INVALID_JOB_ID`, `404 PAYMENT_NOT_FOUND`, and `500 PAYMENT_UPDATE_FAILED`.

## Common error shape

```json
{"error":{"code":"TOKEN_INVALID","message":"access token is invalid or expired"}}
```
