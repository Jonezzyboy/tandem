package cli

import (
	"context"

	"github.com/jonezzyboy/tandem/internal/core"
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
	failed := false
	rows := [][]ui.Cell{}
	for _, r := range core.Sync(ctx, c, core.OrderedLegs(c, g.Levels)) {
		mark := ui.Cell{Text: "✓", Color: e.ui.Green}
		if !r.OK {
			mark = ui.Cell{Text: "✗", Color: e.ui.Orange}
			failed = true
		}
		rows = append(rows, []ui.Cell{mark, ui.Plain(r.Leg.Name()), ui.Plain(r.Message)})
	}
	e.ui.Table(e.out, "  ", rows)
	if failed {
		return errReported
	}
	return nil
}
