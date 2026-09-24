package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/ui"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

func runStart(ctx context.Context, e *env, args []string) error {
	flags := newFlags("start", "td start <ID> <repo>... [--title T]")
	title := flags.String("title", "", "change title, used for PR titles")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		flags.Usage()
		return errReported
	}
	id := pos[0]
	var repos []workspace.Repo
	for _, a := range pos[1:] {
		r, err := workspace.Resolve(e.roots, a)
		if err != nil {
			return err
		}
		repos = append(repos, r)
	}
	c, results, err := core.Start(ctx, e.store, id, *title, repos)
	if err != nil {
		return err
	}

	failed := 0
	rows := [][]ui.Cell{}
	for _, res := range results {
		switch {
		case res.Existing:
			rows = append(rows, []ui.Cell{{Text: "•", Color: e.ui.Dim}, ui.Plain(res.Repo.Name), {Text: "already in " + id, Color: e.ui.Dim}})
		case res.Err != nil:
			failed++
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(res.Repo.Name), {Text: res.Err.Error(), Color: e.ui.Orange}})
		default:
			note := "worktree " + res.Leg.Worktree
			if res.Warning != "" {
				note += " (" + res.Warning + ")"
			}
			rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(res.Leg.Repo), ui.Plain(note)})
		}
	}
	if c.Title != "" {
		fmt.Fprintf(e.out, "%s  %s\n", e.ui.Bold(c.ID), c.Title)
	} else {
		fmt.Fprintln(e.out, e.ui.Bold(c.ID))
	}
	e.ui.Table(e.out, "  ", rows)
	if len(c.Legs) > 0 {
		printEdges(e, c)
	}
	if failed > 0 {
		return errReported
	}
	return nil
}

func printEdges(e *env, c *change.Change) {
	g, err := core.BuildGraph(c)
	for _, w := range g.Warnings {
		fmt.Fprintf(e.out, "  %s %v\n", e.ui.Orange("warning"), w)
	}
	if err != nil {
		fmt.Fprintf(e.out, "  %s %v\n", e.ui.Orange("graph"), err)
		return
	}
	downstream := map[string][]string{}
	for _, edge := range g.Edges {
		d := filepath.Base(edge.To)
		if !slices.Contains(downstream[edge.From], d) {
			downstream[edge.From] = append(downstream[edge.From], d)
		}
	}
	if len(downstream) == 0 {
		fmt.Fprintln(e.out, e.ui.Dim("  edges    none found: legs can merge in any order (declare one with td link)"))
		return
	}
	var parts []string
	for from, tos := range downstream {
		sort.Strings(tos)
		parts = append(parts, filepath.Base(from)+" → "+strings.Join(tos, ", "))
	}
	sort.Strings(parts)
	fmt.Fprintln(e.out, e.ui.Dim("  edges    "+strings.Join(parts, " · ")))
}
