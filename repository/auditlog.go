package repository

import (
	"context"
	"time"
)

type AuditLog struct {
	ID         int64
	ActorID    int64
	Action     string
	EntityType string
	EntityID   *int64
	Metadata   map[string]any
	CreatedAt  time.Time
}

//go:generate go tool mockgen -destination=../mock/mock_repository/auditlog.go golang-mysql/repository AuditLogRepository
type AuditLogRepository interface {
	Create(ctx context.Context, entry AuditLog) (*AuditLog, error)
	ListByActor(ctx context.Context, actorID int64, limit int) ([]AuditLog, error)
}
