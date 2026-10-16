package main

import "testing"

func TestDraftBody(t *testing.T) {
	got := draftBody("Retries were missing.", []legCommits{
		{name: "proto", subjects: []string{"Add retry field"}},
		{name: "api", subjects: []string{"Retry on 503", "Log attempts"}},
		{name: "web"},
	})
	want := "Retries were missing.\n\n### What changed\n- **proto**: Add retry field\n- **api**\n  - Retry on 503\n  - Log attempts\n\n### How it was verified\n- "
	if got != want {
		t.Errorf("draftBody =\n%q\nwant\n%q", got, want)
	}
}

func TestLeadParagraph(t *testing.T) {
	desc := "\n```\n| a | b |\n```\nUsers hit 503s\nwith no retry.\n\nSecond paragraph."
	if got := leadParagraph(desc); got != "Users hit 503s with no retry." {
		t.Errorf("leadParagraph = %q", got)
	}
	long := make([]rune, 700)
	for i := range long {
		long[i] = 'x'
	}
	if got := []rune(leadParagraph(string(long))); len(got) != 601 || got[600] != '…' {
		t.Errorf("long paragraph not cut: %d runes", len(got))
	}
}

func TestAuthored(t *testing.T) {
	got := authored([]string{"Add retries", "", "Pin github.com/acme/proto to merged dabaf3e360dd", "Pin github.com/acme/proto to 1a2b3c4", "Pin the version banner"})
	if len(got) != 2 || got[0] != "Add retries" || got[1] != "Pin the version banner" {
		t.Errorf("authored = %q", got)
	}
}
