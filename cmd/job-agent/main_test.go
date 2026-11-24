package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	data := `{"companies":["example"],"as_of":"2026-01-01T00:00:00Z","max_age_seconds":600,"runs":[{"job":"discover-companies","outcome":"success","finished_at":"2025-12-31T23:59:00Z"},{"job":"poll-boards","outcome":"success","finished_at":"2025-12-31T23:59:00Z"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args []string
		want int
	}{
		{nil, 2},
		{[]string{"unknown", "--once"}, 2},
		{[]string{"discover-companies"}, 2},
		{[]string{"discover-companies", "--once"}, 1},
		{[]string{"poll-boards", "--once"}, 1},
		{[]string{"discovery-monitor", "--once"}, 1},
		{[]string{"discover-companies", "--once", "--fixture", path}, 0},
		{[]string{"poll-boards", "--once", "--fixture", path}, 0},
		{[]string{"discovery-monitor", "--once", "--fixture", path}, 0},
		{[]string{"discover-companies", "--once", "--fixture", path, "extra"}, 2},
	}
	for _, test := range tests {
		if got := run(test.args); got != test.want {
			t.Errorf("run(%v) = %d, want %d", test.args, got, test.want)
		}
	}
}

func TestMonitorRejectsMissingSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	data := `{"as_of":"2026-01-01T00:00:00Z","max_age_seconds":600,"runs":[]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if got := run([]string{"discovery-monitor", "--once", "--fixture", path}); got != 1 {
		t.Fatalf("missing runs returned %d", got)
	}
}
