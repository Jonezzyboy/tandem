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
	flags := newFlags("start", "td start <ID> <repo>... [--title T] [--worktree]")
	title := flags.String("title", "", "change title, used for PR titles")
	worktree := flags.Bool("worktree", false, "give each repo a worktree under the change instead of checking the branch out in its clone")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		flags.Usage()
		return errReported
	}
	id := pos[0]
	repos, err := resolveRepos(e, pos[1:])
	if err != nil {
		return err
	}
	c, results, err := core.Start(ctx, e.store, id, *title, repos, core.StartOptions{Worktrees: *worktree})
	if err != nil {
		return err
	}
	if *worktree && !c.Worktrees {
		fmt.Fprintln(e.out, e.ui.Orange("  "+c.ID+" already uses branches in the clones; --worktree only applies when a change is created"))
	}
	return printStart(e, c, results)
}

// runAdd puts more repos into an existing change: each gets the change's
// branch, and the merge order is recomputed with them in it.
func runAdd(ctx context.Context, e *env, args []string) error {
	flags := newFlags("add", "td add [ID] <repo>...")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, rest, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		flags.Usage()
		return errReported
	}
	repos, err := resolveRepos(e, rest)
	if err != nil {
		return err
	}
	c, results, err := core.Start(ctx, e.store, c.ID, "", repos, core.StartOptions{})
	if err != nil {
		return err
	}
	if err := printStart(e, c, results); err != nil {
		return err
	}
	for _, r := range results {
		if !r.Existing && r.Err == nil {
			fmt.Fprintln(e.out, e.ui.Dim("  run td pr to open its PR and refresh the merge order in the others"))
			break
		}
	}
	return nil
}

func resolveRepos(e *env, names []string) ([]workspace.Repo, error) {
	var repos []workspace.Repo
	for _, a := range names {
		r, err := workspace.Resolve(e.roots, a)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

func printStart(e *env, c *change.Change, results []core.AddResult) error {
	failed := 0
	rows := [][]ui.Cell{}
	for _, res := range results {
		switch {
		case res.Existing:
			rows = append(rows, []ui.Cell{{Text: "•", Color: e.ui.Dim}, ui.Plain(res.Repo.Name), {Text: "already in " + c.ID, Color: e.ui.Dim}})
		case res.Err != nil:
			failed++
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(res.Repo.Name), {Text: res.Err.Error(), Color: e.ui.Orange}})
		default:
			note := "branch " + c.Branch
			if !res.Created {
				note += " (existing)"
			}
			switch {
			case res.Leg.Worktree != "":
				note += ", worktree " + res.Leg.Worktree
			case res.Switched:
				note += ", checked out"
			}
			cells := []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(res.Leg.Repo), ui.Plain(note)}
			if res.Warning != "" {
				cells[0] = ui.Cell{Text: "!", Color: e.ui.Orange}
				cells = append(cells, ui.Cell{Text: res.Warning, Color: e.ui.Orange})
			}
			rows = append(rows, cells)
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
