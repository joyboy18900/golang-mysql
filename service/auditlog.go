package service

import (
	"context"
	"time"
)

type CreateAuditLogRequest struct {
	ActorID    int64          `json:"actor_id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   *int64         `json:"entity_id"`
	Metadata   map[string]any `json:"metadata"`
}

type AuditLogResponse struct {
	ID         int64          `json:"id"`
	ActorID    int64          `json:"actor_id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   *int64         `json:"entity_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ListAuditLogRequest struct {
	ActorID int64
	Page    int
	Limit   int
}

type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

type ListAuditLogResponse struct {
	Data       []AuditLogResponse `json:"data"`
	Pagination Pagination         `json:"pagination"`
}

//go:generate go tool mockgen -destination=../mock/mock_service/auditlog.go golang-mysql/service AuditLogService
type AuditLogService interface {
	Create(ctx context.Context, req CreateAuditLogRequest) (*AuditLogResponse, error)
	ListByActor(ctx context.Context, req ListAuditLogRequest) (*ListAuditLogResponse, error)
}
