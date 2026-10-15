package main

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	notifyCI       = "ci"
	notifyReview   = "review"
	notifyMerged   = "merged"
	notifyPins     = "pins"
	notifyTrain    = "train"
	notifyRequests = "requests"
)

var notifyKinds = []string{notifyCI, notifyReview, notifyMerged, notifyPins, notifyTrain, notifyRequests}

// Note is one notification. Change is the change it opens, "" for the Inbox.
type Note struct {
	Kind   string `json:"kind"`
	Change string `json:"change"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// changeNotes lists what changed on GitHub between two remote views of a change.
func changeNotes(prev, next core.ChangeView) []Note {
	if !prev.Remote || !next.Remote {
		return nil
	}
	var notes []Note
	add := func(kind, line string) {
		notes = append(notes, Note{Kind: kind, Change: next.ID, Body: line})
	}
	for _, l := range next.Legs {
		i := slices.IndexFunc(prev.Legs, func(p core.LegView) bool { return p.Repo == l.Repo })
		if i < 0 || prev.Legs[i].PR == nil || l.PR == nil || prev.Legs[i].PR.Number != l.PR.Number {
			continue
		}
		was, now := prev.Legs[i].PR, l.PR
		pr := fmt.Sprintf("%s #%d", l.Name, now.Number)
		if was.State != "MERGED" && now.State == "MERGED" {
			add(notifyMerged, pr+" merged")
			continue
		}
		if now.State != "OPEN" {
			continue
		}
		switch {
		case was.Fail == 0 && now.Fail > 0:
			add(notifyCI, pr+" CI failing: "+strings.Join(now.Failing, ", "))
		case (was.Fail > 0 || was.Pending > 0) && now.Fail == 0 && now.Pending == 0 && now.Pass > 0:
			add(notifyCI, pr+" CI passed")
		}
		if was.Review != now.Review {
			switch now.Review {
			case "APPROVED":
				add(notifyReview, pr+" approved")
			case "CHANGES_REQUESTED":
				add(notifyReview, pr+" has changes requested")
			}
		}
	}
	return notes
}

// notify sends notes the settings allow, folding those for one change into a
// single notification. Notes about the change on screen are dropped while the
// window is active.
func (a *App) notify(notes ...Note) {
	a.mu.Lock()
	on, active, focus, send := a.settings.Notify, a.active, a.focus, a.send
	a.mu.Unlock()
	var groups []Note
	for _, n := range notes {
		if !on[n.Kind] || (active && (n.Change == focus || n.Change == "" && focus == "")) {
			continue
		}
		title := n.Title
		if title == "" {
			title = a.noteTitle(n.Change)
		}
		if i := slices.IndexFunc(groups, func(g Note) bool { return g.Change == n.Change && g.Title == title }); i >= 0 {
			groups[i].Body += "\n" + n.Body
			continue
		}
		n.Title = title
		groups = append(groups, n)
	}
	for _, g := range groups {
		send(g)
	}
}

func (a *App) noteTitle(id string) string {
	a.mu.Lock()
	v := a.views[id]
	a.mu.Unlock()
	if v.Title == "" {
		return id
	}
	return id + " · " + v.Title
}

// sendNative shows n in Notification Center; clicking it opens n's change.
func (a *App) sendNative(n Note) {
	if a.ctx == nil || !a.notifyReady {
		return
	}
	err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    fmt.Sprintf("%s-%s-%d", n.Kind, n.Change, time.Now().UnixNano()),
		Title: n.Title,
		Body:  n.Body,
		Data:  map[string]any{"change": n.Change},
	})
	if err != nil {
		a.emit("error", "Notification: "+err.Error())
	}
}

// initNotifications runs once at startup. Wails needs a bundle identifier,
// so a bare `wails dev` binary goes without.
func (a *App) initNotifications() {
	if !runtime.IsNotificationAvailable(a.ctx) || runtime.InitializeNotifications(a.ctx) != nil {
		return
	}
	a.notifyReady = true
	runtime.OnNotificationResponse(a.ctx, func(r runtime.NotificationResult) {
		if r.Error != nil {
			return
		}
		id, _ := r.Response.UserInfo["change"].(string)
		runtime.WindowShow(a.ctx)
		a.emit("open", id)
	})
}

type NotifyStatus struct {
	Available bool `json:"available"`
	Allowed   bool `json:"allowed"`
}

func (a *App) NotificationStatus() NotifyStatus {
	if !a.notifyReady {
		return NotifyStatus{}
	}
	return NotifyStatus{Available: true, Allowed: notificationsAllowed()}
}

// AllowNotifications asks macOS for permission. Once refused, macOS stops
// asking and the user has to turn it on in System Settings.
func (a *App) AllowNotifications() (NotifyStatus, error) {
	if !a.notifyReady {
		return NotifyStatus{}, fmt.Errorf("notifications aren't available in this build")
	}
	if _, err := runtime.RequestNotificationAuthorization(a.ctx); err != nil {
		return NotifyStatus{Available: true}, err
	}
	return a.NotificationStatus(), nil
}

// OpenNotificationSettings opens macOS's Notifications settings, where a
// refusal is undone.
func (a *App) OpenNotificationSettings() error {
	return exec.Command("open", "x-apple.systempreferences:com.apple.Notifications-Settings.extension").Run()
}

// TestNotification sends one notification regardless of focus or settings.
func (a *App) TestNotification() {
	a.send(Note{Title: "Tandem", Body: "Notifications are on. Click one to open its change."})
}

// WindowActive tells the app whether its window has focus.
func (a *App) WindowActive(active bool) {
	a.mu.Lock()
	a.active = active
	a.mu.Unlock()
}

func reviewRequests(prev *Inbox, next Inbox) []Note {
	if prev == nil || prev.ReviewError != "" || next.ReviewError != "" {
		return nil
	}
	var notes []Note
	for _, it := range next.Review {
		if !slices.ContainsFunc(prev.Review, func(p InboxItem) bool { return p.URL == it.URL }) {
			notes = append(notes, Note{
				Kind:  notifyRequests,
				Title: "Review requested by " + it.Author,
				Body:  fmt.Sprintf("%s #%d · %s", it.Repo, it.Number, it.Title),
			})
		}
	}
	return notes
}
