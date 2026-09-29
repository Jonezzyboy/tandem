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
	"golang.org/x/mod/modfile"
	gomodule "golang.org/x/mod/module"
)

type PinResult struct {
	Upstream   *change.Leg
	Downstream *change.Leg
	Module     string
	Rev        string
	// Merged says Rev is the upstream PR's merge commit, not its branch head.
	Merged    bool
	Changed   bool
	Committed bool
	Pushed    bool
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

type upstreamRev struct {
	sha    string
	merged bool
	why    string
}

// Pin points every downstream Go requirement at its upstream leg: the merge
// commit once the upstream PR has merged, otherwise its pushed HEAD, since go
// get fetches the commit from origin. With commit, go.mod and go.sum are
// committed and nothing else is; with push too, each downstream that gained a
// commit is pushed.
func Pin(ctx context.Context, c *change.Change, g Graph, commit, push bool) []PinResult {
	edges := goEdges(g)
	out := make([]PinResult, len(edges))
	revs := map[string]upstreamRev{}
	for _, e := range edges {
		if _, done := revs[e.From]; done {
			continue
		}
		up, _ := c.Leg(e.From)
		revs[e.From] = resolveUpstream(ctx, c, up)
	}

	// Edges into the same downstream run in sequence: they may edit one go.mod.
	byDown := map[string][]int{}
	for i, e := range edges {
		up, _ := c.Leg(e.From)
		down, _ := c.Leg(e.To)
		r := revs[e.From]
		out[i] = PinResult{Upstream: up, Downstream: down, Module: e.Via, Rev: r.sha, Merged: r.merged, Skipped: r.why}
		if out[i].Skipped == "" {
			byDown[e.To] = append(byDown[e.To], i)
		}
	}
	var wg sync.WaitGroup
	for _, idx := range byDown {
		wg.Go(func() {
			var committed []int
			for _, i := range idx {
				r := &out[i]
				e := edges[i]
				msg := fmt.Sprintf("Pin %s to %s", e.Via, short(r.Rev))
				if r.Merged {
					msg = fmt.Sprintf("Pin %s to merged %s", e.Via, short(r.Rev))
				}
				r.Changed, r.Committed, r.Err = PinTo(ctx, c, r.Downstream, e.Dir, e.Via, r.Rev, commit, msg)
				if r.Committed {
					committed = append(committed, i)
				}
			}
			if !push || len(committed) == 0 {
				return
			}
			err := pushBranch(ctx, c, out[committed[0]].Downstream, false)
			for _, i := range committed {
				if err != nil {
					out[i].Err = fmt.Errorf("committed the pin but %w", err)
				} else {
					out[i].Pushed = true
				}
			}
		})
	}
	wg.Wait()
	return out
}

func resolveUpstream(ctx context.Context, c *change.Change, l *change.Leg) upstreamRev {
	if pr, err := ViewPR(ctx, c, l); err == nil && pr != nil && pr.State == "MERGED" {
		if sha := pr.MergeSHA(); sha != "" {
			return upstreamRev{sha: sha, merged: true}
		}
		return upstreamRev{why: fmt.Sprintf("%s merged but GitHub reported no merge commit to pin to", l.Name())}
	}
	sha, why := pushedHead(ctx, c, l)
	return upstreamRev{sha: sha, why: why}
}

// pushedHead is the tip of the upstream's branch (checked out or not), provided
// origin already has it.
func pushedHead(ctx context.Context, c *change.Change, l *change.Leg) (string, string) {
	dir := l.Dir()
	if gitx.HasRemote(ctx, dir, "origin") {
		if err := gitx.Fetch(ctx, dir); err != nil {
			return "", FirstLine(err.Error())
		}
	}
	head, err := gitx.Run(ctx, dir, "rev-parse", "refs/heads/"+c.Branch)
	if err != nil {
		return "", FirstLine(err.Error())
	}
	remote, err := gitx.Run(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+c.Branch)
	if err != nil || remote != head {
		return "", fmt.Sprintf("%s has commits that are not pushed: publish it first", l.Name())
	}
	return head, ""
}

// pinnedTo reports whether the go.mod in dir (relative to the leg's working
// tree) requires module at a pseudo-version of rev.
func pinnedTo(l *change.Leg, dir, module, rev string) bool {
	path := filepath.Join(l.Dir(), dir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	f, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		return false
	}
	for _, r := range f.Require {
		if r.Mod.Path != module || !gomodule.IsPseudoVersion(r.Mod.Version) {
			continue
		}
		v, err := gomodule.PseudoVersionRev(r.Mod.Version)
		return err == nil && strings.HasPrefix(rev, v)
	}
	return false
}

// PinTo runs go get module@rev in dir (relative to the leg's repo), which must
// have the change's branch checked out. It reports whether go.mod or go.sum
// changed and whether that was committed.
func PinTo(ctx context.Context, c *change.Change, l *change.Leg, dir, module, rev string, commit bool, message string) (bool, bool, error) {
	if err := requireBranch(ctx, c, l); err != nil {
		return false, false, err
	}
	root := l.Dir()
	abs := filepath.Join(root, dir)
	files := []string{filepath.Join(dir, "go.mod"), filepath.Join(dir, "go.sum")}
	if commit {
		out, err := gitx.Run(ctx, root, append([]string{"status", "--porcelain", "--"}, files...)...)
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
	diff, err := gitx.Run(ctx, root, append([]string{"status", "--porcelain", "--"}, files...)...)
	if err != nil || diff == "" {
		return false, false, err
	}
	if !commit {
		return true, false, nil
	}
	var present []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			present = append(present, f)
		}
	}
	if _, err := gitx.Run(ctx, root, append([]string{"add", "--"}, present...)...); err != nil {
		return true, false, err
	}
	if _, err := gitx.Run(ctx, root, append([]string{"commit", "--quiet", "-m", message, "--"}, present...)...); err != nil {
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
