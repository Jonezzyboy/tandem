package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestShareLinkRoundTrip(t *testing.T) {
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.account = &Account{Login: "sam"}
	c := &change.Change{ID: "DEV-12", Branch: "dev-12-retries", Title: "Retries", Ticket: "https://acme.atlassian.net/browse/DEV-12",
		Legs: []change.Leg{{Repo: "acme/api", Base: "main", PR: 4, PRURL: "https://github.com/acme/api/pull/4"}, {Repo: "acme/web", Base: "develop"}}}
	if err := a.store.Save(c); err != nil {
		t.Fatal(err)
	}
	links, err := a.ShareLink("DEV-12", " check the retry banner ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(links.Web, shareWeb+"#") || !strings.HasPrefix(links.App, "tandem://test#") {
		t.Fatalf("links = %+v", links)
	}
	for _, link := range []string{links.Web, links.App, links.Web[strings.IndexByte(links.Web, '#')+1:]} {
		sc, err := a.OpenShare(link)
		if err != nil {
			t.Fatalf("OpenShare(%q): %v", link, err)
		}
		s := sc.Share
		if sc.Key != "DEV-12" || s.From != "sam" || s.Note != "check the retry banner" || len(s.Repos) != 2 ||
			s.Repos[0].Branch != "dev-12-retries" || s.Repos[0].PR != 4 || s.Repos[1].Base != "develop" {
			t.Errorf("share = %+v", s)
		}
	}
	if _, ok := a.shares()["DEV-12"]; !ok {
		t.Error("opened share wasn't kept")
	}
}

func TestParseShareRejectsUnsafeNames(t *testing.T) {
	enc := func(s Share) string {
		data, _ := json.Marshal(s)
		return "tandem://test#" + base64.RawURLEncoding.EncodeToString(data)
	}
	ok := Share{V: 1, ID: "X-1", Repos: []ShareRepo{{Name: "acme/api", Branch: "x-1"}}}
	if _, err := ParseShare(enc(ok)); err != nil {
		t.Fatalf("good share rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Share){
		"path traversal": func(s *Share) { s.Repos[0].Name = "../etc" },
		"dot-dot repo":   func(s *Share) { s.Repos[0].Name = "acme/.." },
		"option branch":  func(s *Share) { s.Repos[0].Branch = "--upload-pack=x" },
		"spaced branch":  func(s *Share) { s.Repos[0].Branch = "a b" },
		"no branch":      func(s *Share) { s.Repos[0].Branch = "" },
		"bad base":       func(s *Share) { s.Repos[0].Base = "-x" },
		"non-github PR":  func(s *Share) { s.Repos[0].URL = "https://evil.example/pull/1" },
		"http ticket":    func(s *Share) { s.Ticket = "javascript:alert(1)" },
		"no repos":       func(s *Share) { s.Repos = nil },
		"other version":  func(s *Share) { s.V = 2 },
	} {
		s := ok
		s.Repos = []ShareRepo{ok.Repos[0]}
		mutate(&s)
		if _, err := ParseShare(enc(s)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseShare("https://example.com/not-a-link"); err == nil {
		t.Error("garbage accepted")
	}
}
