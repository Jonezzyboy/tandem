// Package prbody maintains the "Related PRs" block Tandem keeps in every PR
// of a change.
package prbody

import (
	"sort"
	"strconv"
	"strings"
)

const (
	startMarker = "<!-- tandem:related -->"
	endMarker   = "<!-- /tandem:related -->"
)

type Entry struct {
	Level int
	Ref   string
}

// Block lists the PRs in merge order; PRs sharing a level can merge in either order.
func Block(entries []Entry) string {
	byLevel := map[int][]string{}
	var levels []int
	for _, e := range entries {
		if _, ok := byLevel[e.Level]; !ok {
			levels = append(levels, e.Level)
		}
		byLevel[e.Level] = append(byLevel[e.Level], e.Ref)
	}
	sort.Ints(levels)
	var b strings.Builder
	b.WriteString(startMarker + "\n### Related PRs — merge in this order\n")
	for i, l := range levels {
		refs := byLevel[l]
		sort.Strings(refs)
		b.WriteString(strconv.Itoa(i+1) + ". " + strings.Join(refs, " · ") + "\n")
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
