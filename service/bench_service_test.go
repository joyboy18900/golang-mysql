package service

import (
	"testing"
	"time"
)

func TestParsePlan(t *testing.T) {
	tests := []struct {
		name        string
		rawJSON     string
		elapsed     time.Duration
		wantSummary string
		wantErr     bool
	}{
		{
			name: "table scan before index",
			rawJSON: `{
				"query_block": {
					"select_id": 1,
					"table": {
						"table_name": "audit_log",
						"access_type": "ALL",
						"rows_examined_per_scan": 1000000,
						"filtered": "10.00"
					}
				}
			}`,
			elapsed:     25330 * time.Microsecond,
			wantSummary: "ALL on audit_log, Execution Time: 25.33 ms",
		},
		{
			name: "ref access after index",
			rawJSON: `{
				"query_block": {
					"select_id": 1,
					"table": {
						"table_name": "audit_log",
						"access_type": "ref",
						"key": "idx_audit_log_actor_id_created_at",
						"rows_examined_per_scan": 80
					}
				}
			}`,
			elapsed:     430 * time.Microsecond,
			wantSummary: "ref using idx_audit_log_actor_id_created_at on audit_log, Execution Time: 0.43 ms",
		},
		{
			name: "index access on a pruned partition",
			rawJSON: `{
				"query_block": {
					"select_id": 1,
					"table": {
						"table_name": "audit_log",
						"partitions": ["p2026_08"],
						"access_type": "index",
						"key": "idx_audit_log_actor_id_created_at"
					}
				}
			}`,
			elapsed:     500 * time.Microsecond,
			wantSummary: "index using idx_audit_log_actor_id_created_at on audit_log, Execution Time: 0.50 ms",
		},
		{
			name: "access node nested under ordering_operation",
			rawJSON: `{
				"query_block": {
					"select_id": 1,
					"ordering_operation": {
						"using_filesort": false,
						"table": {
							"table_name": "audit_log",
							"access_type": "ref",
							"key": "idx_audit_log_actor_id_created_at"
						}
					}
				}
			}`,
			elapsed:     1200 * time.Microsecond,
			wantSummary: "ref using idx_audit_log_actor_id_created_at on audit_log, Execution Time: 1.20 ms",
		},
		{
			name:    "malformed json",
			rawJSON: `not json`,
			wantErr: true,
		},
		{
			name:    "no access node in plan",
			rawJSON: `{"query_block": {"select_id": 1}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePlan(tt.rawJSON, tt.elapsed)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Summary != tt.wantSummary {
				t.Fatalf("Summary = %q, want %q", result.Summary, tt.wantSummary)
			}
			if result.RawPlan != tt.rawJSON {
				t.Fatalf("RawPlan should be the original raw JSON unchanged")
			}
		})
	}
}
