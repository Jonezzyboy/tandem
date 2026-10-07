package jira

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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

func TestSearchMoveToAndComment(t *testing.T) {
	var moved, commented string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/search/jql":
			if r.URL.Query().Get("jql") != `status = "Ready for Test"` {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"errorMessages":["bad jql"]}`))
				return
			}
			w.Write([]byte(`{"issues":[{"key":"DEV-1","fields":{"summary":"One","status":{"name":"Ready for Test"},"issuetype":{"name":"Story"}}},
				{"key":"DEV-2","fields":{"summary":"Two","status":{"name":"Ready for Test"},"issuetype":{"name":"Bug"}}}]}`))
		case r.URL.Path == "/rest/api/3/issue/DEV-1/transitions" && r.Method == http.MethodGet:
			w.Write([]byte(`{"transitions":[{"id":"11","name":"Fix Failed","to":{"name":"Failed Test","statusCategory":{"key":"indeterminate"}}},
				{"id":"21","name":"Confirmed","to":{"name":"Done","statusCategory":{"key":"done"}}}]}`))
		case r.URL.Path == "/rest/api/3/issue/DEV-1/transitions":
			var body struct{ Transition struct{ ID string } }
			json.NewDecoder(r.Body).Decode(&body)
			moved = body.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/rest/api/3/status":
			w.Write([]byte(`[{"name":"Signoff"},{"name":"Confirm Fix"},{"name":"Signoff"}]`))
		case r.URL.Path == "/rest/api/3/issue/DEV-1/comment":
			data, _ := io.ReadAll(r.Body)
			commented = string(data)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := Client{Email: "me@example.com", Token: "tok"}
	ref := Ref{Site: srv.URL, Key: "DEV-1"}

	got, err := c.Search(context.Background(), srv.URL+"/", `status = "Ready for Test"`)
	if err != nil || len(got) != 2 || got[0].Key != "DEV-1" || got[1].Type != "Bug" {
		t.Fatalf("Search = %+v, %v", got, err)
	}
	if st, err := c.Statuses(context.Background(), srv.URL); err != nil || !slices.Equal(st, []string{"Confirm Fix", "Signoff"}) {
		t.Errorf("Statuses = %v, %v", st, err)
	}
	if q := StatusJQL([]string{"Confirm Fix", "Signoff"}); q != `status in ("Confirm Fix", "Signoff") ORDER BY updated DESC` {
		t.Errorf("StatusJQL = %s", q)
	}
	if _, err := c.Search(context.Background(), srv.URL, "nonsense"); err == nil || !strings.Contains(err.Error(), "bad jql") {
		t.Errorf("bad jql err = %v", err)
	}
	ts, err := c.Transitions(context.Background(), ref)
	if err != nil || len(ts) != 2 || ts[1] != (Transition{ID: "21", Name: "Confirmed", To: "Done", ToCategory: "done"}) {
		t.Fatalf("Transitions = %+v, %v", ts, err)
	}
	if err := c.Transition(context.Background(), ref, "21"); err != nil || moved != "21" {
		t.Errorf("Transition: moved %q, %v", moved, err)
	}
	if err := c.Comment(context.Background(), ref, "Tested on Safari.\nPasses."); err != nil ||
		!strings.Contains(commented, `"text":"Tested on Safari."`) || !strings.Contains(commented, `"text":"Passes."`) {
		t.Errorf("Comment sent %s, %v", commented, err)
	}
}
