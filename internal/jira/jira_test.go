package jira

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]string{
		"https://chive.atlassian.net/browse/DEV-3346":                                         "https://chive.atlassian.net/browse/DEV-3346",
		" https://chive.atlassian.net/browse/dev-12?focusedCommentId=1 ":                      "https://chive.atlassian.net/browse/DEV-12",
		"https://chive.atlassian.net/jira/software/projects/DEV/boards/1?selectedIssue=DEV-7": "https://chive.atlassian.net/browse/DEV-7",
	}
	for in, want := range cases {
		r, ok := Parse(in)
		if !ok || r.URL() != want {
			t.Errorf("Parse(%q) = %+v %v, want %s", in, r, ok, want)
		}
	}
	for _, in := range []string{"DEV-1", "https://chive.atlassian.net/browse/", "https://github.com/a/b/pull/1", "ftp://x/browse/DEV-1"} {
		if r, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %+v, want no match", in, r)
		}
	}
}

func TestIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, _ := r.BasicAuth(); u != "me@example.com" || p != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/rest/api/3/issue/DEV-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"key":"DEV-1","fields":{"summary":"Price Decimal Formatting","status":{"name":"In Progress"},"issuetype":{"name":"Story"}}}`))
	}))
	defer srv.Close()
	ref := Ref{Site: srv.URL, Key: "DEV-1"}

	got, err := Client{Email: "me@example.com", Token: "tok"}.Issue(context.Background(), ref)
	if err != nil || got != (Issue{Key: "DEV-1", Summary: "Price Decimal Formatting", Type: "Story", Status: "In Progress"}) {
		t.Fatalf("Issue = %+v, %v", got, err)
	}
	for _, c := range []Client{{}, {Email: "me@example.com", Token: "wrong"}} {
		if _, err := c.Issue(context.Background(), ref); !errors.Is(err, ErrNoAccess) {
			t.Errorf("%+v: err = %v, want ErrNoAccess", c, err)
		}
	}
}
