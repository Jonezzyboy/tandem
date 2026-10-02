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
	"strconv"
	"strings"
	"time"
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
	Key      string
	Summary  string
	Type     string
	Status   string
	Priority string
	Assignee string
	Reporter string
	Updated  time.Time
	// Description is the issue's description as plain text.
	Description string
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"?fields=summary,status,issuetype,priority,assignee,reporter,updated,description", nil)
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
	type person struct {
		DisplayName string `json:"displayName"`
	}
	var body struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string                 `json:"summary"`
			Status      struct{ Name string }  `json:"status"`
			IssueType   struct{ Name string }  `json:"issuetype"`
			Priority    *struct{ Name string } `json:"priority"`
			Assignee    *person                `json:"assignee"`
			Reporter    *person                `json:"reporter"`
			Updated     string                 `json:"updated"`
			Description *node                  `json:"description"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Issue{}, fmt.Errorf("reading Jira's answer: %w", err)
	}
	f := body.Fields
	is := Issue{Key: body.Key, Summary: f.Summary, Type: f.IssueType.Name, Status: f.Status.Name}
	if f.Priority != nil {
		is.Priority = f.Priority.Name
	}
	if f.Assignee != nil {
		is.Assignee = f.Assignee.DisplayName
	}
	if f.Reporter != nil {
		is.Reporter = f.Reporter.DisplayName
	}
	// Jira's timestamps carry a zone offset without a colon: 2026-10-02T11:04:05.123+0100.
	is.Updated, _ = time.Parse("2006-01-02T15:04:05.000-0700", f.Updated)
	if f.Description != nil {
		is.Description = strings.TrimSpace(f.Description.text())
	}
	return is, nil
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// node is one node of Atlassian Document Format, the JSON tree Jira Cloud
// stores rich text as.
type node struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Content []node `json:"content"`
	Attrs   struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"attrs"`
}

// text flattens the tree: blocks end in a blank line, list items get a bullet
// and nested content keeps its order. Formatting and media are dropped.
func (n node) text() string {
	var b strings.Builder
	n.write(&b, "")
	return blankRuns.ReplaceAllString(b.String(), "\n\n")
}

func (n node) write(b *strings.Builder, indent string) {
	switch n.Type {
	case "text":
		b.WriteString(n.Text)
		return
	case "hardBreak":
		b.WriteString("\n" + indent)
		return
	case "mention", "emoji":
		b.WriteString(n.Attrs.Text)
		return
	case "inlineCard", "blockCard":
		b.WriteString(n.Attrs.URL)
		return
	case "bulletList", "orderedList":
		for i, item := range n.Content {
			bullet := "• "
			if n.Type == "orderedList" {
				bullet = strconv.Itoa(i+1) + ". "
			}
			b.WriteString(indent + bullet)
			for _, c := range item.Content {
				c.write(b, indent+"   ")
			}
			if !strings.HasSuffix(b.String(), "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		return
	}
	for _, c := range n.Content {
		c.write(b, indent)
	}
	switch n.Type {
	case "paragraph", "heading", "codeBlock", "blockquote", "rule", "panel", "table", "tableRow":
		b.WriteString("\n")
		if indent == "" {
			b.WriteString("\n")
		}
	case "tableCell", "tableHeader":
		b.WriteString(" ")
	}
}
