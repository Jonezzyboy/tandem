package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/ui"
)

func runLink(ctx context.Context, e *env, args []string) error {
	flags := newFlags("link", "td link [ID] <upstream> <downstream>")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, rest, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		flags.Usage()
		return errReported
	}
	up, err := c.Leg(rest[0])
	if err != nil {
		return err
	}
	down, err := c.Leg(rest[1])
	if err != nil {
		return err
	}
	c.AddDeclared(change.Edge{From: up.Repo, To: down.Repo})
	if _, err := core.BuildGraph(c); err != nil {
		return err
	}
	if err := e.store.Save(c); err != nil {
		return err
	}
	printEdges(e, c)
	return nil
}

func runPath(ctx context.Context, e *env, args []string) error {
	flags := newFlags("path", "td path [ID] [leg]")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, rest, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	switch len(rest) {
	case 0:
		fmt.Fprintln(e.out, e.store.Dir(c.ID))
	case 1:
		l, err := c.Leg(rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(e.out, l.Worktree)
	default:
		flags.Usage()
		return errReported
	}
	return nil
}

func runList(ctx context.Context, e *env, args []string) error {
	flags := newFlags("list", "td list")
	if _, err := parseArgs(flags, args); err != nil {
		return err
	}
	all, err := e.store.List()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(e.out, "no changes yet: run td start <ID> <repo>...")
		return nil
	}
	rows := [][]ui.Cell{}
	for _, c := range all {
		rows = append(rows, []ui.Cell{
			{Text: c.ID, Color: e.ui.Bold}, ui.Plain(strconv.Itoa(len(c.Legs)) + " legs"), ui.Plain(c.Title),
		})
	}
	e.ui.Table(e.out, "", rows)
	return nil
}
