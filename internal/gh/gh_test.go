package gh

import (
	"slices"
	"testing"
)

func TestRollup(t *testing.T) {
	pr := PR{StatusCheckRollup: []Check{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "lint", Status: "COMPLETED", Conclusion: "SKIPPED"},
		{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Name: "e2e", Status: "IN_PROGRESS"},
		{Context: "ci/legacy", State: "ERROR"},
		{Context: "ci/cover", State: "PENDING"},
	}}
	r := pr.Rollup()
	if r.Pass != 2 || r.Fail != 2 || r.Pending != 2 || !slices.Equal(r.Failing, []string{"test", "ci/legacy"}) {
		t.Errorf("rollup = %+v", r)
	}
}

func TestRef(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/proto/pull/412": "acme/proto#412",
		"https://github.com/a/b/issues/3":        "https://github.com/a/b/issues/3",
	}
	for in, want := range cases {
		if got := Ref(in); got != want {
			t.Errorf("Ref(%q) = %q, want %q", in, got, want)
		}
	}
}
