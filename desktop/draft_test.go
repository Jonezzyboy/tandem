package main

import (
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestDraftRoundTrip(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	if err := a.store.Save(&change.Change{ID: "ABC-1", Branch: "ABC-1"}); err != nil {
		t.Fatal(err)
	}
	if d, err := a.Draft("ABC-1"); d != nil || err != nil {
		t.Fatalf("draft before saving = %v, %v", d, err)
	}
	want := PRDraft{Title: "T", Body: "- a\n  - b", Reviewers: "sam"}
	if err := a.SaveDraft("ABC-1", want); err != nil {
		t.Fatal(err)
	}
	if d, err := a.Draft("ABC-1"); err != nil || d == nil || *d != want {
		t.Fatalf("draft = %v, %v", d, err)
	}
	if err := a.DiscardDraft("ABC-1"); err != nil {
		t.Fatal(err)
	}
	if d, _ := a.Draft("ABC-1"); d != nil {
		t.Errorf("draft after discard = %v", d)
	}
	if err := a.SaveDraft("NOPE-1", want); err == nil {
		t.Error("saved a draft for a change that doesn't exist")
	}
}
