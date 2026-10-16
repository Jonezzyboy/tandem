package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/jira"
)

// moveTicket moves the change's Jira ticket to status, reporting the result
// in the change's activity. A change without a ticket, or a status of "",
// does nothing.
func (a *App) moveTicket(id, status string) {
	a.moveTicketWith(a.jiraClient(), id, status)
}

func (a *App) moveTicketWith(client jira.Client, id, status string) {
	if status == "" {
		return
	}
	c, err := a.store.Load(id)
	if err != nil {
		return
	}
	r, ok := jira.Parse(c.Ticket)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	is, err := client.Issue(ctx, r)
	if err != nil {
		a.activity(id, fmt.Sprintf("Jira: couldn't read %s to move it: %s", r.Key, core.FirstLine(err.Error())), "warn")
		return
	}
	if strings.EqualFold(is.Status, status) {
		return
	}
	ts, err := client.Transitions(ctx, r)
	if err != nil {
		a.activity(id, fmt.Sprintf("Jira: couldn't move %s: %s", r.Key, core.FirstLine(err.Error())), "warn")
		return
	}
	i := slices.IndexFunc(ts, func(t jira.Transition) bool { return strings.EqualFold(t.To, status) })
	if i < 0 {
		a.activity(id, fmt.Sprintf("Jira: %s can't go from %s to %s in its workflow", r.Key, is.Status, status), "warn")
		return
	}
	if err := client.Transition(ctx, r, ts[i].ID); err != nil {
		a.activity(id, fmt.Sprintf("Jira: couldn't move %s: %s", r.Key, core.FirstLine(err.Error())), "warn")
		return
	}
	a.activity(id, fmt.Sprintf("Jira: moved %s from %s to %s", r.Key, is.Status, ts[i].To), "ok")
}

// openedForReview reports whether publishing opened a PR or took one out of
// draft, which is when the ticket moves on.
func openedForReview(plans []*core.PRPlan) bool {
	return slices.ContainsFunc(plans, func(p *core.PRPlan) bool {
		return p.Err == nil && (p.Action == core.PRCreate || p.Ready)
	})
}
