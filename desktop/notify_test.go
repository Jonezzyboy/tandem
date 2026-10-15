package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
)

func leg(repo string, pr core.PRView) core.LegView {
	return core.LegView{Repo: "acme/" + repo, Name: repo, PR: &pr}
}

func TestChangeNotes(t *testing.T) {
	prev := core.ChangeView{ID: "ABC-1", Remote: true, Legs: []core.LegView{
		leg("api", core.PRView{Number: 1, State: "OPEN", Pending: 2}),
		leg("web", core.PRView{Number: 2, State: "OPEN", Pass: 3}),
		leg("proto", core.PRView{Number: 3, State: "OPEN", Pass: 1, Review: "REVIEW_REQUIRED"}),
		leg("cli", core.PRView{Number: 4, State: "OPEN", Pass: 1}),
	}}
	next := core.ChangeView{ID: "ABC-1", Remote: true, Legs: []core.LegView{
		leg("api", core.PRView{Number: 1, State: "OPEN", Pass: 2}),
		leg("web", core.PRView{Number: 2, State: "OPEN", Pass: 2, Fail: 1, Failing: []string{"test"}}),
		leg("proto", core.PRView{Number: 3, State: "OPEN", Pass: 1, Review: "APPROVED"}),
		leg("cli", core.PRView{Number: 4, State: "MERGED", Pass: 1}),
	}}
	var got []string
	for _, n := range changeNotes(prev, next) {
		got = append(got, n.Kind+": "+n.Body)
	}
	want := []string{
		"ci: api #1 CI passed",
		"ci: web #2 CI failing: test",
		"review: proto #3 approved",
		"merged: cli #4 merged",
	}
	if !slices.Equal(got, want) {
		t.Errorf("notes = %q, want %q", got, want)
	}
	if n := changeNotes(core.ChangeView{ID: "ABC-1"}, next); n != nil {
		t.Errorf("first remote read notified: %v", n)
	}
	if n := changeNotes(next, next); n != nil {
		t.Errorf("unchanged view notified: %v", n)
	}
}

func TestNotifyFiltersAndGroups(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	var sent []Note
	a.send = func(n Note) { sent = append(sent, n) }
	a.settings.Notify[notifyReview] = false
	a.views["ABC-1"] = core.ChangeView{ID: "ABC-1", Title: "Retries"}
	a.notify(
		Note{Kind: notifyCI, Change: "ABC-1", Body: "api #1 CI passed"},
		Note{Kind: notifyMerged, Change: "ABC-1", Body: "cli #4 merged"},
		Note{Kind: notifyReview, Change: "ABC-1", Body: "proto #3 approved"},
	)
	if len(sent) != 1 || sent[0].Title != "ABC-1 · Retries" || sent[0].Body != "api #1 CI passed\ncli #4 merged" {
		t.Fatalf("sent = %+v", sent)
	}

	sent = nil
	a.active, a.focus = true, "ABC-1"
	a.notify(Note{Kind: notifyCI, Change: "ABC-1", Body: "x"}, Note{Kind: notifyCI, Change: "ABC-2", Body: "y"})
	if len(sent) != 1 || sent[0].Change != "ABC-2" {
		t.Errorf("with ABC-1 on screen, sent = %+v", sent)
	}
}

func TestReviewRequests(t *testing.T) {
	old := &Inbox{Review: []InboxItem{{URL: "u1"}}}
	next := Inbox{Review: []InboxItem{{URL: "u1"}, {URL: "u2", Repo: "acme/api", Number: 9, Title: "Fix", Author: "sam"}}}
	got := reviewRequests(old, next)
	if len(got) != 1 || got[0].Title != "Review requested by sam" || got[0].Body != "acme/api #9 · Fix" {
		t.Errorf("notes = %+v", got)
	}
	if reviewRequests(nil, next) != nil {
		t.Error("first inbox read notified")
	}
}

func TestNormalizeRootsAndNotify(t *testing.T) {
	home, _ := os.UserHomeDir()
	s := normalize(Settings{Roots: []string{" ~/src ", "/x/../y", "", "~/src"}, Notify: map[string]bool{notifyCI: false}})
	if want := []string{filepath.Join(home, "src"), "/y"}; !slices.Equal(s.Roots, want) {
		t.Errorf("roots = %q, want %q", s.Roots, want)
	}
	for _, k := range notifyKinds {
		if s.Notify[k] != (k != notifyCI) {
			t.Errorf("notify[%s] = %v", k, s.Notify[k])
		}
	}
}

func TestCodeRootsPrefersSettings(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, []string{"/default"})
	if got := a.codeRoots(); !slices.Equal(got, []string{"/default"}) {
		t.Errorf("codeRoots = %q", got)
	}
	a.settings.Roots = []string{"/chosen"}
	if got := a.codeRoots(); !slices.Equal(got, []string{"/chosen"}) {
		t.Errorf("codeRoots = %q", got)
	}
}

func TestRunSetupFixOnlyKnownCommands(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	if err := a.RunSetupFix("rm -rf /"); err == nil {
		t.Error("unknown fix accepted")
	}
}
