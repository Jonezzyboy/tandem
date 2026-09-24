// Package core computes a change's state from git, GitHub and manifests. The
// CLI renders it; the desktop app is meant to render the same values.
package core

import (
	"context"
	"strconv"
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

// Snapshot reads every leg's local and GitHub state concurrently. A leg whose
// PR number was unknown gets it filled in, so callers should save c after.
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
				sel := c.Branch
				if l.PR > 0 {
					sel = strconv.Itoa(l.PR)
				}
				out[i].PR, out[i].PRErr = gh.View(ctx, l.Worktree, sel)
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

// Blockers counts what stands between a leg and merging.
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
