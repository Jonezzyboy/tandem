package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.28.0", "0.27.0", true},
		{"0.27.1", "0.27.0", true},
		{"1.0.0", "0.99.9", true},
		{"0.27.0", "0.27.0", false},
		{"0.26.9", "0.27.0", false},
		{"0.28.0", "dev", false},
		{"0.27.0-rc1", "0.26.0", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	got := parseChecksums("AB12  Tandem_v1.0.0_macos_universal.zip\ncd34 *td.tar.gz\n\nbroken line here\n")
	if got["Tandem_v1.0.0_macos_universal.zip"] != "ab12" || got["td.tar.gz"] != "cd34" || len(got) != 2 {
		t.Errorf("parseChecksums = %v", got)
	}
}

func TestBundleOf(t *testing.T) {
	if b, err := bundleOf("/Applications/Tandem.app/Contents/MacOS/Tandem"); err != nil || b != "/Applications/Tandem.app" {
		t.Errorf("bundleOf = %q, %v", b, err)
	}
	for _, exe := range []string{"/tmp/go-build/desktop", "/x/Tandem/Contents/MacOS/Tandem", "/x/Tandem.app/MacOS/Tandem"} {
		if _, err := bundleOf(exe); err == nil {
			t.Errorf("bundleOf(%q) accepted", exe)
		}
	}
}

func TestCheckUpdateReadsCask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "# comment\ncask \"tandem-repos\" do\n  version \"9.9.9\"\n  sha256 \"abc\"\nend\n")
	}))
	defer srv.Close()
	defer func(old string) { caskURL = old }(caskURL)
	caskURL = srv.URL
	defer func(old string) { version = old }(version)
	version = "1.0.0"
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.ctx = context.Background()
	u, err := a.CheckUpdate()
	if err != nil || u.Version != "9.9.9" || !u.Newer || !strings.HasSuffix(u.URL, "/releases/tag/v9.9.9") {
		t.Errorf("CheckUpdate = %+v, %v", u, err)
	}
}
