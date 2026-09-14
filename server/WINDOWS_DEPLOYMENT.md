# Native Windows Server deployment

Target: Windows Server 2019 x64, including the existing VMware guest. No Docker,
Hyper-V, Linux VM, Firebase, or GCP is required. Authentication remains phone/PIN.
This guide is preparation, not evidence that the remote server has been deployed.

## 1. Preflight with the administrator

Confirm administrator access, OS evaluation expiry/licensing, free resources,
backup destination, maintenance window, and whether users connect over LAN or
the internet. Check existing PostgreSQL installations and listeners before
installing anything. Do not replace another application's database or web server.

```powershell
Get-Service *postgres* -ErrorAction SilentlyContinue
Get-NetTCPConnection -State Listen |
    Where-Object { $_.LocalPort -in 80,443,5432,8085 } |
    Select-Object LocalAddress, LocalPort, OwningProcess
```

## 2. Build and copy the backend

From `server/`, cross-compile on a build machine with the Go version in `go.mod`:

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dist/windows-amd64/sahayak-api.exe ./cmd/api
```

Or build using PowerShell on a Windows development machine:

```powershell
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -o dist/windows-amd64/sahayak-api.exe ./cmd/api
if ($LASTEXITCODE -ne 0) { throw 'Backend build failed' }
```

Go is not needed on the deployment server when copying a prebuilt executable.
Use a dedicated directory such as `C:\Sahayak\api`, not a user's Desktop. Copy
the executable and `deploy/windows/SahayakService.xml` there. Keep migrations
in a separate administrator-managed directory. Do not copy the development `.env`.

## 3. Install and prepare PostgreSQL

If PostgreSQL is already installed, first confirm its version and authorized
access with the administrator. Reuse it with a dedicated Sahayak database/login
when compatible. Do not reinstall, restart, or change shared instance settings
without checking the impact on existing applications.

For a new installation, use the latest patched PostgreSQL **17 x64** installer linked from the
[official Windows downloads page](https://www.postgresql.org/download/windows/).
Its tested platforms include Windows Server 2019. Install server and command-line
tools; Stack Builder is not required for this application. The installer sets up
the database service. Record its actual service name.

For a dedicated local instance, keep PostgreSQL `listen_addresses` set to `localhost`; use SCRAM password
authentication, not `trust`. Do not expose TCP 5432 to the LAN or internet.
Create a dedicated `sahayak` login and database owned by that login. Do not run
the API using PostgreSQL's `postgres` superuser. Use an administrator's interactive
`psql` session and `\password sahayak` to set the password without putting it in
shell history. A random hexadecimal password avoids URL-escaping problems.

Apply migrations with the project's migration runner, for example the official
[golang-migrate Windows CLI](https://github.com/golang-migrate/migrate/releases):

```powershell
# DATABASE_URL must already be set privately in this session.
# Run from the copied server directory containing migrations/.
.\migrate.exe -path .\migrations -database $env:DATABASE_URL up
if ($LASTEXITCODE -ne 0) { throw 'Migration failed; do not start the API' }
```

The CLI receives the database URL as an argument: use only a trusted administrator
session, do not record/transcribe this command with credentials, and clear the
session variable afterwards. The CLI tracks applied versions. Never run it
automatically during every service startup.

**Migration 000003 deletes users, jobs, payments, and dependent sessions.** On the
first deployment, use a new empty database. For any existing database, stop and
inspect its migration version and take a recoverable backup before proceeding.
Never reapply all SQL files manually to an existing deployment.

## 4. Create private production configuration

Create `.env` on the server in `C:\Sahayak\api` using an editor. No environment
file or real secret is included in this repository or build output.

Required settings:

- `DATABASE_URL`: PostgreSQL connection URL using the dedicated login,
  `127.0.0.1:5432`, and the `sahayak` database. `sslmode=disable` is acceptable
  only for this same-machine loopback connection.
- `PIN_PEPPER`: independently generated cryptographically random secret,
  at least 32 characters.
- `JWT_SECRET`: a different independently generated random secret,
  at least 32 characters.
- `PORT`: set to `8085` after checking it is free. This guide uses `8085`;
  the application's default remains `8080` when the setting is omitted.
- `ALLOWED_ORIGIN`: web-client origin if applicable; native Flutter is not
  controlled by browser CORS. CORS is not a firewall or authentication control.

Use a password manager's secure generator or a cryptographic RNG; never use sample
values. Back up these secrets securely. Changing `PIN_PEPPER` breaks existing PIN
logins. Changing `JWT_SECRET` invalidates existing access tokens.

Use NTFS Security settings to disable broad inherited access on the deployment
directory: Administrators and SYSTEM full control; LOCAL SERVICE read/execute;
LOCAL SERVICE modify on the `logs` subdirectory only. Create that subdirectory
first. Ordinary users must not read `.env` or modify the executable/service XML.
Do not grant LOCAL SERVICE write access to the executable or configuration.

## 5. Test interactively, then install the service

From `C:\Sahayak\api`, run `.\sahayak-api.exe` in PowerShell. In another terminal:

```powershell
Invoke-RestMethod http://127.0.0.1:8085/health
```

Stop the interactive process with Ctrl+C before starting the service.

Download the administrator-approved x64 executable from the official
[WinSW releases](https://github.com/winsw/winsw/releases). The supplied XML targets
WinSW 2.12.0; review release security guidance before installing. Rename the
wrapper to `SahayakService.exe` beside `SahayakService.xml`. Do not rename the
backend executable to the wrapper name. The XML sets the working directory so
the API reads its private `.env`, uses LocalService, restarts after failures,
and rotates logs. Verify the wrapper's documented runtime prerequisites.

After confirming the actual PostgreSQL service name, add a `<depend>` element to
the server's XML containing that name. In an elevated PowerShell:

```powershell
Set-Location C:\Sahayak\api
.\SahayakService.exe install
if ($LASTEXITCODE -ne 0) { throw 'Service installation failed' }
.\SahayakService.exe start
if ($LASTEXITCODE -ne 0) { throw 'Service start failed' }
Get-Service SahayakAPI
Invoke-RestMethod http://127.0.0.1:8085/health
```

Inspect `logs` for errors. Never send `.env` or logs containing credentials to
chat. Confirm stop/start and reboot behavior during an approved maintenance
window. A running service alone does not prove authentication/database health.

## 6. HTTPS and network access before client use

Put an administrator-managed reverse proxy in front of the API, for example
[Caddy as a Windows service](https://caddyserver.com/docs/running). Its upstream
is `127.0.0.1:8085`. Domain, certificates, proxy service account, and inbound rules
must be chosen with the client's IT team; do not replace an existing IIS binding.

The current API listens on all interfaces. Keep Windows Firewall enabled and
ensure remote access to its API port and PostgreSQL is blocked, including any
broad program-level allow rules. Expose only the HTTPS proxy as required. Do not
send PINs or bearer tokens over unencrypted LAN/public HTTP.

For mobile-data access, a private VMware/LAN IP is insufficient. Arrange DNS and
a reachable HTTPS endpoint using approved routing/NAT, VPN, or tunneling. Public
certificate issuance also requires the chosen challenge's prerequisites. For
LAN-only access, clients still need trusted TLS certificates. Do not disable TLS
validation in Flutter. Configure request rate limiting before public exposure:
four-digit PINs have low entropy and account lockout alone is not sufficient.

## 7. Acceptance, backups, and updates

- Test registration and PIN login for HIRER and LABOURER; `/v1/me` must return the
  authenticated user. Test invalid/missing tokens and fixed-role enforcement.
- Test refresh rotation, replay rejection, logout, wrong PINs, five-attempt
  lockout, and recovery after the lock expires using dedicated test accounts.
- With valid job fixtures, test payment ownership and offline mark-paid behavior.
  Job creation/matching endpoints are not supplied by this deployment guide.
- Test the Flutter release build against the final HTTPS URL from its intended
  network, not just `localhost`. Do not treat `/health` as an end-to-end test.
- Schedule PostgreSQL `pg_dump` backups, copy them off this VM, restrict access,
  define retention, and prove a restore into a separate database. Back up secrets
  separately with equally strict access. A VMware snapshot alone is not the plan.
- For updates: retain the previous executable, back up the DB, stop the service,
  copy the new executable, apply only reviewed unapplied migrations, start, and
  test. Binary rollback is safe only if compatible with the resulting schema.

Native Windows service operation, firewall isolation, HTTPS, PostgreSQL
installation, and reboot recovery must be verified on the client's server.
