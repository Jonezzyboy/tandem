package prbody

import (
	"strings"
	"testing"
)

func TestBlockOrdersByLevel(t *testing.T) {
	got := Block([]Entry{
		{Level: 3, Ref: "acme/monolith#5521"},
		{Level: 2, Ref: "acme/orchestrator#1873"},
		{Level: 1, Ref: "acme/proto#412"},
		{Level: 2, Ref: "acme/connector#88"},
	})
	want := startMarker + `
### Related PRs — merge in this order
1. acme/proto#412
2. acme/connector#88 · acme/orchestrator#1873
3. acme/monolith#5521
` + endMarker
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestApply(t *testing.T) {
	block := Block([]Entry{{Level: 1, Ref: "a/b#1"}})
	if got := Apply("", block); got != block {
		t.Errorf("empty body: %q", got)
	}
	once := Apply("Fixes retries.\n", block)
	if once != "Fixes retries.\n\n"+block {
		t.Errorf("append: %q", once)
	}
	if twice := Apply(once, block); twice != once {
		t.Errorf("not idempotent: %q", twice)
	}
	newer := Block([]Entry{{Level: 1, Ref: "a/b#1"}, {Level: 2, Ref: "c/d#2"}})
	edited := Apply(once+"\n\nTrailing note", newer)
	if strings.Count(edited, startMarker) != 1 || !strings.Contains(edited, "c/d#2") || !strings.HasSuffix(edited, "Trailing note") {
		t.Errorf("replace: %q", edited)
	}
}
