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

func manifestFiles(dir string) []string {
	return []string{filepath.Join(dir, "go.mod"), filepath.Join(dir, "go.sum")}
}

// pinnedTo reports whether the leg's go.mod in dir requires module at a
// pseudo-version of rev: at ref, or in the working tree when ref is empty.
func pinnedTo(ctx context.Context, l *change.Leg, dir, module, rev, ref string) bool {
	path := filepath.Join(dir, "go.mod")
	var data []byte
	if ref != "" {
		out, err := gitx.Run(ctx, l.Dir(), "show", ref+":"+filepath.ToSlash(filepath.Clean(path)))
		if err != nil {
			return false
		}
		data = []byte(out)
	} else {
		var err error
		if data, err = os.ReadFile(filepath.Join(l.Dir(), path)); err != nil {
			return false
		}
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
		return err == nil && v != "" && strings.HasPrefix(rev, v)
	}
	return false
}

// PinView is a Go requirement of an open downstream leg on an upstream leg
// whose PR has merged, while the downstream's HEAD isn't at the merge commit.
type PinView struct {
	Module   string `json:"module"`
	Upstream string `json:"upstream"`
	Dir      string `json:"dir"`
	Rev      string `json:"rev"`
	// Applied means the working tree is already at Rev, waiting to be committed.
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// MergedPins fills each open leg's Pins from the merge commits the view's PRs
// report.
func MergedPins(ctx context.Context, c *change.Change, g Graph, v *ChangeView) {
	idx := map[string]int{}
	for i := range v.Legs {
		idx[v.Legs[i].Repo] = i
		v.Legs[i].Pins = nil
	}
	for _, e := range goEdges(g) {
		ui, uok := idx[e.From]
		di, dok := idx[e.To]
		if !uok || !dok {
			continue
		}
		up, down := &v.Legs[ui], &v.Legs[di]
		if up.PR == nil || up.PR.MergeSHA == "" || down.PR == nil || down.PR.State != "OPEN" {
			continue
		}
		l, err := c.Leg(e.To)
		if err != nil || pinnedTo(ctx, l, e.Dir, e.Via, up.PR.MergeSHA, branchTip(ctx, l, c.Branch)) {
			continue
		}
		down.Pins = append(down.Pins, PinView{
			Module: e.Via, Upstream: up.Name, Dir: e.Dir, Rev: up.PR.MergeSHA,
			Applied: down.OnBranch && pinnedTo(ctx, l, e.Dir, e.Via, up.PR.MergeSHA, ""),
		})
	}
}

// branchTip is the ref holding the leg's branch as its PR sees it: origin's,
// or the local branch before it is pushed. Whatever the clone has checked out
// doesn't matter.
func branchTip(ctx context.Context, l *change.Leg, branch string) string {
	if ref := "refs/remotes/origin/" + branch; gitx.RefExists(ctx, l.Dir(), ref) {
		return ref
	}
	return "refs/heads/" + branch
}

// ManifestsClean reports whether go.mod and go.sum in dir have no uncommitted
// changes, so a go get there touches nothing but its own edit.
func ManifestsClean(ctx context.Context, l *change.Leg, dir string) bool {
	out, err := gitx.Run(ctx, l.Dir(), append([]string{"status", "--porcelain", "--"}, manifestFiles(dir)...)...)
	return err == nil && out == ""
}

// CommitPins applies any of pins not yet in the working tree, commits go.mod
// and go.sum for all of them and pushes the leg.
func CommitPins(ctx context.Context, c *change.Change, l *change.Leg, pins []PinView) error {
	if err := requireBranch(ctx, c, l); err != nil {
		return err
	}
	var dirs, msgs []string
	for _, p := range pins {
		if !p.Applied {
			if !ManifestsClean(ctx, l, p.Dir) {
				return fmt.Errorf("%s has other uncommitted go.mod/go.sum changes: commit or discard them first", l.Name())
			}
			if _, _, err := PinTo(ctx, c, l, p.Dir, p.Module, p.Rev, false, ""); err != nil {
				return err
			}
		}
		dirs = append(dirs, p.Dir)
		msgs = append(msgs, fmt.Sprintf("Pin %s to merged %s", p.Module, short(p.Rev)))
	}
	committed, err := commitManifests(ctx, l.Dir(), dirs, strings.Join(msgs, "; "))
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("%s: go.mod and go.sum have nothing to commit", l.Name())
	}
	if err := pushBranch(ctx, c, l, false); err != nil {
		return fmt.Errorf("committed the pin but %w", err)
	}
	return nil
}

// commitManifests commits go.mod and go.sum in dirs and nothing else,
// reporting whether there was anything to commit.
func commitManifests(ctx context.Context, root string, dirs []string, message string) (bool, error) {
	var present []string
	for _, d := range dirs {
		for _, f := range manifestFiles(d) {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				present = append(present, f)
			}
		}
	}
	if len(present) == 0 {
		return false, nil
	}
	if _, err := gitx.Run(ctx, root, append([]string{"add", "--"}, present...)...); err != nil {
		return false, err
	}
	staged, err := gitx.Run(ctx, root, append([]string{"diff", "--cached", "--name-only", "--"}, present...)...)
	if err != nil || staged == "" {
		return false, err
	}
	if _, err := gitx.Run(ctx, root, append([]string{"commit", "--quiet", "-m", message, "--"}, present...)...); err != nil {
		return false, err
	}
	return true, nil
}

// PinTo runs go get module@rev in dir (relative to the leg's repo), which must
// have the change's branch checked out. It reports whether go.mod or go.sum
// changed and whether that was committed.
func PinTo(ctx context.Context, c *change.Change, l *change.Leg, dir, module, rev string, commit bool, message string) (bool, bool, error) {
	if err := requireBranch(ctx, c, l); err != nil {
		return false, false, err
	}
	root := l.Dir()
	files := manifestFiles(dir)
	if commit && !ManifestsClean(ctx, l, dir) {
		return false, false, fmt.Errorf("%s has uncommitted go.mod/go.sum changes: commit or discard them first", l.Name())
	}
	cmd := exec.CommandContext(ctx, "go", "get", module+"@"+rev)
	cmd.Dir = filepath.Join(root, dir)
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, false, fmt.Errorf("go get %s@%s: %s", module, short(rev), goError(string(out)))
	}
	diff, err := gitx.Run(ctx, root, append([]string{"status", "--porcelain", "--"}, files...)...)
	if err != nil || diff == "" {
		return false, false, err
	}
	if !commit {
		return true, false, nil
	}
	committed, err := commitManifests(ctx, root, []string{dir}, message)
	return true, committed, err
}

// goError is go's output without its progress lines, on one line: callers
// show only the first line, and "go: downloading" would hide the failure.
func goError(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "go: downloading ") || strings.HasPrefix(line, "go: finding ") || strings.HasPrefix(line, "go: extracting ") {
			continue
		}
		keep = append(keep, line)
	}
	if len(keep) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.Join(keep, "; ")
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
