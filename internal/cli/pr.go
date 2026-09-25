package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/ui"
)

func runPR(ctx context.Context, e *env, args []string) error {
	flags := newFlags("pr", "td pr [ID] [--title T] [--body B | --body-file F] [--draft] [--ready] [--reviewer a,b] [--dry-run]")
	title := flags.String("title", "", "PR title (default: the change title, prefixed with its ID)")
	body := flags.String("body", "", "shared PR description")
	bodyFile := flags.String("body-file", "", "read the shared description from a file, - for stdin")
	draft := flags.Bool("draft", false, "open new PRs as drafts")
	ready := flags.Bool("ready", false, "mark open draft PRs ready for review")
	reviewers := flags.String("reviewer", "", "comma-separated reviewers for new PRs")
	dryRun := flags.Bool("dry-run", false, "show what would happen without pushing or touching GitHub")
	forceLease := flags.Bool("force-with-lease", false, "push with --force-with-lease, for branches rebased by td sync")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, _, err := e.changeFrom(pos)
	if err != nil {
		return err
	}

	if *bodyFile != "" {
		var data []byte
		if *bodyFile == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(*bodyFile)
		}
		if err != nil {
			return err
		}
		*body = string(data)
	}
	if *body != "" {
		c.Body = *body
	}
	if *reviewers != "" {
		c.Reviewers = splitList(*reviewers)
	}
	t, err := core.PRTitle(c, *title)
	if err != nil {
		return err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return err
	}
	opts := core.PROptions{Draft: *draft, Ready: *ready, ForceWithLease: *forceLease}
	plans := core.PlanPRs(ctx, c, g, opts)

	fmt.Fprintf(e.out, "%s  %s\n", e.ui.Bold(c.ID), t)
	rows := [][]ui.Cell{}
	for _, p := range plans {
		note := ui.Cell{Text: p.Note}
		switch {
		case p.Err != nil:
			note = ui.Cell{Text: "error: " + core.FirstLine(p.Err.Error()), Color: e.ui.Orange}
		case p.Action == core.PRSkip:
			note.Color = e.ui.Dim
		case p.State.Status.Dirty > 0:
			note.Text += fmt.Sprintf(" · %d uncommitted files left out", p.State.Status.Dirty)
		}
		rows = append(rows, []ui.Cell{ui.Plain(strconv.Itoa(p.Level)), ui.Plain(p.Leg.Name()), note})
	}
	e.ui.Table(e.out, "  ", rows)
	if *dryRun {
		return nil
	}

	core.PublishPRs(ctx, c, plans, t, opts)
	if err := e.store.Save(c); err != nil {
		return err
	}

	fmt.Fprintln(e.out)
	failed := false
	rows = [][]ui.Cell{}
	for _, p := range plans {
		switch {
		case p.Err != nil:
			failed = true
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(p.Leg.Name()), {Text: "error: " + core.FirstLine(p.Err.Error()), Color: e.ui.Orange}})
		case p.State.PR != nil:
			rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(p.Leg.Name()), ui.Plain(p.State.PR.URL)})
		}
	}
	e.ui.Table(e.out, "  ", rows)
	if failed {
		return errReported
	}
	return nil
}
