package main

import (
	"fmt"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestHistory(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	if err := a.store.Save(&change.Change{ID: "X-1", Branch: "X-1"}); err != nil {
		t.Fatal(err)
	}
	a.Record("X-1", "first", "muted")
	a.activity("X-1", "second", "ok")
	a.Record("X-1", "  ", "muted")
	a.Record("NOPE-1", "lost", "muted")
	got := a.History("X-1")
	if len(got) != 2 || got[0].Text != "second" || got[0].Tone != "ok" || got[1].Text != "first" {
		t.Fatalf("history = %+v", got)
	}
	for i := range historyKeep * 6 / 5 {
		a.Record("X-1", fmt.Sprint(i), "muted")
	}
	got = a.History("X-1")
	if len(got) > historyKeep*6/5 || got[0].Text != fmt.Sprint(historyKeep*6/5-1) {
		t.Errorf("after trimming: %d entries, newest %q", len(got), got[0].Text)
	}
}
