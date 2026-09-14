# Sahayak backend

Go API for Sahayak with `HIRER` and `LABOURER` accounts, four-digit PIN authentication, PostgreSQL, and offline-payment records.

## Authentication

- Registration and login use an Indian mobile number plus a four-digit PIN.
- Phone ownership is not verified in this MVP.
- PINs are protected with Argon2id, a unique salt, and `PIN_PEPPER`.
- Access tokens expire after 15 minutes. Refresh tokens expire after 30 days and rotate on use.
- Five failed PIN attempts lock the account for 15 minutes.
- PIN recovery is not available yet.

## Configuration

Create `server/.env` locally. It is excluded from Git.

- `PORT` defaults to `8080`.
- `ALLOWED_ORIGIN` defaults to `*`.
- `DATABASE_URL` is required when running Go directly.
- `PIN_PEPPER` and `JWT_SECRET` must be different random values of at least 32 characters.
- `POSTGRES_PASSWORD` and `API_PORT` are used by Docker Compose.

Generate separate PIN/JWT secrets with `openssl rand -base64 48`. For the Compose database password, use `openssl rand -hex 32` so it can safely appear in the connection URL. Never commit the resulting values.

## Run directly

Apply every `*.up.sql` migration in numeric order, then run:

```sh
go run ./cmd/api
```

Migration `000003` intentionally clears development users, jobs, and payments while replacing Firebase identities with PIN credentials.

## Run with Docker

For native Windows Server deployment without Docker, see [WINDOWS_DEPLOYMENT.md](WINDOWS_DEPLOYMENT.md).

From `server/`, create `.env` with `POSTGRES_PASSWORD`, `PIN_PEPPER`, and `JWT_SECRET` set to the separately generated values above. Optionally set `API_PORT=8080` and `ALLOWED_ORIGIN=*`. Environment examples are intentionally not distributed in Git.

```sh
docker compose up --build -d
curl http://localhost:8080/health
```

PostgreSQL data is persisted in the `sahayak_postgres_data` Docker volume. The migration container runs before the API starts.

## Verify

```sh
go test -race ./...
go vet ./...
go build ./cmd/api
docker build -t sahayak-api .
```

To include the PostgreSQL lockout regression test, set `TEST_DATABASE_URL` to a disposable database with all migrations applied before running `go test -race ./...`. Without it, that test is skipped.

Complete API documentation is in [`API.md`](API.md), with an OpenAPI 3.1 definition in [`openapi.yaml`](openapi.yaml).
