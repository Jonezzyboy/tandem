// Package jira reads the Jira Cloud issue a change was started from.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Keep in sync with parseTicket in the desktop frontend.
var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)

// IsKey reports whether s is an issue key such as DEV-123.
func IsKey(s string) bool {
	return keyPattern.MatchString(s)
}

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

const issueFields = "summary,status,issuetype,priority,assignee,reporter,updated,description"

func (c Client) Issue(ctx context.Context, r Ref) (Issue, error) {
	var body rawIssue
	if err := c.do(ctx, http.MethodGet, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"?fields="+issueFields, nil, &body); err != nil {
		return Issue{}, err
	}
	return body.issue(), nil
}

// Search returns up to 50 issues matching jql on site, most relevant first as
// jql orders them.
func (c Client) Search(ctx context.Context, site, jql string) ([]Issue, error) {
	q := url.Values{"jql": {jql}, "fields": {issueFields}, "maxResults": {"50"}}
	var body struct {
		Issues []rawIssue `json:"issues"`
	}
	if err := c.do(ctx, http.MethodGet, strings.TrimRight(site, "/")+"/rest/api/3/search/jql?"+q.Encode(), nil, &body); err != nil {
		return nil, err
	}
	out := make([]Issue, len(body.Issues))
	for i, raw := range body.Issues {
		out[i] = raw.issue()
	}
	return out, nil
}

// Transition is a move the issue's workflow allows from where it is now.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   string `json:"to"`
	// ToCategory is the target status's category: "new", "indeterminate" or "done".
	ToCategory string `json:"toCategory"`
}

func (c Client) Transitions(ctx context.Context, r Ref) ([]Transition, error) {
	var body struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := c.do(ctx, http.MethodGet, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"/transitions", nil, &body); err != nil {
		return nil, err
	}
	out := make([]Transition, len(body.Transitions))
	for i, t := range body.Transitions {
		out[i] = Transition{ID: t.ID, Name: t.Name, To: t.To.Name, ToCategory: t.To.StatusCategory.Key}
	}
	return out, nil
}

// Transition moves the issue along the transition with id.
func (c Client) Transition(ctx context.Context, r Ref, id string) error {
	return c.do(ctx, http.MethodPost, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"/transitions",
		map[string]any{"transition": map[string]string{"id": id}}, nil)
}

// Statuses lists every status name on site, once each, alphabetically.
func (c Client) Statuses(ctx context.Context, site string) ([]string, error) {
	var body []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, strings.TrimRight(site, "/")+"/rest/api/3/status", nil, &body); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range body {
		if !seen[s.Name] {
			seen[s.Name] = true
			out = append(out, s.Name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// StatusJQL matches issues in any of statuses, most recently updated first.
func StatusJQL(statuses []string) string {
	quoted := make([]string, len(statuses))
	for i, s := range statuses {
		quoted[i] = strconv.Quote(s)
	}
	return "status in (" + strings.Join(quoted, ", ") + ") ORDER BY updated DESC"
}

// Comment adds text to the issue as a plain comment, a paragraph per line.
func (c Client) Comment(ctx context.Context, r Ref, text string) error {
	var paras []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		p := map[string]any{"type": "paragraph"}
		if line != "" {
			p["content"] = []map[string]any{{"type": "text", "text": line}}
		}
		paras = append(paras, p)
	}
	doc := map[string]any{"type": "doc", "version": 1, "content": paras}
	return c.do(ctx, http.MethodPost, r.Site+"/rest/api/3/issue/"+url.PathEscape(r.Key)+"/comment", map[string]any{"body": doc}, nil)
}

// do sends a request with the client's credentials and decodes a JSON answer
// into out, when given.
func (c Client) do(ctx context.Context, method, u string, in, out any) error {
	if c.Email == "" || c.Token == "" {
		return ErrNoAccess
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.Email, c.Token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	// Jira answers 404 for an issue the account can't see.
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
		return ErrNoAccess
	case resp.StatusCode == http.StatusBadRequest:
		var e struct {
			ErrorMessages []string `json:"errorMessages"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("Jira refused the request: %s", strings.Join(e.ErrorMessages, "; "))
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("Jira answered %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("reading Jira's answer: %w", err)
	}
	return nil
}

type person struct {
	DisplayName string `json:"displayName"`
}

type rawIssue struct {
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

func (raw rawIssue) issue() Issue {
	f := raw.Fields
	is := Issue{Key: raw.Key, Summary: f.Summary, Type: f.IssueType.Name, Status: f.Status.Name}
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
	return is
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
