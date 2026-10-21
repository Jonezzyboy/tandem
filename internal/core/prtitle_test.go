package core

import (
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestPRTitle(t *testing.T) {
	c := &change.Change{ID: "ABC-123", Title: "Add request retries"}
	for _, tc := range []struct{ in, want string }{
		{"", "ABC-123: Add request retries"},
		{"Retry 503s", "ABC-123: Retry 503s"},
		{"ABC-123 Retry 503s", "ABC-123: Retry 503s"},
		{"ABC-123: Retry 503s", "ABC-123: Retry 503s"},
		{"ABC-123:Retry 503s", "ABC-123: Retry 503s"},
	} {
		if got, err := PRTitle(c, tc.in); err != nil || got != tc.want {
			t.Errorf("PRTitle(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := PRTitle(&change.Change{ID: "ABC-1"}, "ABC-1:"); err == nil {
		t.Error("a title of just the ID was accepted")
	}
}
