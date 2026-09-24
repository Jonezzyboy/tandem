// Package core computes a change's state from git, GitHub and manifests, and
// carries out its operations. The CLI and the desktop app both render it.
package core

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/graph"
)

type Graph struct {
	Levels map[string]int
	Edges  []graph.Edge
	// Warnings are unreadable manifests; their edges are missing from Edges.
	Warnings []error
}

func BuildGraph(c *change.Change) (Graph, error) {
	var g Graph
	nodes := make([]graph.Node, 0, len(c.Legs))
	names := make([]string, 0, len(c.Legs))
	for _, l := range c.Legs {
		m, err := graph.ReadManifest(l.Worktree)
		if err != nil {
			g.Warnings = append(g.Warnings, err)
		}
		nodes = append(nodes, graph.Node{Name: l.Repo, Manifest: m})
		names = append(names, l.Repo)
	}
	g.Edges = graph.Infer(nodes)
	for _, e := range c.Declared {
		g.Edges = append(g.Edges, graph.Edge{From: e.From, To: e.To})
	}
	levels, err := graph.Levels(names, g.Edges)
	if err != nil {
		return g, err
	}
	g.Levels = levels
	return g, nil
}

type LegState struct {
	Leg       *change.Leg
	Status    gitx.Status
	StatusErr error
	PR        *gh.PR
	PRErr     error
}

// Snapshot reads every leg's local and, with withPR, GitHub state
// concurrently. A leg whose PR number was unknown gets it filled in, so
// callers should save c after.
func Snapshot(ctx context.Context, c *change.Change, withPR bool) []LegState {
	out := make([]LegState, len(c.Legs))
	var wg sync.WaitGroup
	for i := range c.Legs {
		l := &c.Legs[i]
		out[i].Leg = l
		wg.Go(func() {
			out[i].Status, out[i].StatusErr = gitx.StatusOf(ctx, l.Worktree, l.BaseRef)
		})
		if withPR {
			wg.Go(func() {
				out[i].PR, out[i].PRErr = ViewPR(ctx, c, l)
			})
		}
	}
	wg.Wait()
	for _, s := range out {
		if s.PR != nil && s.Leg.PR == 0 {
			s.Leg.PR, s.Leg.PRURL = s.PR.Number, s.PR.URL
		}
	}
	return out
}

func ViewPR(ctx context.Context, c *change.Change, l *change.Leg) (*gh.PR, error) {
	sel := c.Branch
	if l.PR > 0 {
		sel = strconv.Itoa(l.PR)
	}
	return gh.View(ctx, l.Worktree, sel)
}

// Blockers lists what stands between a leg and merging.
func (s LegState) Blockers() []string {
	var b []string
	if s.PR == nil {
		return append(b, "no PR")
	}
	if s.PR.State != "OPEN" {
		return nil
	}
	if s.PR.IsDraft {
		b = append(b, "draft")
	}
	if r := s.PR.Rollup(); r.Fail > 0 {
		b = append(b, "failing checks")
	}
	switch s.PR.ReviewDecision {
	case "CHANGES_REQUESTED":
		b = append(b, "changes requested")
	case "REVIEW_REQUIRED":
		b = append(b, "awaiting review")
	}
	if s.Status.Dirty > 0 {
		b = append(b, "uncommitted changes")
	}
	return b
}

func SortByLevel(states []LegState, levels map[string]int) {
	sort.SliceStable(states, func(i, j int) bool {
		return legLess(states[i].Leg, states[j].Leg, levels)
	})
}

func OrderedLegs(c *change.Change, levels map[string]int) []*change.Leg {
	legs := make([]*change.Leg, len(c.Legs))
	for i := range c.Legs {
		legs[i] = &c.Legs[i]
	}
	sort.SliceStable(legs, func(i, j int) bool { return legLess(legs[i], legs[j], levels) })
	return legs
}

func legLess(a, b *change.Leg, levels map[string]int) bool {
	if la, lb := levels[a.Repo], levels[b.Repo]; la != lb {
		return la < lb
	}
	return a.Repo < b.Repo
}

func FirstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
