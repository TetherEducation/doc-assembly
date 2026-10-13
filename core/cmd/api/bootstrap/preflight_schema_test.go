package bootstrap

import (
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/migrations"
)

// The engine in these cases embeds migrations up to 23.
func TestEvaluateSchema(t *testing.T) {
	const latest = 23
	for _, tc := range []struct {
		name      string
		state     migrations.State
		wantAhead bool
		wantErr   string // empty: must start
	}{
		{"not initialized", migrations.State{}, false, "database schema not initialized"},
		{"first migration never completed", migrations.State{Dirty: true}, false, "database schema not initialized"},
		{"clean v22: one behind", migrations.State{Version: 22, Initialized: true}, false, "at version 22, but this engine requires 23"},
		{"dirty v22: behind and broken", migrations.State{Version: 22, Dirty: true, Initialized: true}, false, "at version 22 (dirty), but this engine requires 23"},
		{"clean v23: current", migrations.State{Version: 23, Initialized: true}, false, ""},
		{"dirty v23: own migration half-applied", migrations.State{Version: 23, Dirty: true, Initialized: true}, false, "migration 23 is dirty"},
		{"clean v24: ahead after a release or rollback", migrations.State{Version: 24, Initialized: true}, true, ""},
		{"dirty v24: a newer migration is running", migrations.State{Version: 24, Dirty: true, Initialized: true}, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ahead, err := evaluateSchema(tc.state, latest)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want start, got %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			if ahead != tc.wantAhead {
				t.Fatalf("ahead = %v, want %v", ahead, tc.wantAhead)
			}
		})
	}
}
