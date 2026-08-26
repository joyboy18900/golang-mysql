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

type ListByActorParams struct {
	ActorID int64
	Page    int
	Limit   int
}

type ListByActorResult struct {
	Items      []AuditLog
	TotalItems int64
}

//go:generate go tool mockgen -destination=../mock/mock_repository/auditlog.go golang-mysql/repository AuditLogRepository
type AuditLogRepository interface {
	Create(ctx context.Context, entry AuditLog) (*AuditLog, error)
	ListByActor(ctx context.Context, params ListByActorParams) (ListByActorResult, error)
}
