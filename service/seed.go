package service

import "context"

type Seeder interface {
	Seed(ctx context.Context, rowCount int) (int64, error)
}
