package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

// DraftDescription writes a PR description to edit: why, from the Jira
// ticket's opening paragraph, and what changed, from each leg's commits.
func (a *App) DraftDescription(id string) (string, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	why := ""
	if c.Ticket != "" {
		if t, _ := a.JiraLookup(c.Ticket); t.Error == "" {
			why = leadParagraph(t.Description)
		}
	}
	g, _ := core.BuildGraph(c)
	var legs []legCommits
	for _, l := range core.OrderedLegs(c, g.Levels) {
		base := l.BaseRef
		if base == "" {
			base = "origin/" + l.Base
		}
		out, err := gitx.Run(ctx, l.Dir(), "log", "--no-merges", "--reverse", "--format=%s", base+".."+c.Branch)
		if err != nil {
			continue
		}
		legs = append(legs, legCommits{name: l.Name(), subjects: authored(strings.Split(out, "\n"))})
	}
	return draftBody(why, legs), nil
}

type legCommits struct {
	name     string
	subjects []string
}

func draftBody(why string, legs []legCommits) string {
	var b strings.Builder
	if why != "" {
		b.WriteString(why + "\n\n")
	}
	b.WriteString("### What changed\n")
	for _, l := range legs {
		switch len(l.subjects) {
		case 0:
		case 1:
			fmt.Fprintf(&b, "- **%s**: %s\n", l.name, l.subjects[0])
		default:
			fmt.Fprintf(&b, "- **%s**\n", l.name)
			for _, s := range l.subjects {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
	}
	b.WriteString("\n### How it was verified\n- ")
	return b.String()
}

// leadParagraph is a ticket description's first paragraph of prose, skipping
// fenced blocks, cut to a readable length.
func leadParagraph(desc string) string {
	var para []string
	fenced := false
	for _, line := range strings.Split(desc, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, t)
	}
	text := strings.Join(para, " ")
	if r := []rune(text); len(r) > 600 {
		text = strings.TrimSpace(string(r[:600])) + "…"
	}
	return text
}

// pinCommit matches the commits Tandem makes when it re-pins a Go module.
var pinCommit = regexp.MustCompile(`^Pin \S+ to (merged )?[0-9a-f]{7,}$`)

// authored drops blank subjects and Tandem's own pin commits.
func authored(subjects []string) []string {
	out := subjects[:0]
	for _, s := range subjects {
		if s = strings.TrimSpace(s); s != "" && !pinCommit.MatchString(s) {
			out = append(out, s)
		}
	}
	return out
}
