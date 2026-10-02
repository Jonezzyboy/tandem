package change

import (
	"slices"
	"testing"
	"time"
)

func TestRemoveLegDropsItsDeclaredEdges(t *testing.T) {
	c := &Change{ID: "X", Legs: []Leg{{Repo: "acme/proto"}, {Repo: "acme/grpc"}, {Repo: "acme/api"}}}
	c.AddDeclared(Edge{From: "acme/grpc", To: "acme/api"})
	c.AddDeclared(Edge{From: "acme/proto", To: "acme/api"})

	removed, err := c.RemoveLeg("grpc")
	if err != nil || removed.Repo != "acme/grpc" {
		t.Fatalf("RemoveLeg = %+v, %v", removed, err)
	}
	if len(c.Legs) != 2 || !slices.Equal(c.Declared, []Edge{{From: "acme/proto", To: "acme/api"}}) {
		t.Errorf("after remove: legs %v, declared %v", c.Legs, c.Declared)
	}
	if _, err := c.RemoveLeg("grpc"); err == nil {
		t.Error("removing a missing leg succeeded")
	}
	if !c.RemoveDeclared(Edge{From: "acme/proto", To: "acme/api"}) || len(c.Declared) != 0 {
		t.Errorf("RemoveDeclared left %v", c.Declared)
	}
	if c.RemoveDeclared(Edge{From: "acme/proto", To: "acme/api"}) {
		t.Error("removed an edge twice")
	}
}

func TestUsesWorktrees(t *testing.T) {
	cases := []struct {
		c    Change
		want bool
	}{
		{Change{Worktrees: true}, true},
		{Change{Legs: []Leg{{Worktree: "/w/a"}, {Worktree: "/w/b"}}}, true},
		{Change{Legs: []Leg{{Worktree: "/w/a"}, {Source: "/r/b"}}}, false},
		{Change{}, false},
	}
	for i, tc := range cases {
		if got := tc.c.UsesWorktrees(); got != tc.want {
			t.Errorf("case %d: UsesWorktrees = %v, want %v", i, got, tc.want)
		}
	}
}

func TestListFollowsSavedOrder(t *testing.T) {
	s := Store{Home: t.TempDir()}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"A-1", "B-2", "C-3", "D-4"} {
		if err := s.Save(&Change{ID: id, Branch: id, Created: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	ids := func() []string {
		all, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range all {
			out = append(out, c.ID)
		}
		return out
	}
	if got := ids(); !slices.Equal(got, []string{"D-4", "C-3", "B-2", "A-1"}) {
		t.Errorf("unordered = %v, want newest first", got)
	}
	// GONE-9 was cleaned; D-4 is unranked, so it leads.
	if err := s.SetOrder([]string{"B-2", "GONE-9", "A-1", "C-3"}); err != nil {
		t.Fatal(err)
	}
	if got := ids(); !slices.Equal(got, []string{"D-4", "B-2", "A-1", "C-3"}) {
		t.Errorf("ordered = %v", got)
	}
}
