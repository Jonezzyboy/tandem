package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gitx"
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
	if err := change.ValidateID(id); err != nil {
		return err
	}
	c, err := e.store.Load(id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c = &change.Change{ID: id, Branch: id, Created: time.Now().UTC()}
	case err != nil:
		return err
	}
	if *title != "" {
		c.Title = *title
	}

	names := map[string]string{}
	for _, l := range c.Legs {
		names[l.Name()] = l.Repo
	}
	var repos []workspace.Repo
	for _, a := range pos[1:] {
		r, err := workspace.Resolve(e.roots, a)
		if err != nil {
			return err
		}
		short := filepath.Base(r.Name)
		if prev, ok := names[short]; ok {
			if prev == r.Name {
				fmt.Fprintf(e.out, "  %s %s already in %s\n", e.ui.Dim("•"), r.Name, id)
				continue
			}
			return fmt.Errorf("%s and %s share the name %s; one change can hold only one of them", prev, r.Name, short)
		}
		names[short] = r.Name
		repos = append(repos, r)
	}

	type result struct {
		leg  change.Leg
		warn string
		err  error
	}
	results := make([]result, len(repos))
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			leg, warn, err := addLeg(ctx, e.store.Dir(id), c.Branch, r)
			results[i] = result{leg, warn, err}
		})
	}
	wg.Wait()

	failed := 0
	rows := [][]ui.Cell{}
	for i, res := range results {
		if res.err != nil {
			failed++
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(repos[i].Name), {Text: res.err.Error(), Color: e.ui.Orange}})
			continue
		}
		note := "worktree " + res.leg.Worktree
		if res.warn != "" {
			note += " (" + res.warn + ")"
		}
		rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(res.leg.Repo), ui.Plain(note)})
		c.Legs = append(c.Legs, res.leg)
	}
	if c.Title != "" {
		fmt.Fprintf(e.out, "%s  %s\n", e.ui.Bold(c.ID), c.Title)
	} else {
		fmt.Fprintln(e.out, e.ui.Bold(c.ID))
	}
	e.ui.Table(e.out, "  ", rows)
	if len(c.Legs) > 0 {
		if err := e.store.Save(c); err != nil {
			return err
		}
		printEdges(e, c)
	}
	if failed > 0 {
		return errReported
	}
	return nil
}

func addLeg(ctx context.Context, dir, branch string, r workspace.Repo) (change.Leg, string, error) {
	var warn string
	if gitx.HasRemote(ctx, r.Path, "origin") {
		if err := gitx.Fetch(ctx, r.Path); err != nil {
			warn = "fetch failed, used local refs"
		}
	}
	base, err := gitx.DefaultBase(ctx, r.Path)
	if err != nil {
		return change.Leg{}, "", err
	}
	dst := filepath.Join(dir, filepath.Base(r.Name))
	if _, err := workspace.AddWorktree(ctx, r.Path, dst, branch, base); err != nil {
		return change.Leg{}, "", err
	}
	return change.Leg{Repo: r.Name, Source: r.Path, Worktree: dst, Base: base.Branch, BaseRef: base.Ref}, warn, nil
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
