package scheduler

import "testing"

// InputJSON arrives as float64 from JSON, but a task written in Go or seeded
// by hand can carry an int. Reading only one of them silently drops the
// setting and the run falls back to defaults — a schedule configured for two
// jobs would quietly prepare five.
func TestNumFrom(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"json number", float64(4), 4, true},
		{"go int", 4, 4, true},
		{"int64", int64(4), 4, true},
		{"fractional min_score", 4.5, 4.5, true},
		{"string is not a number", "4", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := numFrom(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
