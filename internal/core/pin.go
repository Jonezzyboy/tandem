package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/graph"
)

type PinResult struct {
	Upstream   *change.Leg
	Downstream *change.Leg
	Module     string
	Rev        string
	Changed    bool
	Committed  bool
	// Skipped says why nothing was attempted; Err is a failure while pinning.
	Skipped string
	Err     error
}

// goEdges returns the edges pinning can act on: a Go module one leg provides
// and another requires.
func goEdges(g Graph) []graph.Edge {
	var out []graph.Edge
	for _, e := range g.Edges {
		if e.Kind == graph.Go {
			out = append(out, e)
		}
	}
	return out
}

// Pin points every downstream Go requirement at its upstream leg's pushed HEAD.
// The upstream must be pushed first, since go get fetches the commit from its
// origin. With commit, go.mod and go.sum are committed and nothing else is.
func Pin(ctx context.Context, c *change.Change, g Graph, commit bool) []PinResult {
	edges := goEdges(g)
	out := make([]PinResult, len(edges))
	heads := map[string]string{}
	headErr := map[string]string{}
	for _, e := range edges {
		if _, done := heads[e.From]; done {
			continue
		}
		up, _ := c.Leg(e.From)
		sha, why := pushedHead(ctx, c, up)
		heads[e.From], headErr[e.From] = sha, why
	}

	// Edges into the same downstream run in sequence: they may edit one go.mod.
	byDown := map[string][]int{}
	for i, e := range edges {
		up, _ := c.Leg(e.From)
		down, _ := c.Leg(e.To)
		out[i] = PinResult{Upstream: up, Downstream: down, Module: e.Via, Rev: heads[e.From], Skipped: headErr[e.From]}
		if out[i].Skipped == "" {
			byDown[e.To] = append(byDown[e.To], i)
		}
	}
	var wg sync.WaitGroup
	for _, idx := range byDown {
		wg.Go(func() {
			for _, i := range idx {
				r := &out[i]
				e := edges[i]
				msg := fmt.Sprintf("Pin %s to %s", e.Via, short(r.Rev))
				r.Changed, r.Committed, r.Err = PinTo(ctx, r.Downstream, e.Dir, e.Via, r.Rev, commit, msg)
			}
		})
	}
	wg.Wait()
	return out
}

func pushedHead(ctx context.Context, c *change.Change, l *change.Leg) (string, string) {
	if gitx.HasRemote(ctx, l.Worktree, "origin") {
		if err := gitx.Fetch(ctx, l.Worktree); err != nil {
			return "", FirstLine(err.Error())
		}
	}
	head, err := gitx.Run(ctx, l.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", FirstLine(err.Error())
	}
	remote, err := gitx.Run(ctx, l.Worktree, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+c.Branch)
	if err != nil || remote != head {
		return "", fmt.Sprintf("%s has commits that are not pushed: publish it first", l.Name())
	}
	return head, ""
}

// PinTo runs go get module@rev in dir (relative to the leg's worktree). It
// reports whether go.mod or go.sum changed and whether that was committed.
func PinTo(ctx context.Context, l *change.Leg, dir, module, rev string, commit bool, message string) (bool, bool, error) {
	abs := filepath.Join(l.Worktree, dir)
	files := []string{filepath.Join(dir, "go.mod"), filepath.Join(dir, "go.sum")}
	if commit {
		out, err := gitx.Run(ctx, l.Worktree, append([]string{"status", "--porcelain", "--"}, files...)...)
		if err != nil {
			return false, false, err
		}
		if out != "" {
			return false, false, fmt.Errorf("%s has uncommitted go.mod/go.sum changes: commit or discard them first", l.Name())
		}
	}
	cmd := exec.CommandContext(ctx, "go", "get", module+"@"+rev)
	cmd.Dir = abs
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, false, fmt.Errorf("go get %s@%s: %s", module, short(rev), strings.TrimSpace(string(out)))
	}
	diff, err := gitx.Run(ctx, l.Worktree, append([]string{"status", "--porcelain", "--"}, files...)...)
	if err != nil || diff == "" {
		return false, false, err
	}
	if !commit {
		return true, false, nil
	}
	var present []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(l.Worktree, f)); err == nil {
			present = append(present, f)
		}
	}
	if _, err := gitx.Run(ctx, l.Worktree, append([]string{"add", "--"}, present...)...); err != nil {
		return true, false, err
	}
	if _, err := gitx.Run(ctx, l.Worktree, append([]string{"commit", "--quiet", "-m", message, "--"}, present...)...); err != nil {
		return true, false, err
	}
	return true, true, nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
