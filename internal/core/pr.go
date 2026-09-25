package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/prbody"
)

type PRAction int

const (
	PRSkip PRAction = iota
	PRCreate
	PRUpdate
)

type PRPlan struct {
	Leg    *change.Leg
	Level  int
	State  LegState
	Action PRAction
	// Ready marks an open draft PR ready for review once its body is updated.
	Ready bool
	Note  string
	Err   error
}

type PROptions struct {
	Draft          bool
	Ready          bool
	ForceWithLease bool
}

// PRTitle is title, else the change's title, prefixed with the change ID.
func PRTitle(c *change.Change, title string) (string, error) {
	if title == "" {
		title = c.Title
	}
	if title == "" {
		return "", fmt.Errorf("no title: give the change one, e.g. td start %s --title", c.ID)
	}
	if !strings.HasPrefix(title, c.ID) {
		title = c.ID + " " + title
	}
	return title, nil
}

// PlanPRs reads every leg's state and decides, per leg, whether to open a PR,
// refresh an existing one, or leave it. Plans come back in merge order.
func PlanPRs(ctx context.Context, c *change.Change, g Graph, o PROptions) []*PRPlan {
	states := Snapshot(ctx, c, true)
	SortByLevel(states, g.Levels)
	plans := make([]*PRPlan, len(states))
	for i, s := range states {
		p := &PRPlan{Leg: s.Leg, Level: g.Levels[s.Leg.Repo], State: s}
		switch {
		case s.StatusErr != nil:
			p.Err = s.StatusErr
		case s.PRErr != nil:
			p.Err = s.PRErr
		case s.PR != nil && s.PR.State == "MERGED":
			p.Note = "already merged"
		case s.PR != nil && s.PR.State == "OPEN":
			p.Action, p.Note = PRUpdate, "push, refresh related PRs on #"+strconv.Itoa(s.PR.Number)
			if o.Ready && s.PR.IsDraft {
				p.Ready = true
				p.Note += ", mark ready for review"
			}
		case s.Status.Ahead == 0:
			p.Note = "nothing to open: no commits ahead of " + s.Leg.BaseRef
		default:
			p.Action, p.Note = PRCreate, fmt.Sprintf("push %d commits, open PR into %s", s.Status.Ahead, s.Leg.Base)
			if o.Draft {
				p.Note += " as draft"
			}
		}
		plans[i] = p
	}
	return plans
}

// PublishPRs carries out the plans: push each acting leg, open missing PRs,
// then rewrite the Related PRs block in every open PR. Failures land in each
// plan's Err; the caller saves c to keep new PR numbers.
func PublishPRs(ctx context.Context, c *change.Change, plans []*PRPlan, title string, o PROptions) {
	var wg sync.WaitGroup
	for _, p := range plans {
		if p.Action == PRSkip || p.Err != nil {
			continue
		}
		wg.Go(func() { publish(ctx, c, p, title, o) })
	}
	wg.Wait()

	var entries []prbody.Entry
	for _, p := range plans {
		if p.State.PR != nil && p.State.PR.State != "CLOSED" {
			entries = append(entries, prbody.Entry{Level: p.Level, Ref: gh.Ref(p.State.PR.URL)})
		}
	}
	block := prbody.Block(entries)
	for _, p := range plans {
		if p.Action == PRSkip || p.Err != nil || p.State.PR == nil || p.State.PR.State != "OPEN" {
			continue
		}
		wg.Go(func() {
			updated := prbody.Apply(p.State.PR.Body, block)
			if updated == p.State.PR.Body {
				return
			}
			if err := gh.EditBody(ctx, p.Leg.Dir(), p.State.PR.Number, updated); err != nil {
				p.Err = err
				return
			}
			p.State.PR.Body = updated
		})
	}
	wg.Wait()

	for _, p := range plans {
		if !p.Ready || p.Err != nil {
			continue
		}
		wg.Go(func() {
			if err := gh.Ready(ctx, p.Leg.Dir(), p.State.PR.Number); err != nil {
				p.Err = err
				return
			}
			p.State.PR.IsDraft = false
		})
	}
	wg.Wait()
}

func publish(ctx context.Context, c *change.Change, p *PRPlan, title string, o PROptions) {
	push := []string{"push", "--quiet", "-u", "origin", c.Branch}
	if o.ForceWithLease {
		push = append(push, "--force-with-lease")
	}
	if _, err := gitx.Run(ctx, p.Leg.Dir(), push...); err != nil {
		if strings.Contains(err.Error(), "non-fast-forward") || strings.Contains(err.Error(), "fetch first") {
			err = fmt.Errorf("push rejected: branch diverged from origin (after a sync, publish with force-with-lease)")
		}
		p.Err = err
		return
	}
	if p.Action != PRCreate {
		return
	}
	n, u, err := gh.Create(ctx, p.Leg.Dir(), gh.CreateOpts{
		Base: p.Leg.Base, Head: c.Branch, Title: title, Body: c.Body, Draft: o.Draft, Reviewers: c.Reviewers,
	})
	if err != nil {
		p.Err = err
		return
	}
	p.Leg.PR, p.Leg.PRURL = n, u
	p.State.PR = &gh.PR{Number: n, URL: u, State: "OPEN", Body: c.Body, IsDraft: o.Draft}
}
