# Sahayak API reference

This document describes every endpoint currently exposed by the Sahayak Go backend. The machine-readable equivalent is [`openapi.yaml`](openapi.yaml).

## Conventions

### Base URL

Local development:

```text
http://localhost:8080
```

The production Cloud Run URL will be added after deployment.

### Authentication

Every `/v1` endpoint requires a Firebase ID token obtained after successful phone OTP authentication:

```http
Authorization: Bearer <firebase-id-token>
```

The backend verifies the token with Firebase Admin and requires a non-empty Firebase `phone_number` claim. The internal Firebase UID is never returned in API responses.

Common authentication failures:

| HTTP | Code | Meaning |
|---|---|---|
| `401` | `TOKEN_REQUIRED` | The Authorization header is missing or is not a Bearer token. |
| `401` | `TOKEN_INVALID` | The Firebase token is invalid, expired, or missing a UID. |
| `403` | `PHONE_REQUIRED` | The Firebase account was not authenticated by phone number. |

### Content type

JSON requests must send:

```http
Content-Type: application/json
```

JSON responses use:

```http
Content-Type: application/json
```

### Error format

All application errors use one shape:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable explanation"
  }
}
```

### Money

Money is represented as integer paise to avoid floating-point errors:

```text
₹900.00 = 90000 paise
```

The current currency is always `INR`. Sahayak only records offline payment status and does not process money.

### Roles

| Value | Meaning |
|---|---|
| `HIRER` | Creates unloading work and hires labourers. |
| `LABOURER` | Receives and accepts unloading work. |

The role is fixed after registration. Re-registering with the same role is safe; requesting the opposite role returns `ROLE_CONFLICT`.

## `GET /health`

Checks whether the HTTP service is running. This endpoint is public and does not require Firebase authentication.

Request:

```http
GET /health HTTP/1.1
Host: localhost:8080
```

Successful response — `200 OK`:

```json
{
  "status": "ok"
}
```

Important: this is currently a process-health check. It does not independently query Firebase or PostgreSQL on each request.

## `POST /v1/auth/register`

Creates the Sahayak database profile after Firebase phone OTP succeeds. It can also safely return an existing profile when the same account registers again with its existing role.

Request body limit: 1 KiB. Unknown JSON fields, multiple JSON values, empty bodies, and unsupported roles are rejected.

Request:

```http
POST /v1/auth/register HTTP/1.1
Host: localhost:8080
Authorization: Bearer <firebase-id-token>
Content-Type: application/json

{
  "role": "HIRER"
}
```

The role may be `HIRER` or `LABOURER`.

Successful response — `200 OK`:

```json
{
  "id": "6c0ad00c-e70c-45a8-971f-704128355a1d",
  "phoneNumber": "+919876543210",
  "role": "HIRER",
  "createdAt": "2026-08-29T14:30:00Z",
  "updatedAt": "2026-08-29T14:30:00Z"
}
```

Endpoint-specific failures:

| HTTP | Code | Meaning |
|---|---|---|
| `400` | `INVALID_ROLE` | The body is invalid or role is not `HIRER`/`LABOURER`. |
| `409` | `ROLE_CONFLICT` | The account already has the opposite role. |
| `500` | `USER_SAVE_FAILED` | The profile could not be stored. |

Authentication failures listed under [Authentication](#authentication) may also be returned.

## `GET /v1/me`

Returns the Sahayak profile belonging to the authenticated Firebase account.

Request:

```http
GET /v1/me HTTP/1.1
Host: localhost:8080
Authorization: Bearer <firebase-id-token>
```

Successful response — `200 OK`:

```json
{
  "id": "6c0ad00c-e70c-45a8-971f-704128355a1d",
  "phoneNumber": "+919876543210",
  "role": "LABOURER",
  "createdAt": "2026-08-29T14:30:00Z",
  "updatedAt": "2026-08-29T14:30:00Z"
}
```

Endpoint-specific failures:

| HTTP | Code | Meaning |
|---|---|---|
| `404` | `PROFILE_NOT_FOUND` | Firebase login succeeded, but the Sahayak registration step has not been completed. |
| `500` | `USER_READ_FAILED` | The profile could not be read. |

## `GET /v1/jobs/{jobId}/payment`

Returns the offline-payment record for a job owned by the authenticated Hirer.

Path parameter:

| Name | Type | Required | Description |
|---|---|---|---|
| `jobId` | UUID | Yes | Identifier of the unloading job. |

Request:

```http
GET /v1/jobs/11111111-1111-4111-8111-111111111111/payment HTTP/1.1
Host: localhost:8080
Authorization: Bearer <firebase-id-token>
```

Successful pending response — `200 OK`:

```json
{
  "id": "7560fcfa-9858-4ad3-8077-a6b44fa0020c",
  "jobId": "11111111-1111-4111-8111-111111111111",
  "amountPaise": 90000,
  "currency": "INR",
  "status": "PENDING",
  "paidAt": null,
  "createdAt": "2026-08-29T14:35:00Z",
  "updatedAt": "2026-08-29T14:35:00Z"
}
```

Endpoint-specific failures:

| HTTP | Code | Meaning |
|---|---|---|
| `400` | `INVALID_JOB_ID` | `jobId` is not a valid UUID. |
| `404` | `PAYMENT_NOT_FOUND` | No visible payment exists for this Hirer and job. |
| `500` | `PAYMENT_READ_FAILED` | The payment could not be read. |

The `404` response intentionally does not reveal whether another user owns the job.

## `POST /v1/jobs/{jobId}/payment/mark-paid`

Records that the owning Hirer paid the agreed amount outside Sahayak. It does not initiate or verify a bank, cash, card, or UPI transaction.

This operation is idempotent. Calling it again keeps the payment in `PAID_OFFLINE` state and preserves the original `paidAt` timestamp.

The request has no body.

Request:

```http
POST /v1/jobs/11111111-1111-4111-8111-111111111111/payment/mark-paid HTTP/1.1
Host: localhost:8080
Authorization: Bearer <firebase-id-token>
```

Successful response — `200 OK`:

```json
{
  "id": "7560fcfa-9858-4ad3-8077-a6b44fa0020c",
  "jobId": "11111111-1111-4111-8111-111111111111",
  "amountPaise": 90000,
  "currency": "INR",
  "status": "PAID_OFFLINE",
  "paidAt": "2026-08-29T16:00:00Z",
  "createdAt": "2026-08-29T14:35:00Z",
  "updatedAt": "2026-08-29T16:00:00Z"
}
```

Endpoint-specific failures:

| HTTP | Code | Meaning |
|---|---|---|
| `400` | `INVALID_JOB_ID` | `jobId` is not a valid UUID. |
| `404` | `PAYMENT_NOT_FOUND` | Payment does not exist or the authenticated user is not the owning Hirer. |
| `500` | `PAYMENT_UPDATE_FAILED` | The payment status could not be updated. |

## CORS and preflight

The API accepts `GET`, `POST`, and `OPTIONS`. It allows the `Authorization` and `Content-Type` headers. `OPTIONS` requests return `204 No Content`.

`ALLOWED_ORIGIN` controls the response origin. Native Flutter apps do not rely on browser CORS, but the value should be restricted before adding a web client.

## Current scope boundary

Only the endpoints above exist today. The database already contains foundational job-cost and offline-payment tables, but job creation, labourer discovery, invitations, accept/reject, live status, maps, and notifications are not yet exposed as APIs. They must not be assumed from the current schema.
