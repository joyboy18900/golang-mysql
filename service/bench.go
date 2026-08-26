package service

import "context"

type QueryPlanResult struct {
	Summary string
	RawPlan string
}

type QueryBenchmark interface {
	ListByActorPlan(ctx context.Context, actorID int64, limit int) (*QueryPlanResult, error)
}
