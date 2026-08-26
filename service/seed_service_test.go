package service_test

import (
	"context"
	"errors"
	"testing"

	"golang-mysql/mock/mock_repository"
	"golang-mysql/service"

	"go.uber.org/mock/gomock"
)

func TestSeedService_Seed(t *testing.T) {
	tests := []struct {
		name       string
		rowCount   int
		batchSize  int
		wantBatch  []int
		wantErr    bool
		insertErr  error
		analyzeErr error
	}{
		{
			name:      "splits into full batches",
			rowCount:  25000,
			batchSize: 10000,
			wantBatch: []int{10000, 10000, 5000},
		},
		{
			name:      "row count smaller than batch size",
			rowCount:  100,
			batchSize: 10000,
			wantBatch: []int{100},
		},
		{
			name:      "exact multiple of batch size",
			rowCount:  20000,
			batchSize: 10000,
			wantBatch: []int{10000, 10000},
		},
		{
			name:      "batch insert failure stops seeding",
			rowCount:  10000,
			batchSize: 10000,
			wantBatch: []int{10000},
			insertErr: errors.New("insert failed"),
			wantErr:   true,
		},
		{
			name:       "analyze failure surfaces after all batches",
			rowCount:   10000,
			batchSize:  10000,
			wantBatch:  []int{10000},
			analyzeErr: errors.New("analyze failed"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mock_repository.NewMockAuditLogRepository(ctrl)

			for _, batch := range tt.wantBatch {
				call := repo.EXPECT().
					BatchInsert(gomock.Any(), gomock.Len(batch)).
					Return(int64(batch), tt.insertErr)
				if tt.insertErr != nil {
					call.Times(1)
					break
				}
			}
			if tt.insertErr == nil {
				repo.EXPECT().Analyze(gomock.Any()).Return(tt.analyzeErr)
			}

			seeder := service.NewSeedService(repo, 20000, tt.batchSize)
			inserted, err := seeder.Seed(context.Background(), tt.rowCount)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inserted != int64(tt.rowCount) {
				t.Fatalf("inserted = %d, want %d", inserted, tt.rowCount)
			}
		})
	}
}
