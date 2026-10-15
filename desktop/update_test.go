package main

import "testing"

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.28.0", "0.27.0", true},
		{"0.27.1", "0.27.0", true},
		{"1.0.0", "0.99.9", true},
		{"0.27.0", "0.27.0", false},
		{"0.26.9", "0.27.0", false},
		{"0.28.0", "dev", false},
		{"0.27.0-rc1", "0.26.0", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
