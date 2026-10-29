package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/jira"
)

func TestMoveTicket(t *testing.T) {
	status, moved := "In Progress", ""
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/transitions"):
			json.NewEncoder(w).Encode(map[string]any{"transitions": []map[string]any{
				{"id": "21", "name": "Review", "to": map[string]any{"name": "In Review", "statusCategory": map[string]string{"key": "indeterminate"}}},
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/transitions"):
			var body struct{ Transition struct{ ID string } }
			json.NewDecoder(r.Body).Decode(&body)
			moved = body.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		default:
			json.NewEncoder(w).Encode(map[string]any{"key": "DEV-1", "fields": map[string]any{"status": map[string]string{"name": status}}})
		}
	}))
	defer srv.Close()

	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.ctx = context.Background()
	var log []string
	a.emit = func(event string, data any) {
		if u, ok := data.(ActivityUpdate); ok {
			log = append(log, u.Text)
		}
	}
	ticket := srv.URL + "/browse/DEV-1"
	if err := a.store.Save(&change.Change{ID: "DEV-1", Branch: "DEV-1", Ticket: ticket}); err != nil {
		t.Fatal(err)
	}
	client := jira.Client{Site: srv.URL, Email: "e", Token: "t", HTTP: srv.Client()}

	a.moveTicketWith(client, "DEV-1", "in review")
	if moved != "21" || len(log) != 1 || !strings.Contains(log[0], "moved DEV-1 from In Progress to In Review") {
		t.Errorf("moved %q, log %q", moved, log)
	}

	moved, log, status = "", nil, "In Review"
	a.moveTicketWith(client, "DEV-1", "In Review")
	if moved != "" || len(log) != 0 {
		t.Errorf("a ticket already there was moved: %q %q", moved, log)
	}

	status = "Backlog"
	a.moveTicketWith(client, "DEV-1", "Done")
	if moved != "" || len(log) != 1 || !strings.Contains(log[0], "can't go from Backlog to Done") {
		t.Errorf("an impossible move: moved %q, log %q", moved, log)
	}
}

func TestOpenedForReview(t *testing.T) {
	if openedForReview([]*core.PRPlan{{Action: core.PRUpdate}}) {
		t.Error("an update counted as opening")
	}
	if !openedForReview([]*core.PRPlan{{Action: core.PRUpdate}, {Action: core.PRCreate}}) {
		t.Error("a new PR didn't count")
	}
	if !openedForReview([]*core.PRPlan{{Action: core.PRUpdate, Ready: true}}) {
		t.Error("a PR leaving draft didn't count")
	}
}
