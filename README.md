# Rikami API

⚠️ **Under active refactor.** `rikami-api` is being built out alongside the rest of the Rikami toolkit. Right now only a small slice of the API is live; everything that isn't wired up yet is clearly marked with a 🚧 **WIP** tag.

**Rikami API** (`rikami-api`) is the control-plane service of the [Rikami](https://github.com/b-zago) toolkit. It's a small Go HTTP service that authenticates operators, hands out the cluster's sealed-secrets certificate, and 🚧 will drive the lifecycle of GitOps-managed applications — generating, updating, sleeping, waking, and killing the charts that `rika manifest` produces.

Where the `rika` CLI runs locally (or in CI) to turn a `rikami.yaml` into a Helm chart, `rikami-api` is the always-on server that backs the CLI's networked commands (`rika login`, `rika seal`, and the 🚧 `rika summon` deploy flow).

## How it fits

`rikami-api` sits between operators and the cluster, backed by Postgres (users/roles) and Redis (sessions):

- **`rika` CLI** signs each request and logs in to obtain a short-lived token.
- **`rikami-api`** verifies the signature, checks the caller's role permissions, and performs the requested action.
- **GitOps** (ArgoCD or similar) reconciles whatever `rikami-api` writes to the k8s repo. 🚧

## Concepts

### Users, roles & permissions

Operators are stored in Postgres as **users**, each assigned a **role**. A role is just a set of boolean permissions — one per action the API can perform:

- `summon` — trigger chart generation / deploy
- `regen` — regenerate an existing chart
- `update` — bump the rikami-charts library version a chart depends on
- `sleep` — "turn off" an application
- `awake` — bring a slept application back
- `kill` — delete a chart
- `users` — manage other users (register / edit roles)

The [initial migration](migrations/001_init.sql) seeds three default roles:

- **`admin`** — every permission.
- **`moderator`** — everything except `users`.
- **`maintainer`** — `regen`, `update`, `sleep`, `awake` only (no `summon`, `kill`, or `users`).

On startup `rikami-api` registers a default **admin** account (from `ADMIN_*` env vars) if no admin exists yet.

### Request signing (HMAC)

Every authenticated request is signed. The client computes `HMAC-SHA256(body, hmac_token)` (base64, raw) and sends it in the `x-rikami-signature` header; `rikami-api` recomputes it with the user's stored HMAC token and rejects anything that doesn't match. This is the same `HMAC` shared secret the `rika` CLI keeps in its config.

### Token auth flow

Passwords are hashed with **Argon2id** (`salt$hash`, base64). On a successful `POST /login` `rikami-api` mints a **token pair** and stores it in Redis:

- **short token** — used on every request, 10-minute TTL (`x-rikami-token`).
- **refresh token** — used to mint a new short token, 6-hour TTL (`x-rikami-ref-token`).

Every request must carry an `x-rikami-signature`. The auth middleware then resolves it in order:

1. **`x-rikami-token` present** — if the short token is valid, verify the signature, check the permission, and run the handler. If it's expired, respond `{ "needs_refresh": true }`.
2. **Otherwise `x-rikami-ref-token` present** — if the refresh token is valid, mint a new short token (returned in the `x-rikami-token` response header), then verify and run as above. If it's missing or expired, return `401` and the client must log in again.
3. **Neither present** — `401`.

Redis keys: `user:<id>` (the live token pair), `token:short:<token>` → refresh token, `token:refresh:<token>` → cached `{permissions, hmac, user_id}`. Rotating in a new login invalidates the previous pair.

## API

Currently live:

- `GET /cert` — returns the cluster's [bitnami sealed-secrets](https://github.com/bitnami-labs/sealed-secrets) public certificate, used by `rika` to seal secrets before they're committed. 🚧 The cert is presently a baked-in constant.
- `POST /login` — body `{ "user", "password" }` plus an `x-rikami-signature` header. On success returns the short/refresh token pair as JSON.

Planned action endpoints (the auth middleware and permission model already exist; the handlers and routes do not yet):

- 🚧 `summon` — trigger generation/deploy of an app's chart (backs the `rika summon` stub).
- 🚧 `regen` — regenerate an existing chart.
- 🚧 `update` — update the rikami-charts library version a chart depends on.
- 🚧 `kill` — delete a chart.
- 🚧 `sleep` — "turn off" an application by prefixing its `values-<env>.yaml` files with `_`, so GitOps stops reconciling them.
- 🚧 `awake` — bring an application back by removing the `_` prefix.
- 🚧 user management — register and manage operators (gated by the `users` permission).

All requests are wrapped with logging, a per-handler timeout, and a uniform error layer that emits the canned bodies from [errors.json](errors.json) (a global 5s `TimeoutHandler` sits in front of the mux).

## Configuration

`rikami-api` is configured entirely through environment variables — all are **required**, and the process exits on startup if any is missing:

- **Postgres** — `POSTGRES_HOST` (port `5432` assumed), `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`.
- **Redis** — `REDIS_HOST` (port `6379` assumed).
- **Bootstrap admin** — `ADMIN_USER`, `ADMIN_PASSWORD`, `ADMIN_HMAC` (used to seed the default admin account).

The server listens on `:8080`.

## Data stores

- **Postgres** — `users` and `roles` tables. Schema lives in [migrations/](migrations/) and is managed with [Atlas](https://atlasgo.io) (see [atlas.hcl](atlas.hcl)). A pooled `pgx` connection (max 25 conns) is opened and ping-checked at startup.
- **Redis** — session / token store. Pooled client, ping-checked at startup.

## Install / run

Requires Go 1.26+.

### Local (docker-compose)

The quickest way to bring up `rikami-api` with its dependencies:

```sh
docker compose up --build --watch
```

This starts Postgres, Redis, and the API (live-rebuild via [Dockerfile.dev](Dockerfile.dev)) with sensible local defaults wired in [docker-compose.yml](docker-compose.yml). Apply migrations against the local DB with Atlas:

```sh
atlas migrate apply --env local
```

### Build

```sh
# binary
go build -o server .

# production image (static binary on debian:bookworm-slim)
docker build -t rikami-api .
```

## CI/CD

- **test-build** runs on every pull request to `main` / `staging`.
- **build-deploy** builds and pushes the `webserver` image to GHCR on pushes to `main` / `staging`.

Both delegate to reusable workflows in [`b-zago/actions`](https://github.com/b-zago/actions). `rikami-api` deploys _itself_ through Rikami — see its own [rikami.yaml](rikami.yaml).

## Related repositories

- [rikami](https://github.com/b-zago/rikami) — the `rika` CLI that generates charts and talks to `rikami-api`.
- [rikami-charts](https://github.com/b-zago/rikami-charts) — the Helm library generated charts depend on (`oci://ghcr.io/b-zago/rikami-charts`).
- [rikami-base](https://github.com/b-zago/rikami-base) — the shard / fragment / conf templates consumed by `rika manifest`.
- 🚧 **rikami-action** — GitHub Action wrapper around `rika manifest` for CI. Not yet built.

---

This README tracks an actively moving target and may lag slightly behind the code. As the action endpoints land, they'll lose their 🚧 tags.
