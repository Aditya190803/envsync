# envsync

`envsync` is a terminal-first CLI for managing environment variables with local encryption and explicit sync.

This repository includes a working Go MVP with local-first encrypted secrets, explicit sync, remote conflict handling, and restore support.

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
- Optional Convex cloud backup

## Install

### From GitHub Releases (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/scripts/install.sh | bash
```

Install both binaries (`envsync` and `envsync-server`):

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/scripts/install.sh | bash -s -- --with-server --yes
```

Pin to a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/scripts/install.sh | bash -s -- --version v0.1.0 --yes
```

Installer defaults:

- Install dir: `~/.local/bin`
- Version: latest release
- OS/arch: auto-detected (`linux`/`darwin`, `amd64`/`arm64`)
- If local `.env*` files are found (excluding `*example*`), installer prompts whether you want cloud-push setup steps

Optional installer environment variables:

- `ENVSYNC_INSTALL_REPO`
- `ENVSYNC_INSTALL_VERSION`
- `ENVSYNC_INSTALL_DIR`

Default installer repo: `Aditya190803/envsync`.

### Build from source

```bash
go build -o envsync ./cmd/envsync
go build -o envsync-server ./cmd/envsync-server
```

## Quickstart

```bash
# initialize vault (prints recovery phrase once)
envsync init

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
envsync doctor
envsync restore

envsync project create <name>
envsync project list
envsync project use <name>
envsync team create <name>
envsync team list
envsync team use <name>
envsync team add-member <team> <actor> <role>
envsync team list-members [team]

envsync env create <name>
envsync env use <name>

envsync set <KEY> <value>
envsync rotate <KEY> <value>
envsync get <KEY>
envsync delete <KEY>
envsync list [--show]
envsync load
envsync history <KEY>
envsync rollback <KEY> --version <n>

envsync push [--force]
envsync pull [--force-remote]
envsync phrase save
envsync phrase clear
```

## Storage model

Local state:

- `~/.config/envsync/state.json`

Audit log:

- `~/.config/envsync/audit.log`

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

## Convex cloud backup

This repo includes Convex functions in [`convex/backup.ts`](./convex/backup.ts) and schema in [`convex/schema.ts`](./convex/schema.ts).

Deploy from this repo:

```bash
npm install convex
npx convex dev
```

Client setup:

```bash
export ENVSYNC_CONVEX_URL=https://<your-deployment>.convex.cloud
```

Optional backup API key:

```bash
# set in Convex env
npx convex env set ENVSYNC_CONVEX_API_KEY supersecret

# set on client
export ENVSYNC_CONVEX_API_KEY=supersecret
```

Optional Convex deploy key + function path overrides:

```bash
export ENVSYNC_CONVEX_DEPLOY_KEY=<deploy-key>
export ENVSYNC_CONVEX_GET_PATH=backup:getStore
export ENVSYNC_CONVEX_PUT_PATH=backup:putStore
```

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

## Security docs

- v1.0 scope freeze: [`docs/v1.0-scope-freeze.md`](./docs/v1.0-scope-freeze.md)
- Threat model: [`docs/threat-model.md`](./docs/threat-model.md)
- Security architecture: [`docs/security-architecture.md`](./docs/security-architecture.md)
- Team vault model: [`docs/team-vault-model.md`](./docs/team-vault-model.md)
- SSO/auth baseline: [`docs/sso-auth-baseline.md`](./docs/sso-auth-baseline.md)
- Retention and backup policy: [`docs/retention-backup-policy.md`](./docs/retention-backup-policy.md)
- Rotation runbook: [`docs/rotation-runbook.md`](./docs/rotation-runbook.md)
- Recovery phrase lifecycle policy: [`docs/recovery-phrase-lifecycle-policy.md`](./docs/recovery-phrase-lifecycle-policy.md)
- Security sign-off: [`docs/security-signoff-v1.0.md`](./docs/security-signoff-v1.0.md)
- Migration and upgrade guide: [`docs/migration-upgrade.md`](./docs/migration-upgrade.md)
- Responsible disclosure process: [`SECURITY.md`](./SECURITY.md)

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
