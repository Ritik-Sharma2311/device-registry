# device-registry

A REST service for registering and tracking devices (cameras, NVRs, sensors, gateways). One storage interface with three interchangeable backends (in-memory, PostgreSQL, MongoDB), JWT authentication with roles, optimistic concurrency control, cursor pagination, and change events published to NATS.

```
            ┌──────────── HTTP (net/http) ────────────┐
client ───> │ auth (JWT, roles) ─> handlers (api)     │
            └───────────────────┬─────────────────────┘
                                v
                       device.Service  ── validation, IDs, timestamps
                         │            └─> events (NATS)  after a successful write
                         v
                  device.Store (interface)
           ┌─────────────┼───────────────┐
        memory        postgres        mongodb
```

## Design points

- **One Store interface, three backends.** Services never see SQL or BSON. A single contract test suite (`internal/store/storetest`) runs against every backend, so they behave identically, including error values and concurrency.
- **Optimistic concurrency.** Each device has a `version`. `PUT` requires `If-Match: "<version>"`; if someone else changed the device first the answer is `412 Precondition Failed` instead of a silent overwrite. Both SQL and Mongo implement this as a single compare-and-swap statement, so it is atomic without locks. The contract suite fires 20 concurrent updates and asserts exactly one wins.
- **Auth.** `POST /auth/token` returns a 15 minute HS256 JWT carrying a role. `viewer` can read, `admin` can write. Only HS256 is accepted, expiry is mandatory, and credentials are compared in constant time.
- **Pagination.** Keyset (`after=<id>&limit=n`), not offset, so pages stay stable and cheap as data grows.
- **Events.** Created, updated and deleted devices are published to `devices.created`, `devices.updated`, `devices.deleted` as JSON after the write succeeds. Publishing is best effort: a broker outage is logged and does not fail the request.
- **Migrations.** Embedded SQL files applied once each in order, guarded by a Postgres advisory lock so concurrent starts do not race.

## API

| Method and path | Role | Notes |
|---|---|---|
| `POST /auth/token` | none | `{"username","password"}` returns `{"token"}` |
| `GET /devices` | viewer, admin | `kind`, `status`, `limit` (max 200), `after`; returns `items` and `next_after` |
| `GET /devices/{id}` | viewer, admin | `ETag` header holds the version |
| `POST /devices` | admin | `{"name","kind","address","status"}`, `201` with `Location` |
| `PUT /devices/{id}` | admin | needs `If-Match`; `428` missing, `412` stale |
| `DELETE /devices/{id}` | admin | `204` |
| `GET /healthz` | none | liveness |

Errors are JSON `{"error": "..."}`: `422` invalid input, `404` unknown id, `409` duplicate name, `412` version conflict, `401` or `403` for auth.

Kinds: `camera`, `nvr`, `sensor`, `gateway`. Statuses: `online`, `offline`, `unknown`.

## Run

```
docker compose up --build            # registry + Postgres + NATS (+ Mongo available)
```

or without Docker, with the in-memory store:

```
make run
```

Try it:

```
TOKEN=$(curl -s localhost:8080/auth/token -d '{"username":"admin","password":"admin-pass"}' | jq -r .token)
curl -s localhost:8080/devices -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"front-door","kind":"camera","address":"10.0.0.5:554"}'
curl -s localhost:8080/devices -H "Authorization: Bearer $TOKEN"
```

Configuration: flags `-addr`, `-store` (`memory`, `postgres`, `mongodb`), `-nats`. Environment: `REGISTRY_USERS` (`name:password:role,...`), `JWT_SECRET` (16+ chars), `POSTGRES_DSN`, `MONGO_URI`. Secrets are read from the environment, not flags, so they do not appear in the process list.

## Tests

```
make test       # vet + race detector; Postgres and MongoDB suites are skipped
make test-all   # with Postgres and MongoDB from docker compose
```

CI runs everything against real Postgres and MongoDB service containers. The NATS test starts an embedded server, so it needs nothing external.

## Limits

Users come from an environment variable with plaintext passwords, which is fine for a demo and not for production (use a user table with hashed passwords). Event publishing is at-most-once; guaranteeing delivery would need a transactional outbox or JetStream.
