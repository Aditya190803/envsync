# ENV Sync

ENV Sync is a CLI for syncing environment variables across devices and teams on a project basis.

This repository includes a working Go MVP with encrypted secrets, project-based sync, team collaboration, and restore support.

## Features

- Local encryption using `AES-256-GCM`
- Key derivation from recovery phrase using `Argon2id`
- Project + environment scoping (`dev` default)
- Versioned secrets with history + rollback
- Masked listing by default
- Explicit `push` / `pull`
- Conflict detection with override flags
- Optimistic concurrency on remote writes (`revision`)
- Restore flow for second-machine onboarding (`envsync restore`)
- Structured JSON audit logging
- Optional HTTP remote backend (`envsync-server`)
- Cloud mode onboarding via `envsync login`

## Install

### From GitHub Releases (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh | bash
```

Install both binaries (`envsync` and `envsync-server`):

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh | bash -s -- --with-server --yes
```

Pin to a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh | bash -s -- --version v0.1.0 --yes
```

Installer defaults:

- Install dir: `~/.local/bin`
- Version: latest release
- OS/arch: auto-detected (`linux`/`darwin`, `amd64`/`arm64`)
- Post-install next steps: `envsync init` then `envsync login`

Optional installer environment variables:

- `ENVSYNC_INSTALL_REPO`
- `ENVSYNC_INSTALL_VERSION`
- `ENVSYNC_INSTALL_DIR`
- `ENVSYNC_INSTALL_BASE_URL` (artifact endpoint override, useful in CI smoke tests)
- `ENVSYNC_INSTALL_CHECKSUMS_URL` (custom checksums manifest URL)
- `ENVSYNC_INSTALL_SKIP_VERIFY` (`true` to skip checksum verification; not recommended)

Installer integrity behavior:

- Verifies downloaded release assets against `checksums.txt` by default
- Supports `sha256sum` or `shasum -a 256`
- Exits on checksum mismatch

Default installer repo: `Aditya190803/envsync`.

### Build from source

```bash
go build -ldflags "-X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" -o envsync ./cmd/envsync
go build -o envsync-server ./cmd/envsync-server
```

## Quickstart

```bash
# initialize vault (prints recovery phrase once)
envsync init

# sign in for cloud sync
envsync login

# create/select project
envsync project create api
envsync project use api

# manage secrets
envsync set DATABASE_URL postgres://localhost:5432/app
envsync list
envsync get DATABASE_URL

# sync
envsync push
envsync pull

# shell exports
# eval "$(envsync load)"
envsync load
```

For non-interactive usage:

```bash
export ENVSYNC_RECOVERY_PHRASE="<your recovery phrase>"
```

Optional actor identity for team RBAC checks:

```bash
export ENVSYNC_ACTOR="<user-or-service-id>"
```

## Commands

```text
envsync init
envsync login
envsync logout
envsync whoami
envsync doctor
envsync doctor --json
envsync restore

envsync project create <name>
envsync project list
envsync project use <name>
envsync project delete <name>
envsync team create <name>
envsync team list
envsync team use <name>
envsync team add-member <team> <actor> <role>
envsync team remove-member <team> <actor>
envsync team list-members [team]

envsync env create <name>
envsync env use <name>
envsync env list

envsync set <KEY> <value> [--expires-at <RFC3339|duration>]
envsync rotate <KEY> <value>
envsync get <KEY>
envsync delete <KEY>
envsync list [--show]
envsync load
envsync import <file>
envsync export <file>
envsync history <KEY>
envsync rollback <KEY> --version <n>
envsync diff

envsync push [--force]
envsync pull [--force-remote]
envsync phrase save
envsync phrase clear
```

Remote mode selection:

```bash
# choose backend explicitly: cloud|file|http
export ENVSYNC_REMOTE_MODE=cloud
```

Mode defaults:

- `http` when `ENVSYNC_REMOTE_URL` is set (self-host compatibility)
- `cloud` when `ENVSYNC_CLOUD_URL` is set and a cloud session exists
- `file` otherwise

## Storage model

Local state:

- `~/.config/envsync/state.json`

Audit log:

- `~/.config/envsync/audit.log`
- rotation/retention controls:
  - `ENVSYNC_AUDIT_MAX_BYTES` (default `1048576`)
  - `ENVSYNC_AUDIT_MAX_FILES` (default `5`)
  - `ENVSYNC_AUDIT_ROTATE_INTERVAL` (default `24h`)
  - `ENVSYNC_AUDIT_RETENTION_DAYS` (default `30`)
- permission auto-fix toggle:
  - `ENVSYNC_FIX_PERMISSIONS=true`

Default file remote:

- `~/.config/envsync/remote_store.json`

Override file remote path:

```bash
export ENVSYNC_REMOTE_FILE=/path/to/shared/remote_store.json
```

The remote store contains encrypted secret versions and metadata (including remote `revision` and restore metadata).

## Sync and conflicts

- `push` fails on key conflicts unless `--force`
- `pull` fails on key conflicts unless `--force-remote`
- remote writes are guarded by optimistic concurrency (`revision`); concurrent writes are rejected

## Team RBAC (baseline)

- `team create` creates a team and makes current actor `admin`
- `project create` under an active team attaches that project to the team
- roles: `admin`, `maintainer`, `reader` (`writer` accepted as alias for compatibility)
- maintainer/admin required for mutating actions (`set`, `rotate`, `delete`, `rollback`, `push`, `env create`)
- reader+ required for read actions (`get`, `list`, `load`, `history`, `pull`, `project use`)

## Rotation and keychain baseline

- Rotate a secret in-place with a new encrypted version:

```bash
envsync rotate API_KEY new-secret-value
```

- Save validated recovery phrase into OS keychain (macOS Keychain or Linux Secret Service):

```bash
envsync phrase save
```

- Clear phrase from keychain:

```bash
envsync phrase clear
```

`get/set/load/etc.` will use `ENVSYNC_RECOVERY_PHRASE`, then keychain, then prompt.

## Restore on a new machine

`restore` bootstraps local state from remote metadata + encrypted projects.

```bash
export ENVSYNC_RECOVERY_PHRASE="<your recovery phrase>"
envsync restore
```

Requirements:

- A prior successful `push` from an initialized device
- Same recovery phrase used on the source device

## Doctor diagnostics

Run:

```bash
envsync doctor
```

It checks:

- config/state availability
- active project/environment
- remote mode/target reachability
- recovery phrase env presence

## HTTP remote mode

Run the bundled server:

```bash
ENVSYNC_SERVER_ADDR=:8080 envsync-server
```

Client setup:

```bash
export ENVSYNC_REMOTE_URL=http://127.0.0.1:8080
```

Client retry/backoff tuning (for transient remote failures):

```bash
# total attempts, including the first request (default: 3)
export ENVSYNC_REMOTE_RETRY_MAX_ATTEMPTS=3

# base delay used for exponential backoff (default: 200ms)
export ENVSYNC_REMOTE_RETRY_BASE_DELAY=200ms

# cap for backoff delay before jitter (default: 2s)
export ENVSYNC_REMOTE_RETRY_MAX_DELAY=2s
```

Optional bearer token auth:

```bash
# server
export ENVSYNC_SERVER_TOKEN=secret-token

# client
export ENVSYNC_REMOTE_TOKEN=secret-token
```

Optional SSO proxy header auth baseline:

```bash
# server
export ENVSYNC_SERVER_AUTH_MODE=header
export ENVSYNC_SERVER_AUTH_HEADER=X-Auth-Request-User
export ENVSYNC_SERVER_AUTH_PROXY_SECRET=proxy-shared-secret

# proxy injects:
# X-Auth-Request-User: <authenticated-user>
# X-Envsync-Proxy-Secret: proxy-shared-secret
```

Server hardening env vars:

```bash
# requests per minute limit for /v1/store (0 disables)
export ENVSYNC_SERVER_RATE_LIMIT_RPM=240

# token bucket burst capacity
export ENVSYNC_SERVER_RATE_LIMIT_BURST=40
```

Operational endpoints:

- `GET /healthz`
- `GET /metrics` (Prometheus-style counters)

Each response includes `X-Request-Id` for traceability.

## EnvSync Cloud API (Render deploy)

This repo now includes a managed-cloud API service in [`cmd/envsync-cloud/main.go`](./cmd/envsync-cloud/main.go) with:

- `GET /healthz`
- `GET /v1/me` (bearer auth required)
- `GET /v1/store?project=<name>`
- `PUT /v1/store?project=<name>` with `If-Match` optimistic concurrency
- `POST /v1/tokens` (create PAT; returns raw token once)
- `DELETE /v1/tokens/:id` (revoke PAT)

Optional vault ownership routing:
- `organization_id=<uuid>` for org-scoped vaults
- `team_id=<uuid>` for team-scoped vaults
- `organization_id` and `team_id` are mutually exclusive

OpenAPI v1 contract is published at [`cmd/envsync-cloud/openapi.v1.yaml`](./cmd/envsync-cloud/openapi.v1.yaml).

### Local run

```bash
export ENVSYNC_CLOUD_INMEMORY=true
export ENVSYNC_CLOUD_DEV_TOKEN=dev-token
go run ./cmd/envsync-cloud
```

Point CLI to cloud:

```bash
export ENVSYNC_CLOUD_URL=http://127.0.0.1:8081
export ENVSYNC_REMOTE_MODE=cloud
export ENVSYNC_CLOUD_ACCESS_TOKEN=dev-token
envsync login
```

Issue a PAT (once DB mode is enabled):

```bash
curl -sS -X POST "$ENVSYNC_CLOUD_URL/v1/tokens" \
  -H "Authorization: Bearer $ENVSYNC_CLOUD_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"scopes":["profile:read","store:read","store:write"],"expires_at":"2026-12-31T23:59:59Z"}'
```

### Render deploy

Use the included [`render.yaml`](./render.yaml), then set:

- `ENVSYNC_CLOUD_PAT_PEPPER` (required for PAT validation)
- `ENVSYNC_CLOUD_RATE_LIMIT_RPM` (default `240`)
- `ENVSYNC_CLOUD_RATE_LIMIT_BURST` (default `40`)
- `ENVSYNC_CLOUD_MAX_BODY_BYTES` (default `1048576`)
- `ENVSYNC_CLOUD_JWT_ISSUER`
- `ENVSYNC_CLOUD_JWT_AUDIENCE` (or set `ENVSYNC_CLOUD_JWT_SKIP_AUD_CHECK=true` for bring-up only)

Optional bootstrap token:

- `ENVSYNC_CLOUD_DEV_TOKEN` (remove after OIDC is configured)

Operational runbooks:

- Cloud dashboards/alerts: [`docs/cloud-operations.md`](./docs/cloud-operations.md)
- Backup/restore + credential rotation: [`docs/cloud-backup-restore-runbook.md`](./docs/cloud-backup-restore-runbook.md)

## Legacy self-host compatibility (advanced)

Self-host HTTP remotes remain supported during migration using:

```bash
export ENVSYNC_REMOTE_MODE=http
export ENVSYNC_REMOTE_URL=https://<self-host-endpoint>
export ENVSYNC_REMOTE_TOKEN=<auth-token>
```

Default onboarding remains cloud-first with `envsync login`.

### Worker cutover timeline

- Cloud API GA date: **2026-03-15**
- Legacy Worker deprecation date: **2026-04-30**
- One full release-cycle notice is provided before final removal.

## Release asset naming for installer

The installer checks common naming patterns for each binary. Recommended release assets:

- `envsync_<version>_<os>_<arch>.tar.gz`
- `envsync-server_<version>_<os>_<arch>.tar.gz`

Examples:

- `envsync_0.1.0_linux_amd64.tar.gz`
- `envsync-server_0.1.0_darwin_arm64.tar.gz`

Each archive should contain the binary at archive root.

## CI

GitHub Actions workflow: [`./.github/workflows/ci.yml`](./.github/workflows/ci.yml)

Includes:

- `go test ./...`
- `go build ./...`
- token-protected remote endpoint check (`ENVSYNC_CI_REMOTE_TOKEN` secret, fallback `ci-token`)
- local artifact-based release smoke test (`init`, `push`, `pull`, `restore`)
- release workflow for signed artifacts and published checksums/signatures

CI-first usage examples: [`docs/ci-noninteractive-examples.md`](./docs/ci-noninteractive-examples.md)

## Security docs

- v1.0 scope freeze: [`docs/v1.0-scope-freeze.md`](./docs/v1.0-scope-freeze.md)
- Threat model: [`docs/threat-model.md`](./docs/threat-model.md)
- Security architecture: [`docs/security-architecture.md`](./docs/security-architecture.md)
- Team vault model: [`docs/team-vault-model.md`](./docs/team-vault-model.md)
- SSO/auth baseline: [`docs/sso-auth-baseline.md`](./docs/sso-auth-baseline.md)
- Team/RBAC/SSO guide: [`docs/team-rbac-sso.md`](./docs/team-rbac-sso.md)
- Retention and backup policy: [`docs/retention-backup-policy.md`](./docs/retention-backup-policy.md)
- Rotation runbook: [`docs/rotation-runbook.md`](./docs/rotation-runbook.md)
- Recovery phrase lifecycle policy: [`docs/recovery-phrase-lifecycle-policy.md`](./docs/recovery-phrase-lifecycle-policy.md)
- Security sign-off: [`docs/security-signoff-v1.0.md`](./docs/security-signoff-v1.0.md)
- Migration and upgrade guide: [`docs/migration-upgrade.md`](./docs/migration-upgrade.md)
- Responsible disclosure process: [`SECURITY.md`](./SECURITY.md)
- GA command reference: [`docs/commands-ga.md`](./docs/commands-ga.md)
- CLI UX review: [`docs/cli-ux-review.md`](./docs/cli-ux-review.md)
- Incident and recovery runbook: [`docs/incident-recovery-runbook.md`](./docs/incident-recovery-runbook.md)
- Versioning policy: [`docs/release-versioning-policy.md`](./docs/release-versioning-policy.md)
- Release rollback plan: [`docs/release-rollback-plan.md`](./docs/release-rollback-plan.md)
- Alerting operations: [`docs/alerting-operations.md`](./docs/alerting-operations.md)
- On-call ownership and escalation: [`docs/on-call-escalation.md`](./docs/on-call-escalation.md)
- v1.0 release evidence: [`docs/v1.0-release-evidence.md`](./docs/v1.0-release-evidence.md)
- v1.0 RC user validation: [`docs/v1.0-rc-user-validation.md`](./docs/v1.0-rc-user-validation.md)

## Security notes

- Secrets are encrypted before writing local/remote stores
- Recovery phrase is not stored in plaintext
- Losing the recovery phrase means data is unrecoverable
- Recovery phrase can be loaded from OS keychain with `envsync phrase save`

## Current MVP limitations

- Full enterprise SSO federation/SCIM lifecycle is not yet implemented
- Team RBAC identity is actor-ID/proxy-header based, not directory-native
- Advanced distributed abuse protections (WAF/global controls) are not yet included

## Development

```bash
go test ./...
go build ./...
```
