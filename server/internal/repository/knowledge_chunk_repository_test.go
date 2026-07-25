package repository

import "testing"

func TestVectorString(t *testing.T) {
	cases := []struct {
		name string
		in   []float32
		want string
	}{
		{"empty", nil, "[]"},
		{"single", []float32{0.5}, "[0.5]"},
		{"multiple", []float32{0.1, 0.2, 0.3}, "[0.1,0.2,0.3]"},
		{"negative", []float32{-1, 0, 1}, "[-1,0,1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VectorString(c.in); got != c.want {
				t.Errorf("VectorString(%v) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}
