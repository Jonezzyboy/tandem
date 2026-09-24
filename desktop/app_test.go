package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
)

func TestWithin(t *testing.T) {
	home := t.TempDir()
	a := NewApp(change.Store{Home: home}, nil)
	if _, err := a.within(filepath.Join(home, "ABC-1", "api")); err != nil {
		t.Errorf("worktree rejected: %v", err)
	}
	for _, p := range []string{filepath.Dir(home), filepath.Join(home, "..", "x"), "/etc"} {
		if _, err := a.within(p); err == nil {
			t.Errorf("%s allowed", p)
		}
	}
}

func TestMergePath(t *testing.T) {
	got := mergePath("/a:/b", "/b:/c::/a:/d")
	if got != "/a:/b:/c:/d" {
		t.Errorf("mergePath = %q", got)
	}
}

func TestSameViewIgnoresCheckedAt(t *testing.T) {
	a := core.ChangeView{ID: "X", CheckedAt: time.Now()}
	b := a
	b.CheckedAt = a.CheckedAt.Add(time.Minute)
	if !sameView(a, b) {
		t.Error("views differing only in CheckedAt reported as changed")
	}
	b.Blocked = 1
	if sameView(a, b) {
		t.Error("different views reported as same")
	}
}

func TestMergeLocalKeepsGitHubState(t *testing.T) {
	remoteAt := time.Now().Add(-time.Minute)
	prev := core.ChangeView{Remote: true, RemoteAt: remoteAt, Blocked: 1, Legs: []core.LegView{{
		Repo: "acme/api", PR: &core.PRView{Number: 7, State: "OPEN"}, Blockers: []string{"awaiting review"},
	}}}
	next := core.ChangeView{Legs: []core.LegView{{Repo: "acme/api", Dirty: 2, Blockers: []string{}}}}
	got := core.MergeLocal(prev, next)
	l := got.Legs[0]
	if !got.Remote || !got.RemoteAt.Equal(remoteAt) || l.PR == nil || l.PR.Number != 7 {
		t.Fatalf("GitHub state lost: %+v", got)
	}
	if len(l.Blockers) != 2 || l.Blockers[1] != "uncommitted changes" || got.Blocked != 1 {
		t.Errorf("blockers = %v, blocked = %d", l.Blockers, got.Blocked)
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "leg") != "1 leg" || plural(3, "leg") != "3 legs" {
		t.Error(plural(1, "leg"), plural(3, "leg"))
	}
}
