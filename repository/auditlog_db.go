package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type auditLogRepositoryDB struct {
	db *sql.DB
}

func NewAuditLogRepositoryDB(db *sql.DB) AuditLogRepository {
	return auditLogRepositoryDB{db: db}
}

func (r auditLogRepositoryDB) Create(ctx context.Context, entry AuditLog) (*AuditLog, error) {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal audit log metadata: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, action, entity_type, entity_id, metadata)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.ActorID, entry.Action, entry.EntityType, entry.EntityID, string(metadata),
	)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT id, actor_id, action, entity_type, entity_id, metadata, created_at
		 FROM audit_log
		 WHERE id = ?`,
		id,
	)

	created, err := scanAuditLog(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}

	return created, nil
}

func (r auditLogRepositoryDB) ListByActor(ctx context.Context, actorID int64, limit int) ([]AuditLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, actor_id, action, entity_type, entity_id, metadata, created_at
		 FROM audit_log
		 WHERE actor_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		actorID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit log by actor: %w", err)
	}
	defer rows.Close()

	var entries []AuditLog
	for rows.Next() {
		entry, err := scanAuditLog(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit log rows: %w", err)
	}

	return entries, nil
}

func (r auditLogRepositoryDB) BatchInsert(ctx context.Context, entries []AuditLog) (int64, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(entries))
	args := make([]any, 0, len(entries)*6)
	for i, entry := range entries {
		metadata, err := json.Marshal(entry.Metadata)
		if err != nil {
			return 0, fmt.Errorf("marshal audit log metadata: %w", err)
		}
		placeholders[i] = "(?, ?, ?, ?, ?, ?)"
		args = append(args, entry.ActorID, entry.Action, entry.EntityType, entry.EntityID, string(metadata), entry.CreatedAt)
	}

	query := fmt.Sprintf(
		`INSERT INTO audit_log (actor_id, action, entity_type, entity_id, metadata, created_at) VALUES %s`,
		strings.Join(placeholders, ", "),
	)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("batch insert audit log: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("batch insert audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("batch insert audit log: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("batch insert audit log: %w", err)
	}

	return n, nil
}

func (r auditLogRepositoryDB) Analyze(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, "ANALYZE TABLE audit_log"); err != nil {
		return fmt.Errorf("analyze audit log: %w", err)
	}

	return nil
}

func (r auditLogRepositoryDB) ExplainListByActor(ctx context.Context, actorID int64, limit int) (string, error) {
	var plan string
	err := r.db.QueryRowContext(ctx,
		`EXPLAIN FORMAT=JSON
		 SELECT id, actor_id, action, entity_type, entity_id, metadata, created_at
		 FROM audit_log
		 WHERE actor_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		actorID, limit,
	).Scan(&plan)
	if err != nil {
		return "", fmt.Errorf("explain list audit log by actor: %w", err)
	}

	return plan, nil
}

func scanAuditLog(scan func(dest ...any) error) (*AuditLog, error) {
	var entry AuditLog
	var metadata []byte

	if err := scan(&entry.ID, &entry.ActorID, &entry.Action, &entry.EntityType,
		&entry.EntityID, &metadata, &entry.CreatedAt); err != nil {
		return nil, err
	}

	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal audit log metadata: %w", err)
		}
	}

	return &entry, nil
}
