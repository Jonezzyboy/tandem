package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func fakeApp(t *testing.T, dir, v string) string {
	t.Helper()
	app := filepath.Join(dir, "Tandem.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>%s</string></dict></plist>`, v)
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestInstallZip(t *testing.T) {
	src := t.TempDir()
	fakeApp(t, src, "9.9.9")
	zip := filepath.Join(t.TempDir(), "Tandem_v9.9.9_macos_universal.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", filepath.Join(src, "Tandem.app"), zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %s", out)
	}
	data, _ := os.ReadFile(zip)
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "checksums.txt":
			fmt.Fprintf(w, "%s  Tandem_v9.9.9_macos_universal.zip\n", checksum)
		case "Tandem_v9.9.9_macos_universal.zip":
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer func(old string) { releaseDownURL = old }(releaseDownURL)
	releaseDownURL = srv.URL + "/"

	installed := fakeApp(t, t.TempDir(), "1.0.0")
	if err := installZip(context.Background(), "9.9.9", installed, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if v := bundleVersion(installed); v != "9.9.9" {
		t.Errorf("installed version = %q", v)
	}
	for _, leftover := range []string{installed + ".old", installed + ".new"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s left behind", leftover)
		}
	}

	checksum = "0000"
	kept := fakeApp(t, t.TempDir(), "1.0.0")
	if err := installZip(context.Background(), "9.9.9", kept, func(string) {}); err == nil {
		t.Error("a download with the wrong checksum was installed")
	}
	if v := bundleVersion(kept); v != "1.0.0" {
		t.Errorf("after a failed install the app is %q", v)
	}
}

func TestStreamCommand(t *testing.T) {
	var lines []string
	err := streamCommand(context.Background(), func(l string) { lines = append(lines, l) }, "/bin/sh", "-c", "echo one; echo; echo two >&2")
	if err != nil || len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Errorf("lines = %q, err = %v", lines, err)
	}
	err = streamCommand(context.Background(), func(string) {}, "/bin/sh", "-c", "echo nope; exit 3")
	if err == nil || err.Error() != "sh: nope" {
		t.Errorf("failure error = %v", err)
	}
}
