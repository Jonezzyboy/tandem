package prbody

import (
	"strconv"
	"strings"
	"testing"
)

func pr(level int, repo string, n int) Entry {
	return Entry{Level: level, Repo: repo, Number: n, URL: "https://github.com/" + repo + "/pull/" + strconv.Itoa(n)}
}

func TestBlockOrdersByLevel(t *testing.T) {
	got := Block([]Entry{
		pr(3, "acme/monolith", 5521),
		pr(2, "acme/orchestrator", 1873),
		pr(1, "acme/proto", 412),
		pr(2, "acme/connector", 88),
	}, "https://github.com/acme/connector/pull/88", Ticket{Label: "DEV-1 · Retries", URL: "https://acme.atlassian.net/browse/DEV-1"})
	want := startMarker + `
Jira: [DEV-1 · Retries](https://acme.atlassian.net/browse/DEV-1)

### Related PRs — merge in this order

| Step | Repo | PR |
| :-: | --- | --- |
| 1 | ` + "`acme/proto`" + ` | [#412](https://github.com/acme/proto/pull/412) |
| 2 | ` + "`acme/connector`" + ` | **[#88](https://github.com/acme/connector/pull/88) (this PR)** |
| 2 | ` + "`acme/orchestrator`" + ` | [#1873](https://github.com/acme/orchestrator/pull/1873) |
| 3 | ` + "`acme/monolith`" + ` | [#5521](https://github.com/acme/monolith/pull/5521) |

PRs sharing a step can merge in either order.
` + endMarker
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBlockOmitsNoteWhenStepsAreDistinct(t *testing.T) {
	if got := Block([]Entry{pr(1, "a/b", 1), pr(2, "c/d", 2)}, "", Ticket{}); strings.Contains(got, "either order") || strings.Contains(got, "Jira:") {
		t.Errorf("note or ticket line without cause:\n%s", got)
	}
}

func TestApply(t *testing.T) {
	block := Block([]Entry{pr(1, "a/b", 1)}, "", Ticket{})
	if got := Apply("", block); got != block {
		t.Errorf("empty body: %q", got)
	}
	once := Apply("Fixes retries.\n", block)
	if once != "Fixes retries.\n\n"+block {
		t.Errorf("append: %q", once)
	}
	if twice := Apply(once, block); twice != once {
		t.Errorf("not idempotent: %q", twice)
	}
	newer := Block([]Entry{pr(1, "a/b", 1), pr(2, "c/d", 2)}, "", Ticket{})
	edited := Apply(once+"\n\nTrailing note", newer)
	if strings.Count(edited, startMarker) != 1 || !strings.Contains(edited, "c/d/pull/2") || !strings.HasSuffix(edited, "Trailing note") {
		t.Errorf("replace: %q", edited)
	}
}
