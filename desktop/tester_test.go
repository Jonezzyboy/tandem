package main

import (
	"slices"
	"testing"

	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/jira"
	"github.com/jonezzyboy/tandem/internal/testrun"
)

func TestOutcomeOf(t *testing.T) {
	cases := map[jira.Transition]string{
		{Name: "Confirmed", To: "Done", ToCategory: "done"}:                      "passed",
		{Name: "All Good", To: "Resolved", ToCategory: "done"}:                   "passed",
		{Name: "Fix Failed", To: "Failed Test", ToCategory: "indeterminate"}:     "failed",
		{Name: "Require Change", To: "Failed Test", ToCategory: "indeterminate"}: "failed",
		{Name: "Cancel", To: "Rejected", ToCategory: "done"}:                     "failed",
		{Name: "Deliver", To: "Ready for delivery", ToCategory: "indeterminate"}: "moved",
	}
	for tr, want := range cases {
		if got := outcomeOf(tr); got != want {
			t.Errorf("outcomeOf(%s → %s) = %s, want %s", tr.Name, tr.To, got, want)
		}
	}
}

func TestTesterStatusesMigrateAndDedupe(t *testing.T) {
	s := normalize(Settings{Tester: TesterSettings{ReadyStatus: "Confirm Fix"}})
	if !slices.Equal(s.Tester.Statuses, []string{"Confirm Fix"}) || s.Tester.ReadyStatus != "" {
		t.Errorf("migrated = %+v", s.Tester)
	}
	s = normalize(Settings{Tester: TesterSettings{Statuses: []string{" Signoff ", "Confirm Fix", "Signoff", ""}}})
	if !slices.Equal(s.Tester.Statuses, []string{"Signoff", "Confirm Fix"}) {
		t.Errorf("statuses = %q", s.Tester.Statuses)
	}
	if s := normalize(Settings{}); s.Tester.Statuses == nil {
		t.Error("statuses nil, want empty")
	}
}

func TestPRsForMatchesBranchOrTitle(t *testing.T) {
	all := []gh.OpenPR{
		{Repo: "a/x", Number: 1, Branch: "DEV-12", CI: "SUCCESS"},
		{Repo: "a/y", Number: 2, Branch: "DEV-12-price-fix", CI: "FAILURE"},
		{Repo: "a/z", Number: 3, Branch: "feature/dev-12", CI: "PENDING"},
		{Repo: "a/w", Number: 4, Branch: "prices", Title: "DEV-12 Fix prices"},
		{Repo: "a/v", Number: 5, Branch: "DEV-120", Title: "DEV-120 other"},
		{Repo: "a/u", Number: 6, Branch: "XDEV-12"},
	}
	got := prsFor(all, "DEV-12")
	var nums []int
	for _, p := range got {
		nums = append(nums, p.Number)
	}
	if !slices.Equal(nums, []int{4, 1, 2, 3}) {
		t.Fatalf("matched %v", nums)
	}
	if got[1].Pass != 1 || got[2].Fail != 1 || got[3].Pending != 1 || got[2].Branch != "DEV-12-price-fix" {
		t.Errorf("prs = %+v", got)
	}
}

func TestChecklist(t *testing.T) {
	texts := func(cs []testrun.Check) (out []string) {
		for _, c := range cs {
			out = append(out, c.Group+"|"+c.Text)
		}
		return out
	}
	desc := "Show token expiry.\n\n• Context only\n\nAcceptance criteria\n\n• Shows MM/YY\n   • nested detail\n• Expired badge\n\nNotes\n\n• not a check\n"
	if got := texts(checklist(desc)); !slices.Equal(got, []string{"|Shows MM/YY", "|Expired badge"}) {
		t.Errorf("under heading = %q", got)
	}
	// Lists introduced by a sentence each keep it, shortened, as their section.
	desc = "We need an in-app flow.\nFor the main order form page we will need to consider...\n\n• Features\n• Purchase button\n\n" +
		"For the Term Upgrade, Addon, Upsell Pages we will need to concider…\n\n• Features\n\nFor development\n\n• Separate tab\n"
	want := []string{"Main order form page|Features", "Main order form page|Purchase button", "Term Upgrade, Addon, Upsell Pages|Features", "Development|Separate tab"}
	if got := texts(checklist(desc)); !slices.Equal(got, want) {
		t.Errorf("sections = %q", got)
	}
	if got := texts(checklist("1. First\n2. Second\n- [ ] Third\n")); !slices.Equal(got, []string{"|First", "|Second", "|Third"}) {
		t.Errorf("no lead-in = %q", got)
	}
	if got := checklist("Just prose."); got == nil || len(got) != 0 {
		t.Errorf("prose = %#v", got)
	}
}

func TestVerdictComment(t *testing.T) {
	checks := []testrun.Check{{Text: "MM/YY", Done: true}, {Group: "Confirmation", Text: "badge"}}
	want := "Badge wrong.\n\nChecked:\n[x] MM/YY\n\nConfirmation:\n[ ] badge\n\nAttached: a.png"
	if got := verdictComment(" Badge wrong. ", checks, []string{"a.png"}); got != want {
		t.Errorf("comment = %q", got)
	}
	if got := verdictComment("  ", nil, nil); got != "" {
		t.Errorf("empty comment = %q", got)
	}
}
