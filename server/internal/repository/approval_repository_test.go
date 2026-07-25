package repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBuildListByOrgQuery(t *testing.T) {
	orgID := uuid.New()

	tests := []struct {
		name        string
		status      string
		wantArgs    int
		wantStatus  bool // whether the status predicate should be present
		statusValue string
	}{
		{name: "no status filter", status: "", wantArgs: 1, wantStatus: false},
		{name: "pending filter", status: "pending", wantArgs: 2, wantStatus: true, statusValue: "pending"},
		{name: "approved filter", status: "approved", wantArgs: 2, wantStatus: true, statusValue: "approved"},
		{name: "rejected filter", status: "rejected", wantArgs: 2, wantStatus: true, statusValue: "rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := buildListByOrgQuery(orgID, tt.status)

			if len(args) != tt.wantArgs {
				t.Fatalf("args count: got %d want %d", len(args), tt.wantArgs)
			}
			// org_id must always be the first bound argument.
			if args[0] != orgID {
				t.Fatalf("first arg must be orgID, got %v", args[0])
			}
			// Every query filters by org and orders newest-first.
			if !strings.Contains(sql, "WHERE org_id = $1") {
				t.Fatalf("query must scope by org: %s", sql)
			}
			if !strings.Contains(sql, "ORDER BY requested_at DESC") {
				t.Fatalf("query must order newest-first: %s", sql)
			}

			hasStatus := strings.Contains(sql, "status = $2")
			if hasStatus != tt.wantStatus {
				t.Fatalf("status predicate presence: got %v want %v (sql=%s)", hasStatus, tt.wantStatus, sql)
			}
			if tt.wantStatus {
				if args[1] != tt.statusValue {
					t.Fatalf("status arg: got %v want %q", args[1], tt.statusValue)
				}
			}
		})
	}
}
