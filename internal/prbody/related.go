// Package prbody maintains the "Related PRs" block Tandem keeps in every PR
// of a change.
package prbody

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	startMarker = "<!-- tandem:related -->"
	endMarker   = "<!-- /tandem:related -->"
)

type Entry struct {
	Level  int
	Repo   string
	Number int
	URL    string
}

// Ticket is the issue tracker link shown above the table; empty URL omits it.
type Ticket struct {
	Label string
	URL   string
}

// Block renders the PRs as a table in merge order, marking self (a PR URL).
// Links are explicit so GitHub doesn't expand each into the shared title.
func Block(entries []Entry, self string, ticket Ticket) string {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(a.Repo, b.Repo))
	})
	var b strings.Builder
	b.WriteString(startMarker + "\n")
	if ticket.URL != "" {
		b.WriteString("Jira: [" + ticket.Label + "](" + ticket.URL + ")\n\n")
	}
	b.WriteString("### Related PRs — merge in this order\n\n| Step | Repo | PR |\n| :-: | --- | --- |\n")
	step, shared := 0, false
	for i, e := range sorted {
		if i == 0 || e.Level != sorted[i-1].Level {
			step++
		} else {
			shared = true
		}
		pr := "[#" + strconv.Itoa(e.Number) + "](" + e.URL + ")"
		if e.URL == self {
			pr = "**" + pr + " (this PR)**"
		}
		fmt.Fprintf(&b, "| %d | `%s` | %s |\n", step, e.Repo, pr)
	}
	if shared {
		b.WriteString("\nPRs sharing a step can merge in either order.\n")
	}
	b.WriteString(endMarker)
	return b.String()
}

// Apply replaces the block in body, or appends it when absent.
func Apply(body, block string) string {
	if i := strings.Index(body, startMarker); i >= 0 {
		if j := strings.Index(body[i:], endMarker); j >= 0 {
			return body[:i] + block + body[i+j+len(endMarker):]
		}
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return block
	}
	return body + "\n\n" + block
}
