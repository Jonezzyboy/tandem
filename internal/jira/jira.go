// Package jira reads the Jira Cloud issue a change was started from.
package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Keep in sync with parseTicket in the desktop frontend.
var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)

type Ref struct {
	Site string
	Key  string
}

// URL is the issue's canonical browse link.
func (r Ref) URL() string {
	return r.Site + "/browse/" + r.Key
}

// Parse finds the issue in a Jira link: a /browse/KEY page, or a board or
// search page with ?selectedIssue=KEY.
func Parse(link string) (Ref, bool) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Ref{}, false
	}
	key := u.Query().Get("selectedIssue")
	if parts := strings.Split(strings.Trim(u.Path, "/"), "/"); key == "" && len(parts) >= 2 && parts[len(parts)-2] == "browse" {
		key = parts[len(parts)-1]
	}
	key = strings.ToUpper(key)
	if !keyPattern.MatchString(key) {
		return Ref{}, false
	}
	return Ref{Site: "https://" + u.Host, Key: key}, true
}

type Issue struct {
	Key     string
	Summary string
	Type    string
	Status  string
}

// ErrNoAccess means Jira wants credentials it wasn't given or refused them.
var ErrNoAccess = errors.New("Jira needs a sign-in to read this ticket")

type Client struct {
	Email string
	Token string
	HTTP  *http.Client
}

func (c Client) Issue(ctx context.Context, r Ref) (Issue, error) {
	if c.Email == "" || c.Token == "" {
		return Issue{}, ErrNoAccess
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"?fields=summary,status,issuetype", nil)
	if err != nil {
		return Issue{}, err
	}
	req.SetBasicAuth(c.Email, c.Token)
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Issue{}, err
	}
	defer resp.Body.Close()
	switch {
	// Jira answers 404 for an issue the account can't see.
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
		return Issue{}, ErrNoAccess
	case resp.StatusCode != http.StatusOK:
		return Issue{}, fmt.Errorf("Jira answered %s", resp.Status)
	}
	var body struct {
		Key    string `json:"key"`
		Fields struct {
			Summary   string                `json:"summary"`
			Status    struct{ Name string } `json:"status"`
			IssueType struct{ Name string } `json:"issuetype"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Issue{}, fmt.Errorf("reading Jira's answer: %w", err)
	}
	return Issue{Key: body.Key, Summary: body.Fields.Summary, Type: body.Fields.IssueType.Name, Status: body.Fields.Status.Name}, nil
}
