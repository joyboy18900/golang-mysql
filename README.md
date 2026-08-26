# golang-mysql

Fiber + GORM + MySQL reference with keyset (cursor) pagination. One entity,
`audit_log`, carries the whole story: create an entry, list an actor's
activity history a page at a time.

## Run

```bash
docker compose up -d --build
curl "http://localhost:8080/audit-log?actor_id=42"
```

Runs the migrations automatically on startup, then starts the API on
`:8080`. See `curl/flow.md` for requests.

## Endpoints

- `POST /audit-log`
- `GET /audit-log?actor_id=X&limit=Y&cursor=Z`

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
   unaffected, the cursor's `id DESC` tiebreak still produces a strict
   order.

## Pagination

The cursor is an opaque, base64-encoded `"<created_at_unix_micro>:<id>"`.
Clients never construct or parse it - it comes back as `next_cursor` in
the response and gets passed back unchanged on the next request.

```json
{ "items": [ ... ], "next_cursor": "MTc4Nzc1OTM3OTgxMDAwMDoy" }
```

`next_cursor` is `null` on the last page. The service fetches `limit + 1`
rows to decide this: getting back `limit + 1` means there is a next page,
so the extra row is trimmed and its key becomes the next cursor; getting
back `limit` or fewer means there is no next page.

## Tests

```bash
go test ./...
go generate ./...   # regenerate repository and service mocks
```

- `service/cursor_test.go` - cursor encode/decode round trip, malformed
  and garbage-but-valid-base64 rejection.
- `audit_log_integration_test.go` - migrations round-trip; a multi-page
  cursor walk (with a block of rows sharing one `created_at` to exercise
  the `id` tiebreak) that terminates on `next_cursor: null` with no gaps
  or duplicates; both endpoints return the right envelope/errors.

The integration test owns its own database - run it against a scratch
MySQL, not a database with data you care about.

See `curl/flow.md` for a manual walkthrough of every endpoint.
