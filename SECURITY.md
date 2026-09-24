# Security Policy for CamGuid

CamGuid is designed for **private local networks**. TLS terminates at a deployer-managed
reverse proxy in front of `:2005` (see README → Security). The API, relay, and scanner
ports are not exposed outside Docker.

## Version Support

| Version | Supported |
|---------|-----------|
| >= 1.0 (security-hardened) | ✅ |
| older | ❌ |

## Reporting a Vulnerability

Please report issues privately rather than opening a public issue. Include the service,
endpoint, and a minimal reproduction. Do not log secrets or credentials in the report.

## Threat Model & Guarantees

- **Transport** — the only externally reachable surface is the frontend proxy (`:2005`).
  Terminate TLS there; never expose api (`8080`), relay (`8082`), nvr-service (`8090`),
  the scanner's host stack, or PostgreSQL/Redis ports (`5432/5433`, `6379/6380`) publicly.
- **Authentication** — JWT access (15 min) + refresh (7 days) tokens via
  `POST /api/v1/auth/login`. Login is rate-limited per IP+username using Redis
  (`CAMGUID_LOGIN_RATE_LIMIT` / `CAMGUID_LOGIN_RATE_WINDOW`). Passwords are bcrypt-hashed.
- **Authorization (RBAC)** — roles `admin`, `operator`, `viewer`:
  - Device create/update/delete, channel generation, scan endpoints: `admin`/`operator`
  - User registration (admin only), audit log list/export: `admin`/`operator`
  - Dashboards, live wall, stream-token minting: any authenticated user
  - Server-side enforcement in the Go API; the frontend hides UI accordingly.
- **Bootstrap admin** — created once at first boot with `must_change_password=true`.
  Its password is `BOOTSTRAP_ADMIN_PASSWORD` or a one-time random value printed to the
  api logs. The UI forces a password change before any other action.
- **Secrets** — all services hard-fail on missing/placeholder secrets
  (`CHANGE_ME`, `changeme`, `example`, `super_secret`, default images, etc.):
  - `JWT_SECRET`, `ENCRYPTION_KEY` (64-hex, AES-256-GCM), `CAMGUID_STREAM_SIGNING_SECRET`
    for the Go services; `NVR_ENCRYPTION_KEY` (44-char Fernet), `NVR_STREAM_SIGNING_SECRET`
    for nvr-service. See `.env.example` and `scripts/generate-secrets.sh`.
- **Stream access** — `POST /api/v1/stream/token` mints a device-bound, expiring
  (5 min) HMAC-SHA256 token (payload: `did`, `uid`, `exp`). The relay verifies it locally
  (HMAC + PostgreSQL lookup via `ResolveStreamURL`), so no service ever learns a raw RTSP
  URL from the client. Tokens are accepted via `Authorization: Bearer`, `X-Stream-Token`,
  or `?token=`. A device id in a token must match the requested resource.
- **Data at rest** — device `stream_url` / `secondary_url` are AES-256-GCM encrypted
  (`enc1:` prefix) and never appear in API responses; `stream_configured` is the only hint.
  Legacy plaintext rows continue to decrypt (best-effort passthrough); new writes always encrypt.
  nvr-service NVR credentials use Fernet encryption.
- **Audit trail** — `audit_logs` records `auth.login`, `user.create`,
  `auth.password_change`, `device.create/update/delete` with actor, result
  (`success`/`fail`/`denied`), IP, and detail JSON. Retention via
  `CAMGUID_AUDIT_RETENTION_DAYS` (default 90). Admin/operator can export CSV.
- **Scanning** — the scanner's `/scan*` endpoints require `admin`/`operator` and are
  throttled (max concurrent jobs + min interval per user via Redis).
- **Streaming sessions** — snapshot/start/stop/heartbeat on the relay require a valid
  stream token; `POST /snapshot/stop/:deviceId` also verifies the token's device matches.
  Relay logs redact stream URLs.

## Threat Model Notes / Residual Risks

- Logins over plain HTTP are only safe on a trusted LAN; a TLS proxy is mandatory otherwise.
- The scanner requires `NET_RAW`/`NET_ADMIN` (ARP pings). Running it exposes raw sockets;
  keep it off untrusted networks.
- Stream tokens are minted by the API for the *viewing* user — token minting is not
  rate-limited per user beyond authentication. Legit users can start many short FFmpeg
  sessions; the relay caps global sessions (`STREAM_MAX_SESSIONS`) and reclaims idle ones.

## Dependency & Tooling Notes

- CI (`.github/workflows/ci.yml`) runs `go vet`, `go test -race`, nvr-service pytest,
  a frontend production build, and a docker-compose end-to-end smoke test of the
  auth → token → relay flow.
- When bumping dependencies, **never downgrade** pinned crypto versions (Fernet,
  cryptography, golang-jwt, bcrypt).