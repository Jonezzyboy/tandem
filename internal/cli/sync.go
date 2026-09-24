package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/ui"
)

func runSync(ctx context.Context, e *env, args []string) error {
	flags := newFlags("sync", "td sync [ID]")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, _, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	g, _ := core.BuildGraph(c)
	legs := orderedLegs(c, g.Levels)

	type result struct {
		msg string
		ok  bool
	}
	results := make([]result, len(legs))
	var wg sync.WaitGroup
	for i, l := range legs {
		wg.Go(func() {
			msg, ok := syncLeg(ctx, c, l)
			results[i] = result{msg, ok}
		})
	}
	wg.Wait()

	failed := false
	rows := [][]ui.Cell{}
	for i, r := range results {
		mark := ui.Cell{Text: "✓", Color: e.ui.Green}
		if !r.ok {
			mark = ui.Cell{Text: "✗", Color: e.ui.Orange}
			failed = true
		}
		rows = append(rows, []ui.Cell{mark, ui.Plain(legs[i].Name()), ui.Plain(r.msg)})
	}
	e.ui.Table(e.out, "  ", rows)
	if failed {
		return errReported
	}
	return nil
}

// syncLeg rebases only a clean worktree, and aborts a conflicted rebase so the
// leg is left exactly as it was.
func syncLeg(ctx context.Context, c *change.Change, l *change.Leg) (string, bool) {
	if gitx.HasRemote(ctx, l.Worktree, "origin") {
		if err := gitx.Fetch(ctx, l.Worktree); err != nil {
			return firstLine(err.Error()), false
		}
	}
	st, err := gitx.StatusOf(ctx, l.Worktree, l.BaseRef)
	if err != nil {
		return firstLine(err.Error()), false
	}
	if st.Dirty > 0 {
		return fmt.Sprintf("skipped: %d uncommitted files", st.Dirty), false
	}
	if st.Behind == 0 {
		return "up to date with " + l.BaseRef, true
	}
	if _, err := gitx.Run(ctx, l.Worktree, "rebase", "--quiet", l.BaseRef); err != nil {
		gitx.Run(ctx, l.Worktree, "rebase", "--abort")
		return "conflict rebasing onto " + l.BaseRef + ": left unchanged, rebase by hand", false
	}
	msg := fmt.Sprintf("rebased onto %s (%d new commits)", l.BaseRef, st.Behind)
	if st.Ahead > 0 && gitx.RefExists(ctx, l.Worktree, "refs/remotes/origin/"+c.Branch) {
		msg += " · already pushed: publish with td pr --force-with-lease"
	}
	return msg, true
}
