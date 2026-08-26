package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang-mysql/repository"
)

type benchService struct {
	repo repository.AuditLogRepository
}

func NewBenchService(repo repository.AuditLogRepository) QueryBenchmark {
	return benchService{repo: repo}
}

func (s benchService) ListByActorPlan(ctx context.Context, actorID int64, limit int) (*QueryPlanResult, error) {
	rawJSON, err := s.repo.ExplainListByActor(ctx, actorID, limit)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	if _, err := s.repo.ListByActor(ctx, actorID, limit); err != nil {
		return nil, err
	}
	elapsed := time.Since(start)

	return ParsePlan(rawJSON, elapsed)
}

type accessNode struct {
	AccessType string
	TableName  string
	KeyName    string
	Partitions []string
}

func ParsePlan(rawJSON string, elapsed time.Duration) (*QueryPlanResult, error) {
	var explain map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &explain); err != nil {
		return nil, fmt.Errorf("parse explain output: %w", err)
	}

	access, found := findAccessNode(explain)
	if !found {
		return nil, fmt.Errorf("parse explain output: no access node found")
	}

	summary := fmt.Sprintf("%s, Execution Time: %.2f ms",
		describeAccessNode(access), float64(elapsed.Microseconds())/1000)

	return &QueryPlanResult{Summary: summary, RawPlan: rawJSON}, nil
}

// findAccessNode walks MySQL's EXPLAIN FORMAT=JSON tree looking for the node
// that describes how a table is actually accessed (the "table_name"/
// "access_type" object). MySQL nests that object under different wrapper
// keys depending on the query shape - "ordering_operation" when a sort is
// involved, "grouping_operation" for GROUP BY, "nested_loop" (an array) for
// joins - so this recurses through the whole tree rather than assuming a
// fixed path.
func findAccessNode(node any) (accessNode, bool) {
	switch v := node.(type) {
	case map[string]any:
		if accessType, ok := v["access_type"].(string); ok {
			access := accessNode{AccessType: accessType}
			if tableName, ok := v["table_name"].(string); ok {
				access.TableName = tableName
			}
			if key, ok := v["key"].(string); ok {
				access.KeyName = key
			}
			if partitions, ok := v["partitions"].([]any); ok {
				for _, p := range partitions {
					if name, ok := p.(string); ok {
						access.Partitions = append(access.Partitions, name)
					}
				}
			}
			return access, true
		}
		for _, child := range v {
			if found, ok := findAccessNode(child); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range v {
			if found, ok := findAccessNode(child); ok {
				return found, true
			}
		}
	}
	return accessNode{}, false
}

func describeAccessNode(a accessNode) string {
	switch {
	case a.KeyName != "":
		return fmt.Sprintf("%s using %s on %s", a.AccessType, a.KeyName, a.TableName)
	case a.TableName != "":
		return fmt.Sprintf("%s on %s", a.AccessType, a.TableName)
	default:
		return a.AccessType
	}
}
