# Sahayak backend

Go API for Sahayak, a role-based unloading-work application.

## Implemented

- Firebase phone-auth ID-token verification
- Fixed `HIRER` and `LABOURER` registration
- Authenticated profile retrieval
- PostgreSQL user, job-cost, and offline-payment schema
- Automatic pending offline-payment record for each job
- Hirer-owned payment lookup and `PAID_OFFLINE` tracking

## Local configuration

Create `server/.env` locally. It is intentionally excluded from Git. Configure:

- `PORT` (defaults to `8080`)
- `ALLOWED_ORIGIN` (defaults to `*`)
- `DATABASE_URL`
- `GOOGLE_APPLICATION_CREDENTIALS` for a local Firebase service-account JSON
- `GOOGLE_CLOUD_PROJECT`

On Cloud Run, omit `GOOGLE_APPLICATION_CREDENTIALS` and use the runtime service account through Application Default Credentials.

Apply all `*.up.sql` files from `migrations/` in numeric order before starting the API.

## Run

```sh
go run ./cmd/api
```

## Verify

```sh
go test -race ./...
go vet ./...
go build ./cmd/api
```

## Endpoints

Complete documentation:

- [`API.md`](API.md) — human-readable reference with examples and error codes
- [`openapi.yaml`](openapi.yaml) — OpenAPI 3.1 specification for Swagger/Postman/client generation

- `GET /health`
- `POST /v1/auth/register`
- `GET /v1/me`
- `GET /v1/jobs/{jobId}/payment`
- `POST /v1/jobs/{jobId}/payment/mark-paid`

Every `/v1` endpoint requires `Authorization: Bearer <firebase-id-token>`.

Payments are records only: Sahayak does not process money. The Hirer pays outside the app and marks the payment `PAID_OFFLINE`.
