package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"golang-mysql/logs"
	"golang-mysql/repository"
)

var (
	seedActions     = []string{"create", "update", "delete", "login", "logout"}
	seedEntityTypes = []string{"user", "order", "invoice", "session"}
	seedWindowStart = time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	seedWindowEnd   = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	emptyMetadata   = map[string]any{}
)

type seedService struct {
	repo             repository.AuditLogRepository
	actorCardinality int
	batchSize        int
}

func NewSeedService(repo repository.AuditLogRepository, actorCardinality int, batchSize int) Seeder {
	return seedService{repo: repo, actorCardinality: actorCardinality, batchSize: batchSize}
}

func (s seedService) Seed(ctx context.Context, rowCount int) (int64, error) {
	var inserted int64
	scratch := make([]repository.AuditLog, s.batchSize)

	for remaining := rowCount; remaining > 0; {
		batchSize := s.batchSize
		if remaining < batchSize {
			batchSize = remaining
		}

		batch := scratch[:batchSize]
		for i := range batch {
			batch[i] = randomAuditLog(s.actorCardinality)
		}

		n, err := s.repo.BatchInsert(ctx, batch)
		if err != nil {
			return inserted, err
		}
		inserted += n
		remaining -= batchSize

		logs.Info(fmt.Sprintf("seeded %d/%d rows", inserted, rowCount))
	}

	if err := s.repo.Analyze(ctx); err != nil {
		return inserted, err
	}

	return inserted, nil
}

func randomAuditLog(actorCardinality int) repository.AuditLog {
	return repository.AuditLog{
		ActorID:    int64(rand.Intn(actorCardinality)) + 1,
		Action:     seedActions[rand.Intn(len(seedActions))],
		EntityType: seedEntityTypes[rand.Intn(len(seedEntityTypes))],
		Metadata:   emptyMetadata,
		CreatedAt:  randomTimeBetween(seedWindowStart, seedWindowEnd),
	}
}

func randomTimeBetween(start, end time.Time) time.Time {
	delta := end.Sub(start)
	offset := time.Duration(rand.Int63n(int64(delta)))
	return start.Add(offset)
}
