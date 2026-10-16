package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jonezzyboy/tandem/internal/jira"
)

const (
	shareScheme = "tandem"
	// shareWeb hands the link to the app from places that only make https
	// links clickable. The payload rides in the fragment, which browsers never
	// send to the server.
	shareWeb   = "https://jonezzyboy.github.io/tandem/test/"
	sharesFile = "shares.json"
)

// Share is a change as a tester needs it: what to put each repo on, and the
// ticket to report on.
type Share struct {
	V      int         `json:"v"`
	ID     string      `json:"id"`
	Title  string      `json:"title,omitempty"`
	Ticket string      `json:"ticket,omitempty"`
	From   string      `json:"from,omitempty"`
	Note   string      `json:"note,omitempty"`
	Repos  []ShareRepo `json:"repos"`
}

type ShareRepo struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Base   string `json:"base,omitempty"`
	PR     int    `json:"pr,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Key is what tester mode files the test under: the Jira key when the change
// has a ticket, so verdicts can go to it, else the change's ID.
func (s Share) Key() string {
	if r, ok := jira.Parse(s.Ticket); ok {
		return r.Key
	}
	return strings.ToUpper(s.ID)
}

type ShareLinks struct {
	Web string `json:"web"`
	App string `json:"app"`
}

// ShareLink builds a link that opens the change in a tester's Tandem.
func (a *App) ShareLink(id, note string) (ShareLinks, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return ShareLinks{}, err
	}
	s := Share{V: 1, ID: c.ID, Title: c.Title, Ticket: c.Ticket, Note: strings.TrimSpace(note), Repos: []ShareRepo{}}
	if acc := a.Account(); acc.Error == "" {
		s.From = acc.Login
	}
	for _, l := range c.Legs {
		s.Repos = append(s.Repos, ShareRepo{Name: l.Repo, Branch: c.Branch, Base: l.Base, PR: l.PR, URL: l.PRURL})
	}
	if len(s.Repos) == 0 {
		return ShareLinks{}, errors.New("the change has no repos to share")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return ShareLinks{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return ShareLinks{Web: shareWeb + "#" + payload, App: shareScheme + "://test#" + payload}, nil
}

var (
	repoName   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	branchName = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_./-]*$`)
)

// ParseShare reads a shared link in either form, or its bare payload. The
// payload comes from someone else and drives git, so every name is checked.
func ParseShare(text string) (Share, error) {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '#'); i >= 0 {
		text = text[i+1:]
	} else if u, err := url.Parse(text); err == nil && u.Query().Get("d") != "" {
		text = u.Query().Get("d")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(text, "="))
	if err != nil {
		return Share{}, errors.New("that isn't a Tandem tester link")
	}
	var s Share
	if err := json.Unmarshal(data, &s); err != nil || s.V != 1 {
		return Share{}, errors.New("that isn't a Tandem tester link this version understands")
	}
	if s.ID == "" || len(s.Repos) == 0 {
		return Share{}, errors.New("the link names no change or repos")
	}
	if s.Ticket != "" && !strings.HasPrefix(s.Ticket, "https://") {
		return Share{}, fmt.Errorf("the link's ticket %q isn't an https link", s.Ticket)
	}
	for _, r := range s.Repos {
		if !repoName.MatchString(r.Name) || strings.Contains(r.Name, "..") {
			return Share{}, fmt.Errorf("the link has a bad repo name %q", r.Name)
		}
		for _, b := range []string{r.Branch, r.Base} {
			if b != "" && (!branchName.MatchString(b) || strings.Contains(b, "..")) {
				return Share{}, fmt.Errorf("the link has a bad branch name %q", b)
			}
		}
		if r.Branch == "" {
			return Share{}, fmt.Errorf("the link gives no branch for %s", r.Name)
		}
		if r.URL != "" && !strings.HasPrefix(r.URL, "https://github.com/") {
			return Share{}, fmt.Errorf("the link's PR %q isn't on github.com", r.URL)
		}
	}
	return s, nil
}

type SharedChange struct {
	// Key is the tester page to open for it.
	Key   string `json:"key"`
	Share Share  `json:"share"`
}

// OpenShare keeps a shared change so tester mode can plan it.
func (a *App) OpenShare(text string) (SharedChange, error) {
	s, err := ParseShare(text)
	if err != nil {
		return SharedChange{}, err
	}
	all := a.shares()
	all[s.Key()] = s
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return SharedChange{}, err
	}
	path := filepath.Join(a.store.Home, sharesFile)
	if err := os.MkdirAll(a.store.Home, 0o755); err != nil {
		return SharedChange{}, err
	}
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return SharedChange{}, err
	}
	return SharedChange{Key: s.Key(), Share: s}, os.Rename(path+".tmp", path)
}

func (a *App) shares() map[string]Share {
	all := map[string]Share{}
	if data, err := os.ReadFile(filepath.Join(a.store.Home, sharesFile)); err == nil {
		json.Unmarshal(data, &all)
	}
	return all
}

// receiveLink holds a link macOS opened the app with until the frontend
// takes it: a cold start delivers it before the page has loaded.
func (a *App) receiveLink(link string) {
	a.mu.Lock()
	a.pendingLink = link
	a.mu.Unlock()
	a.emit("share-link", nil)
}

// TakeShareLink returns the link the app was last opened with, once.
func (a *App) TakeShareLink() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	link := a.pendingLink
	a.pendingLink = ""
	return link
}
