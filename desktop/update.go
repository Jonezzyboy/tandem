package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/Jonezzyboy/tandem/releases/latest"

type Update struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	// Newer is whether Version is ahead of the running app.
	Newer bool `json:"newer"`
}

// CheckUpdate reads the latest release from GitHub. A dev build is never behind.
func (a *App) CheckUpdate() (Update, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return Update{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Update{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Update{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var rel struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Update{}, err
	}
	v := strings.TrimPrefix(rel.Tag, "v")
	return Update{Version: v, URL: rel.URL, Newer: newerVersion(v, version)}, nil
}

// newerVersion reports whether a is a higher x.y.z than b; anything that
// doesn't parse, such as "dev", is never newer or older.
func newerVersion(a, b string) bool {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
