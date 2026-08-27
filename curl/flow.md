# Manual test flow

Walkthrough for exercising the audit log API by hand. `docker compose up`
migrates and starts the server.

## Start

```bash
docker compose up -d --build
docker compose ps
docker compose logs app --tail 20   # should show "server started on port 8080"
```

## 1. Create audit log entries

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42,"action":"login","entity_type":"session"}'
```

```json
{ "code": 201, "message": "audit log entry created", "data": { "id": 1, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T17:10:20.441Z" } }
```

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42,"action":"update","entity_type":"order","metadata":{"foo":"bar"}}'
```

```json
{ "code": 201, "message": "audit log entry created", "data": { "id": 2, "actor_id": 42, "action": "update", "entity_type": "order", "entity_id": null, "metadata": {"foo":"bar"}, "created_at": "2026-08-26T17:10:25.973Z" } }
```

## 2. List an actor's activity history

```bash
curl "http://localhost:8080/audit-log?actor_id=42"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": { "data": [ { "id": 2, "actor_id": 42, "action": "update", "entity_type": "order", "entity_id": null, "metadata": {"foo":"bar"}, "created_at": "2026-08-26T17:10:25.973Z" }, { "id": 1, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T17:10:20.441Z" } ], "pagination": { "page": 1, "limit": 50, "total_items": 2, "total_pages": 1 } } }
```

`page` and `limit` are optional. This call defaults to `page=1` and
`limit=50`, and the response already includes `pagination.total_pages`.

## 3. Walk pages, one at a time

```bash
curl "http://localhost:8080/audit-log?actor_id=42&page=1&limit=1"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": { "data": [ { "id": 2, "actor_id": 42, "action": "update", "entity_type": "order", "entity_id": null, "metadata": {"foo":"bar"}, "created_at": "2026-08-26T17:10:25.973Z" } ], "pagination": { "page": 1, "limit": 1, "total_items": 2, "total_pages": 2 } } }
```

`page` defaults to `1`, `limit` defaults to `50`. Pass `page=2` to get the
next page:

```bash
curl "http://localhost:8080/audit-log?actor_id=42&page=2&limit=1"
```

```json
{ "code": 200, "message": "audit log entries retrieved", "data": { "data": [ { "id": 1, "actor_id": 42, "action": "login", "entity_type": "session", "entity_id": null, "metadata": {}, "created_at": "2026-08-26T17:10:20.441Z" } ], "pagination": { "page": 2, "limit": 1, "total_items": 2, "total_pages": 2 } } }
```

`pagination.total_pages` tells the client when to stop. A `page` past the
last one still returns `200` with an empty `data` array, not an error.

## 4. Rejection cases

Missing `actor_id`:

```bash
curl "http://localhost:8080/audit-log"
```

```json
{ "code": 422, "message": "actor_id query parameter is required", "data": null }
```

Missing required fields on create:

```bash
curl -X POST http://localhost:8080/audit-log \
  -H "Content-Type: application/json" \
  -d '{"actor_id":42}'
```

```json
{ "code": 422, "message": "actor_id, action and entity_type are required", "data": null }
```

`page` must be a positive integer:

```bash
curl "http://localhost:8080/audit-log?actor_id=42&page=0"
```

```json
{ "code": 422, "message": "page must be a positive integer", "data": null }
```

## Stop

```bash
docker compose down -v
```
