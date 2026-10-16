package gh

import (
	"slices"
	"strings"
	"testing"
)

func TestReviewersQuery(t *testing.T) {
	q := reviewersQuery([]string{"acme/api", "acme/web"})
	for _, want := range []string{
		`r0: repository(owner: "acme", name: "api")`,
		`r1: repository(owner: "acme", name: "web")`,
		`"is:pr author:@me sort:updated-desc repo:acme/api repo:acme/web"`,
		"viewer { login }",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %s:\n%s", want, q)
		}
	}
	if got := owners([]string{"acme/api", "acme/web", "beta/x"}); !slices.Equal(got, []string{"acme", "beta"}) {
		t.Errorf("owners = %q", got)
	}
}
