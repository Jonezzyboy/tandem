package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/ui"
	"golang.org/x/term"
)

// confirm asks on the terminal; without one, only --yes can confirm.
func (e *env) confirm(question string, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, errors.New("stdin is not a terminal: pass --yes to confirm")
	}
	fmt.Fprint(e.out, question+" [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func runPin(ctx context.Context, e *env, args []string) error {
	flags := newFlags("pin", "td pin [ID] [--no-commit]")
	noCommit := flags.Bool("no-commit", false, "update go.mod/go.sum without committing")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, _, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return err
	}
	results := core.Pin(ctx, c, g, !*noCommit)
	if len(results) == 0 {
		fmt.Fprintln(e.out, e.ui.Dim("no Go dependencies between legs: nothing to pin"))
		return nil
	}
	failed := false
	rows := [][]ui.Cell{}
	for _, r := range results {
		target := r.Downstream.Name() + " ← " + r.Module
		switch {
		case r.Skipped != "":
			failed = true
			rows = append(rows, []ui.Cell{{Text: "–", Color: e.ui.Orange}, ui.Plain(target), {Text: r.Skipped, Color: e.ui.Orange}})
		case r.Err != nil:
			failed = true
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(target), {Text: core.FirstLine(r.Err.Error()), Color: e.ui.Orange}})
		case !r.Changed:
			rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(target), {Text: "already at " + shortSHA(r.Rev), Color: e.ui.Dim}})
		default:
			note := "pinned to " + shortSHA(r.Rev)
			if r.Committed {
				note += ", committed"
			} else {
				note += ", not committed"
			}
			rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(target), ui.Plain(note)})
		}
	}
	e.ui.Table(e.out, "  ", rows)
	if failed {
		return errReported
	}
	return nil
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func runMerge(ctx context.Context, e *env, args []string) error {
	flags := newFlags("merge", "td merge [ID] [--method squash|merge|rebase] [--dry-run] [--yes]")
	method := flags.String("method", "squash", "how GitHub merges each PR: squash, merge or rebase")
	dryRun := flags.Bool("dry-run", false, "show the train's plan and stop")
	yes := flags.Bool("yes", false, "merge without asking")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, _, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return err
	}
	checks := core.TrainPreflight(ctx, c, g)
	blocked, toMerge := 0, 0
	rows := [][]ui.Cell{}
	for _, tc := range checks {
		pr := "—"
		if tc.PR != nil {
			pr = "#" + strconv.Itoa(tc.PR.Number)
		}
		status := ui.Cell{Text: "ready", Color: e.ui.Green}
		switch {
		case len(tc.Problems) > 0:
			blocked++
			status = ui.Cell{Text: strings.Join(tc.Problems, "; "), Color: e.ui.Orange}
		case tc.Merged:
			status = ui.Cell{Text: "already merged", Color: e.ui.Dim}
		default:
			toMerge++
		}
		rows = append(rows, []ui.Cell{ui.Plain(strconv.Itoa(tc.Level)), ui.Plain(tc.Leg.Name()), ui.Plain(pr), status})
	}
	fmt.Fprintf(e.out, "%s  merge train\n", e.ui.Bold(c.ID))
	e.ui.Table(e.out, "  ", rows)
	if blocked > 0 {
		fmt.Fprintf(e.out, "%s\n", e.ui.Orange(fmt.Sprintf("  %d of %d legs blocked: fix them, then rerun", blocked, len(checks))))
		return errReported
	}
	if toMerge == 0 {
		fmt.Fprintln(e.out, "  every leg is already merged")
		return nil
	}
	fmt.Fprintln(e.out, e.ui.Dim("  after each level merges, downstream Go legs are re-pinned to its merge commit, pushed, and wait for CI"))
	if *dryRun {
		return nil
	}
	ok, err := e.confirm(fmt.Sprintf("Merge %d PRs in this order with --%s?", toMerge, *method), *yes)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(e.out, "nothing merged")
		return nil
	}
	err = core.RunTrain(ctx, c, g, core.TrainOptions{Method: *method, OnEvent: func(ev core.TrainEvent) {
		if ev.Phase == "done" {
			fmt.Fprintln(e.out, e.ui.Green("  ✓ "+ev.Detail))
			return
		}
		color := e.ui.Dim
		switch ev.Phase {
		case "merged", "pinned":
			color = e.ui.Green
		case "merging":
			color = e.ui.Blue
		}
		fmt.Fprintf(e.out, "  %d  %-20s %s  %s\n", ev.Level, ev.Leg, color(fmt.Sprintf("%-8s", ev.Phase)), ev.Detail)
	}})
	if saveErr := e.store.Save(c); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		fmt.Fprintf(e.out, "%s %v\n%s\n", e.ui.Orange("  ✗"), err, e.ui.Dim("  fix it and rerun td merge: merged legs are skipped"))
		return errReported
	}
	return nil
}

func runClean(ctx context.Context, e *env, args []string) error {
	flags := newFlags("clean", "td clean [--yes]")
	yes := flags.Bool("yes", false, "delete without asking")
	if _, err := parseArgs(flags, args); err != nil {
		return err
	}
	cands, err := core.CleanCandidates(ctx, e.store)
	if err != nil {
		return err
	}
	var ready []core.CleanCandidate
	for _, cc := range cands {
		if !cc.Ready {
			fmt.Fprintf(e.out, "%s  %s\n", e.ui.Dim(cc.Change.ID), e.ui.Dim("kept: "+cc.Reason))
			continue
		}
		ready = append(ready, cc)
		fmt.Fprintf(e.out, "%s  %s\n", e.ui.Bold(cc.Change.ID), cc.Change.Title)
		for _, w := range cc.Worktrees {
			fmt.Fprintf(e.out, "  remove worktree  %s\n", w.Path)
		}
		for _, b := range cc.Branches {
			fmt.Fprintf(e.out, "  delete branch    %s in %s\n", b.Branch, b.Source)
		}
		for _, b := range cc.Kept {
			fmt.Fprintf(e.out, "  %s\n", e.ui.Dim("keep branch      "+b.Branch+" in "+b.Source+" (PR closed unmerged)"))
		}
		for _, f := range cc.Files {
			fmt.Fprintf(e.out, "  delete file      %s\n", f)
		}
		fmt.Fprintf(e.out, "  remove dir       %s (only if empty)\n", cc.Dir)
	}
	if len(ready) == 0 {
		fmt.Fprintln(e.out, "nothing to clean")
		return nil
	}
	ok, err := e.confirm(fmt.Sprintf("Delete everything listed for %d changes?", len(ready)), *yes)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(e.out, "nothing deleted")
		return nil
	}
	failed := false
	for _, cc := range ready {
		if err := core.Clean(ctx, cc); err != nil {
			failed = true
			fmt.Fprintf(e.out, "%s %s: %v\n", e.ui.Orange("✗"), cc.Change.ID, err)
			continue
		}
		fmt.Fprintf(e.out, "%s %s cleaned\n", e.ui.Green("✓"), cc.Change.ID)
	}
	if failed {
		return errReported
	}
	return nil
}
