package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/prbody"
	"github.com/jonezzyboy/tandem/internal/ui"
)

type prAction int

const (
	actSkip prAction = iota
	actCreate
	actUpdate
)

type prPlan struct {
	leg    *change.Leg
	state  core.LegState
	action prAction
	note   string
}

func runPR(ctx context.Context, e *env, args []string) error {
	flags := newFlags("pr", "td pr [ID] [--title T] [--body B | --body-file F] [--draft] [--reviewer a,b] [--dry-run]")
	title := flags.String("title", "", "PR title (default: the change title, prefixed with its ID)")
	body := flags.String("body", "", "shared PR description")
	bodyFile := flags.String("body-file", "", "read the shared description from a file, - for stdin")
	draft := flags.Bool("draft", false, "open new PRs as drafts")
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
	t := *title
	if t == "" {
		t = c.Title
	}
	if t == "" {
		return fmt.Errorf("no title: pass --title, or give the change one with td start %s --title", c.ID)
	}
	if !strings.HasPrefix(t, c.ID) {
		t = c.ID + " " + t
	}

	g, err := core.BuildGraph(c)
	if err != nil {
		return err
	}
	states := core.Snapshot(ctx, c, true)
	sortByLevel(states, g.Levels)

	plans := make([]*prPlan, len(states))
	for i, s := range states {
		p := &prPlan{leg: s.Leg, state: s}
		switch {
		case s.StatusErr != nil:
			p.note = "error: " + firstLine(s.StatusErr.Error())
		case s.PRErr != nil:
			p.note = "error: " + firstLine(s.PRErr.Error())
		case s.PR != nil && s.PR.State == "MERGED":
			p.note = "already merged"
		case s.PR != nil && s.PR.State == "OPEN":
			p.action, p.note = actUpdate, "push, refresh related PRs on #"+strconv.Itoa(s.PR.Number)
		case s.Status.Ahead == 0:
			p.note = "nothing to open: no commits ahead of " + s.Leg.BaseRef
		default:
			p.action, p.note = actCreate, fmt.Sprintf("push %d commits, open PR into %s", s.Status.Ahead, s.Leg.Base)
			if *draft {
				p.note += " as draft"
			}
		}
		if p.action != actSkip && s.Status.Dirty > 0 {
			p.note += e.ui.Orange(fmt.Sprintf(" · %d uncommitted files left out", s.Status.Dirty))
		}
		plans[i] = p
	}

	fmt.Fprintf(e.out, "%s  %s\n", e.ui.Bold(c.ID), t)
	printPlans(e, plans, g.Levels)
	if *dryRun {
		return nil
	}

	var wg sync.WaitGroup
	for _, p := range plans {
		if p.action == actSkip {
			continue
		}
		wg.Go(func() { publish(ctx, c, p, t, *draft, *forceLease) })
	}
	wg.Wait()

	var entries []prbody.Entry
	for _, p := range plans {
		if p.state.PR != nil && p.state.PR.State != "CLOSED" {
			entries = append(entries, prbody.Entry{Level: g.Levels[p.leg.Repo], Ref: gh.Ref(p.state.PR.URL)})
		}
	}
	block := prbody.Block(entries)
	for _, p := range plans {
		if p.action == actSkip || p.state.PR == nil || p.state.PR.State != "OPEN" {
			continue
		}
		wg.Go(func() {
			updated := prbody.Apply(p.state.PR.Body, block)
			if updated == p.state.PR.Body {
				return
			}
			if err := gh.EditBody(ctx, p.leg.Worktree, p.state.PR.Number, updated); err != nil {
				p.action, p.note = actSkip, "error: "+firstLine(err.Error())
			}
		})
	}
	wg.Wait()
	if err := e.store.Save(c); err != nil {
		return err
	}

	fmt.Fprintln(e.out)
	failed := false
	rows := [][]ui.Cell{}
	for _, p := range plans {
		switch {
		case strings.HasPrefix(p.note, "error: "):
			failed = true
			rows = append(rows, []ui.Cell{{Text: "✗", Color: e.ui.Orange}, ui.Plain(p.leg.Name()), {Text: p.note, Color: e.ui.Orange}})
		case p.state.PR != nil:
			rows = append(rows, []ui.Cell{{Text: "✓", Color: e.ui.Green}, ui.Plain(p.leg.Name()), ui.Plain(p.state.PR.URL)})
		}
	}
	e.ui.Table(e.out, "  ", rows)
	if failed {
		return errReported
	}
	return nil
}

func printPlans(e *env, plans []*prPlan, levels map[string]int) {
	rows := [][]ui.Cell{}
	for _, p := range plans {
		color := e.ui.Dim
		if p.action != actSkip {
			color = nil
		}
		if strings.HasPrefix(p.note, "error: ") {
			color = e.ui.Orange
		}
		rows = append(rows, []ui.Cell{ui.Plain(strconv.Itoa(levels[p.leg.Repo])), ui.Plain(p.leg.Name()), {Text: p.note, Color: color}})
	}
	e.ui.Table(e.out, "  ", rows)
}

// publish pushes the leg and opens its PR if needed, recording the outcome on p.
func publish(ctx context.Context, c *change.Change, p *prPlan, title string, draft, forceLease bool) {
	push := []string{"push", "--quiet", "-u", "origin", c.Branch}
	if forceLease {
		push = append(push, "--force-with-lease")
	}
	if _, err := gitx.Run(ctx, p.leg.Worktree, push...); err != nil {
		msg := firstLine(err.Error())
		if strings.Contains(err.Error(), "non-fast-forward") || strings.Contains(err.Error(), "fetch first") {
			msg = "push rejected: branch diverged from origin (after td sync, rerun with --force-with-lease)"
		}
		p.action, p.note = actSkip, "error: "+msg
		return
	}
	if p.action != actCreate {
		return
	}
	n, u, err := gh.Create(ctx, p.leg.Worktree, gh.CreateOpts{
		Base: p.leg.Base, Head: c.Branch, Title: title, Body: c.Body, Draft: draft, Reviewers: c.Reviewers,
	})
	if err != nil {
		p.action, p.note = actSkip, "error: "+firstLine(err.Error())
		return
	}
	p.leg.PR, p.leg.PRURL = n, u
	p.state.PR = &gh.PR{Number: n, URL: u, State: "OPEN", Body: c.Body, IsDraft: draft}
}
