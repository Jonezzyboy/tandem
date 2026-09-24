package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/ui"
)

func runStatus(ctx context.Context, e *env, args []string) error {
	flags := newFlags("status", "td status [ID]")
	offline := flags.Bool("offline", false, "skip GitHub, show local state only")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, _, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	g, graphErr := core.BuildGraph(c)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	states := core.Snapshot(ctx, c, !*offline)
	if err := e.store.Save(c); err != nil {
		return err
	}
	core.SortByLevel(states, g.Levels)

	blockers := 0
	for _, s := range states {
		if !*offline && len(s.Blockers()) > 0 {
			blockers++
		}
	}
	summary := e.ui.Green("ready to merge")
	switch {
	case *offline:
		summary = e.ui.Dim("offline")
	case blockers > 0:
		summary = e.ui.Orange(fmt.Sprintf("%d of %d legs blocked", blockers, len(states)))
	}
	fmt.Fprintf(e.out, "%s  %s  %s\n", e.ui.Bold(c.ID), c.Title, summary)

	rows := [][]ui.Cell{{
		{Text: "#", Color: e.ui.Dim}, {Text: "LEG", Color: e.ui.Dim}, {Text: "LOCAL", Color: e.ui.Dim},
		{Text: "PR", Color: e.ui.Dim}, {Text: "CHECKS", Color: e.ui.Dim}, {Text: "REVIEW", Color: e.ui.Dim},
	}}
	for _, s := range states {
		level := "?"
		if n, ok := g.Levels[s.Leg.Repo]; ok {
			level = strconv.Itoa(n)
		}
		row := []ui.Cell{ui.Plain(level), ui.Plain(s.Leg.Name()), localCell(e, s.Status, s.StatusErr)}
		if !*offline {
			row = append(row, prCells(e, s.PR, s.PRErr)...)
		}
		rows = append(rows, row)
	}
	if *offline {
		rows[0] = rows[0][:3]
	}
	e.ui.Table(e.out, "  ", rows)
	if graphErr != nil {
		fmt.Fprintf(e.out, "  %s %v\n", e.ui.Orange("graph"), graphErr)
	}
	return nil
}

func localCell(e *env, st gitx.Status, err error) ui.Cell {
	if err != nil {
		return ui.Cell{Text: "error: " + core.FirstLine(err.Error()), Color: e.ui.Orange}
	}
	var parts []string
	if st.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", st.Ahead))
	}
	if st.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", st.Behind))
	}
	if st.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d dirty", st.Dirty))
		return ui.Cell{Text: strings.Join(parts, " "), Color: e.ui.Orange}
	}
	if len(parts) == 0 {
		return ui.Cell{Text: "no commits", Color: e.ui.Dim}
	}
	return ui.Plain(strings.Join(append(parts, "clean"), " "))
}

func prCells(e *env, pr *gh.PR, err error) []ui.Cell {
	if err != nil {
		return []ui.Cell{{Text: "error: " + core.FirstLine(err.Error()), Color: e.ui.Orange}}
	}
	if pr == nil {
		return []ui.Cell{{Text: "—", Color: e.ui.Dim}, {Text: "—", Color: e.ui.Dim}, {Text: "—", Color: e.ui.Dim}}
	}
	num := ui.Plain("#" + strconv.Itoa(pr.Number))
	if pr.IsDraft {
		num.Text += " draft"
	}
	if pr.State != "OPEN" {
		return []ui.Cell{num, {Text: strings.ToLower(pr.State), Color: e.ui.Dim}, ui.Plain("")}
	}

	r := pr.Rollup()
	total := r.Pass + r.Fail + r.Pending
	var checks ui.Cell
	switch {
	case total == 0:
		checks = ui.Cell{Text: "none", Color: e.ui.Dim}
	case r.Fail > 0:
		checks = ui.Cell{Text: "✗ " + strings.Join(r.Failing, ", "), Color: e.ui.Orange}
	case r.Pending > 0:
		checks = ui.Plain(fmt.Sprintf("◌ %d/%d running", r.Pending, total))
	default:
		checks = ui.Cell{Text: fmt.Sprintf("✓ %d/%d", r.Pass, total), Color: e.ui.Green}
	}

	var review ui.Cell
	switch pr.ReviewDecision {
	case "APPROVED":
		review = ui.Cell{Text: "approved", Color: e.ui.Green}
	case "CHANGES_REQUESTED":
		review = ui.Cell{Text: "changes requested", Color: e.ui.Orange}
	case "REVIEW_REQUIRED":
		review = ui.Plain("awaiting review")
	default:
		review = ui.Cell{Text: "—", Color: e.ui.Dim}
	}
	return []ui.Cell{num, checks, review}
}
