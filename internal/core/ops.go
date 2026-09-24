package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/checks"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

type SyncResult struct {
	Leg     *change.Leg
	OK      bool
	Message string
}

// Sync fetches every leg and rebases the clean ones onto their base.
func Sync(ctx context.Context, c *change.Change, legs []*change.Leg) []SyncResult {
	out := make([]SyncResult, len(legs))
	var wg sync.WaitGroup
	for i, l := range legs {
		wg.Go(func() {
			msg, ok := syncLeg(ctx, c, l)
			out[i] = SyncResult{Leg: l, OK: ok, Message: msg}
		})
	}
	wg.Wait()
	return out
}

// syncLeg rebases only a clean checkout of the branch, and aborts a conflicted rebase so the
// leg is left exactly as it was.
func syncLeg(ctx context.Context, c *change.Change, l *change.Leg) (string, bool) {
	dir := l.Dir()
	if gitx.HasRemote(ctx, dir, "origin") {
		if err := gitx.Fetch(ctx, dir); err != nil {
			return FirstLine(err.Error()), false
		}
	}
	st, err := gitx.StatusOf(ctx, dir, c.Branch, l.BaseRef)
	if err != nil {
		return FirstLine(err.Error()), false
	}
	if st.Current != c.Branch {
		return "skipped: on " + orDetached(st.Current) + ", not " + c.Branch + " (switch to rebase it)", false
	}
	if st.Dirty > 0 {
		return fmt.Sprintf("skipped: %d uncommitted files", st.Dirty), false
	}
	if st.Behind == 0 {
		return "up to date with " + l.BaseRef, true
	}
	if _, err := gitx.Run(ctx, dir, "rebase", "--quiet", l.BaseRef); err != nil {
		gitx.Run(ctx, dir, "rebase", "--abort")
		return "conflict rebasing onto " + l.BaseRef + ": left unchanged, rebase by hand", false
	}
	msg := fmt.Sprintf("rebased onto %s (%d new commits)", l.BaseRef, st.Behind)
	if st.Ahead > 0 && gitx.RefExists(ctx, dir, "refs/remotes/origin/"+c.Branch) {
		msg += " · already pushed: next publish needs force-with-lease"
	}
	return msg, true
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
