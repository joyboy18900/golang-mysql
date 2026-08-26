package service

import (
	"context"
	"testing"

	"golang-mysql/mock/mock_repository"
	"golang-mysql/repository"

	"go.uber.org/mock/gomock"
)

func TestAuditLogService_ListByActor_TotalPages(t *testing.T) {
	tests := []struct {
		name       string
		totalItems int64
		limit      int
		wantPages  int
	}{
		{name: "zero items", totalItems: 0, limit: 25, wantPages: 0},
		{name: "exact multiple", totalItems: 100, limit: 25, wantPages: 4},
		{name: "remainder rounds up", totalItems: 101, limit: 25, wantPages: 5},
		{name: "single item", totalItems: 1, limit: 50, wantPages: 1},
		{name: "limit larger than item count", totalItems: 5, limit: 50, wantPages: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mock_repository.NewMockAuditLogRepository(ctrl)
			repo.EXPECT().ListByActor(gomock.Any(), gomock.Any()).Return(
				repository.ListByActorResult{Items: []repository.AuditLog{}, TotalItems: tt.totalItems}, nil,
			)

			svc := NewAuditLogService(repo)
			resp, err := svc.ListByActor(context.Background(), ListAuditLogRequest{ActorID: 1, Page: 1, Limit: tt.limit})
			if err != nil {
				t.Fatalf("ListByActor() error = %v", err)
			}
			if resp.Pagination.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", resp.Pagination.TotalPages, tt.wantPages)
			}
		})
	}
}

func TestAuditLogService_ListByActor_PageValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock_repository.NewMockAuditLogRepository(ctrl)

	svc := NewAuditLogService(repo)
	_, err := svc.ListByActor(context.Background(), ListAuditLogRequest{ActorID: 1, Page: -1, Limit: 10})
	if err == nil {
		t.Fatalf("ListByActor() error = nil, want validation error for negative page")
	}
}
