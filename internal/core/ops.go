package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/checks"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

type SyncResult struct {
	Leg     *change.Leg
	OK      bool
	Message string
	// Before and After are the branch's commit around a rebase, both "" when
	// the leg wasn't rebased.
	Before, After string
}

// Sync fetches every leg and rebases the clean ones onto their base.
func Sync(ctx context.Context, c *change.Change, legs []*change.Leg) []SyncResult {
	out := make([]SyncResult, len(legs))
	var wg sync.WaitGroup
	for i, l := range legs {
		wg.Go(func() { out[i] = syncLeg(ctx, c, l, false) })
	}
	wg.Wait()
	return out
}

// SyncStep is what Sync would do to one leg: the git commands it would run,
// or why it would leave the leg alone.
type SyncStep struct {
	Leg      *change.Leg
	Commands []string
	Skip     string
}

// PlanSync fetches every leg and reports what Sync would then do, changing
// nothing else.
func PlanSync(ctx context.Context, c *change.Change, legs []*change.Leg) []SyncStep {
	out := make([]SyncStep, len(legs))
	var wg sync.WaitGroup
	for i, l := range legs {
		wg.Go(func() {
			r := syncLeg(ctx, c, l, true)
			step := SyncStep{Leg: l, Commands: []string{"git fetch origin"}}
			switch {
			case !r.OK:
				step.Skip = r.Message
			case strings.HasPrefix(r.Message, "up to date"):
				step.Skip = r.Message
			default:
				step.Commands = append(step.Commands, "git rebase "+l.BaseRef)
			}
			out[i] = step
		})
	}
	wg.Wait()
	return out
}

// syncLeg rebases only a clean checkout of the branch, and aborts a conflicted rebase so the
// leg is left exactly as it was. With dryRun it fetches and stops short of rebasing.
func syncLeg(ctx context.Context, c *change.Change, l *change.Leg, dryRun bool) SyncResult {
	dir := l.Dir()
	res := func(msg string, ok bool) SyncResult { return SyncResult{Leg: l, OK: ok, Message: msg} }
	if gitx.HasRemote(ctx, dir, "origin") {
		if err := gitx.Fetch(ctx, dir); err != nil {
			return res(FirstLine(err.Error()), false)
		}
	}
	st, err := gitx.StatusOf(ctx, dir, c.Branch, l.BaseRef)
	if err != nil {
		return res(FirstLine(err.Error()), false)
	}
	if st.Current != c.Branch {
		return res("skipped: on "+orDetached(st.Current)+", not "+c.Branch+" (switch to rebase it)", false)
	}
	if st.Dirty > 0 {
		return res(fmt.Sprintf("skipped: %d uncommitted files", st.Dirty), false)
	}
	if st.Behind == 0 {
		return res("up to date with "+l.BaseRef, true)
	}
	if dryRun {
		return res(fmt.Sprintf("%d new commits on %s", st.Behind, l.BaseRef), true)
	}
	before, err := gitx.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return res(FirstLine(err.Error()), false)
	}
	if _, err := gitx.Run(ctx, dir, "rebase", "--quiet", l.BaseRef); err != nil {
		gitx.Run(ctx, dir, "rebase", "--abort")
		return res("conflict rebasing onto "+l.BaseRef+": left unchanged, rebase by hand", false)
	}
	after, _ := gitx.Run(ctx, dir, "rev-parse", "HEAD")
	msg := fmt.Sprintf("rebased onto %s (%d new commits)", l.BaseRef, st.Behind)
	if st.Ahead > 0 && gitx.RefExists(ctx, dir, "refs/remotes/origin/"+c.Branch) {
		msg += " · already pushed: next publish needs force-with-lease"
	}
	r := res(msg, true)
	r.Before, r.After = before, after
	return r
}

// RunChecks runs a leg's checks in order, provided the repo has the change's
// branch checked out. onEvent, when set, is called with done=false as each
// check starts and done=true once it has a result; skipped checks only get
// the latter.
func RunChecks(ctx context.Context, c *change.Change, l *change.Leg, onEvent func(r checks.Result, done bool)) ([]checks.Result, error) {
	if err := requireBranch(ctx, c, l); err != nil {
		return nil, err
	}
	list, err := checks.Detect(l.Dir())
	if err != nil {
		return nil, err
	}
	results := make([]checks.Result, 0, len(list))
	for _, c := range list {
		r := checks.Result{Check: c}
		if c.Skip == "" {
			if onEvent != nil {
				onEvent(r, false)
			}
			r = checks.Run(ctx, c)
		}
		results = append(results, r)
		if onEvent != nil {
			onEvent(r, true)
		}
	}
	return results, nil
}

// requireBranch errs unless l's repo has the change's branch checked out.
func requireBranch(ctx context.Context, c *change.Change, l *change.Leg) error {
	if cur := gitx.CurrentBranch(ctx, l.Dir()); cur != c.Branch {
		return fmt.Errorf("%s is on %s, not %s: switch to it first", l.Name(), orDetached(cur), c.Branch)
	}
	return nil
}
