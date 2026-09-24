package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/checks"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/ui"
)

func runCheck(ctx context.Context, e *env, args []string) error {
	flags := newFlags("check", "td check [ID] [leg...]")
	pos, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	c, rest, err := e.changeFrom(pos)
	if err != nil {
		return err
	}
	var legs []*change.Leg
	if len(rest) == 0 {
		g, _ := core.BuildGraph(c)
		legs = orderedLegs(c, g.Levels)
	}
	for _, name := range rest {
		l, err := c.Leg(name)
		if err != nil {
			return err
		}
		legs = append(legs, l)
	}

	// Legs run concurrently; each prints as a block once done so output never interleaves.
	var mu sync.Mutex
	var wg sync.WaitGroup
	failed := false
	for _, l := range legs {
		wg.Go(func() {
			var b strings.Builder
			ok := checkLeg(ctx, e, l, &b)
			mu.Lock()
			defer mu.Unlock()
			fmt.Fprint(e.out, b.String())
			if !ok {
				failed = true
			}
		})
	}
	wg.Wait()
	if failed {
		return errReported
	}
	return nil
}

func checkLeg(ctx context.Context, e *env, l *change.Leg, b *strings.Builder) bool {
	fmt.Fprintln(b, e.ui.Bold(l.Name()))
	list, err := checks.Detect(l.Worktree)
	if err != nil {
		fmt.Fprintf(b, "  %s %v\n", e.ui.Orange("✗"), err)
		return false
	}
	if len(list) == 0 {
		fmt.Fprintln(b, e.ui.Dim("  no checks found: add them to .tandem.yml"))
		return true
	}
	ok := true
	rows := [][]ui.Cell{}
	var failures []checks.Result
	for _, c := range list {
		if c.Skip != "" {
			rows = append(rows, []ui.Cell{{Text: "–", Color: e.ui.Dim}, ui.Plain(c.Name), {Text: "skipped: " + c.Skip, Color: e.ui.Dim}})
			continue
		}
		r := checks.Run(ctx, c)
		dur := r.Duration.Round(100 * time.Millisecond).String()
		if r.Err != nil {
			ok = false
			failures = append(failures, r)
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(c.Name), {Text: dur, Color: e.ui.Orange}})
			continue
		}
		rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(c.Name), {Text: dur, Color: e.ui.Dim}})
	}
	e.ui.Table(b, "  ", rows)
	for _, f := range failures {
		fmt.Fprintf(b, "  %s\n", e.ui.Orange(f.Check.Name))
		for line := range strings.SplitSeq(f.Output, "\n") {
			fmt.Fprintf(b, "    %s\n", e.ui.Dim(line))
		}
	}
	return ok
}
