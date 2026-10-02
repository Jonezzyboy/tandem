package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/jira"
)

// The Jira API token lives in the login Keychain under this service, keyed by
// the account email kept in settings.
const keychainService = "com.alanjones.tandem.jira"

func jiraToken(email string) string {
	if email == "" {
		return ""
	}
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", email, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// saveJiraToken sends the command on stdin (security -i) so the token never
// appears in a process's arguments.
func saveJiraToken(email, token string) error {
	q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader("add-generic-password -U -s " + q(keychainService) + " -a " + q(email) + " -w " + q(token) + "\n")
	out, err := cmd.CombinedOutput()
	// security -i exits 0 even when a command fails, reporting it on output.
	if msg := strings.TrimSpace(string(out)); err != nil || msg != "" {
		return errors.New("saving the token to the Keychain: " + cmp.Or(msg, fmt.Sprint(err)))
	}
	if jiraToken(email) != token {
		return errors.New("the Keychain didn't keep the token")
	}
	return nil
}

type JiraAccount struct {
	Email    string `json:"email"`
	HasToken bool   `json:"hasToken"`
}

func (a *App) JiraAccount() JiraAccount {
	email := a.Settings().Jira.Email
	return JiraAccount{Email: email, HasToken: jiraToken(email) != ""}
}

// SaveJira stores email in settings and token in the Keychain; an empty token
// keeps the one already saved for email.
func (a *App) SaveJira(email, token string) (JiraAccount, error) {
	email, token = strings.TrimSpace(email), strings.TrimSpace(token)
	if token != "" {
		if email == "" {
			return a.JiraAccount(), errors.New("enter the email of the Atlassian account the token belongs to")
		}
		if err := saveJiraToken(email, token); err != nil {
			return a.JiraAccount(), err
		}
	}
	s := a.Settings()
	s.Jira.Email = email
	if _, err := a.SaveSettings(s); err != nil {
		return a.JiraAccount(), err
	}
	return a.JiraAccount(), nil
}

type JiraTicket struct {
	Key     string `json:"key"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	// Error says why the issue couldn't be read; Key and URL are still set.
	Error    string `json:"error"`
	NoAccess bool   `json:"noAccess"`
}

// JiraLookup reads the issue a Jira link points at.
func (a *App) JiraLookup(link string) (JiraTicket, error) {
	r, ok := jira.Parse(link)
	if !ok {
		return JiraTicket{}, errors.New("not a Jira issue link")
	}
	t := JiraTicket{Key: r.Key, URL: r.URL()}
	email := a.Settings().Jira.Email
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	is, err := jira.Client{Email: email, Token: jiraToken(email)}.Issue(ctx, r)
	if err != nil {
		t.Error, t.NoAccess = err.Error(), errors.Is(err, jira.ErrNoAccess)
		return t, nil
	}
	t.Summary, t.Type, t.Status = is.Summary, is.Type, is.Status
	return t, nil
}

// SetTicket links change id to a Jira issue, or unlinks it when link is empty.
func (a *App) SetTicket(id, link string) error {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return err
	}
	c.Ticket = ""
	if strings.TrimSpace(link) != "" {
		r, ok := jira.Parse(link)
		if !ok {
			return errors.New("not a Jira issue link")
		}
		c.Ticket = r.URL()
	}
	if err := a.store.Save(c); err != nil {
		return err
	}
	go a.refresh(id, false)
	return nil
}
