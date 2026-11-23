package main

import "testing"

func TestRun(t *testing.T) {
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
	}
	for _, test := range tests {
		if got := run(test.args); got != test.want {
			t.Errorf("run(%v) = %d, want %d", test.args, got, test.want)
		}
	}
}
