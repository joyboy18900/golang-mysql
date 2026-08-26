# golang-mysql

Fiber + GORM + MySQL reference with offset (page-number) pagination. One
entity, `audit_log`, carries the whole story: create an entry, list an
actor's activity history a page at a time.

## Run

```bash
docker compose up -d --build
curl "http://localhost:8080/audit-log?actor_id=42"
```

Runs the migrations automatically on startup, then starts the API on
`:8080`. See `curl/flow.md` for requests.

## Endpoints

- `POST /audit-log`
- `GET /audit-log?actor_id=X` (`page`, `limit` optional)

See `curl/flow.md` for full request/response examples.

## Schema (`migrations/`)

1. **`0001_create_audit_log_table`** - `audit_log` (`actor_id`, `action`,
   `entity_type`, `entity_id`, `metadata json`, `created_at datetime(6)`).
   The table sets `DEFAULT CURRENT_TIMESTAMP(6)` on `created_at` as a
   safety net for rows inserted outside the app; the running app relies on
   GORM's `autoCreateTime` convention instead, so this default is normally
   dead but deliberate schema.
2. **`0002_add_actor_id_index`** - composite index on `(actor_id,
   created_at DESC)`. It does not include `id`, so exact `created_at`
   ties get a small in-memory sort by the database - correctness is
   unaffected, the query's `id DESC` tiebreak still produces a strict
   order.

## Tests

```bash
go test ./...
go generate ./...   # regenerate repository and service mocks
```

- `service/auditlog_service_test.go` - the total-pages ceiling-division
  math (zero items, exact multiple, remainder, single item, limit larger
  than item count).
- `audit_log_integration_test.go` - migrations round-trip; a full offset
  walk across every page (with a block of rows sharing one `created_at` to
  exercise the `id` tiebreak) with no gaps or duplicates, a same-page
  double-request check that the tied rows come back in the same order both
  times, and a past-the-last-page request that returns `200` with an empty
  `data` array instead of an error; both endpoints return the right
  envelope/errors.

The integration test owns its own database - run it against a scratch
MySQL, not a database with data you care about.

See `curl/flow.md` for a manual walkthrough of every endpoint.
