package jira

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		w.Write([]byte(`{"key":"DEV-1","fields":{"summary":"Price Decimal Formatting","status":{"name":"In Progress"},"issuetype":{"name":"Story"},
			"priority":{"name":"High"},"assignee":{"displayName":"Sam Lee"},"reporter":null,"updated":"2026-10-02T11:04:05.123+0100",
			"description":{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"Prices show "},{"type":"text","text":"two","marks":[{"type":"strong"}]},{"type":"text","text":" decimals."}]},
				{"type":"bulletList","content":[
					{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Hosted page"}]}]},
					{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Ask "},{"type":"mention","attrs":{"text":"@Sam"}}]}]}]},
				{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://example.com/spec"}}]}]}}}`))
	}))
	defer srv.Close()
	ref := Ref{Site: srv.URL, Key: "DEV-1"}

	got, err := Client{Email: "me@example.com", Token: "tok"}.Issue(context.Background(), ref)
	want := Issue{Key: "DEV-1", Summary: "Price Decimal Formatting", Type: "Story", Status: "In Progress", Priority: "High", Assignee: "Sam Lee",
		Updated:     time.Date(2026, 10, 2, 10, 4, 5, 123e6, time.UTC),
		Description: "Prices show two decimals.\n\n• Hosted page\n• Ask @Sam\n\nhttps://example.com/spec"}
	if err != nil || !got.Updated.Equal(want.Updated) {
		t.Fatalf("Issue = %+v, %v", got, err)
	}
	got.Updated = want.Updated
	if got != want {
		t.Fatalf("Issue = %#v\nwant    %#v", got, want)
	}
	for _, c := range []Client{{}, {Email: "me@example.com", Token: "wrong"}} {
		if _, err := c.Issue(context.Background(), ref); !errors.Is(err, ErrNoAccess) {
			t.Errorf("%+v: err = %v, want ErrNoAccess", c, err)
		}
	}
}
