package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const caskName = "tandem-repos"

// releaseDownURL is where release assets download from; tests point it at a
// local server.
var releaseDownURL = "https://github.com/Jonezzyboy/tandem/releases/download/"

// UpdateProgress is streamed as "update" events while an update installs.
// Phase is "installing", "ready" or "failed".
type UpdateProgress struct {
	Phase   string `json:"phase"`
	Line    string `json:"line"`
	Version string `json:"version"`
}

// InstallUpdate installs the latest release: through Homebrew when Tandem
// came from its cask, so td updates too, else by replacing the app bundle with
// the release's zip. The running app is left alone until RestartToUpdate.
func (a *App) InstallUpdate() error {
	a.mu.Lock()
	if a.installing {
		a.mu.Unlock()
		return errors.New("an update is already installing")
	}
	a.installing = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.installing = false
		a.mu.Unlock()
	}()

	report := func(phase, line string) { a.emit("update", UpdateProgress{Phase: phase, Line: line}) }
	fail := func(err error) error {
		report("failed", err.Error())
		return err
	}
	bundle, err := appBundle()
	if err != nil {
		return fail(err)
	}
	u, err := a.CheckUpdate()
	if err != nil {
		return fail(err)
	}
	if !u.Newer {
		return fail(fmt.Errorf("%s is already the latest", version))
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	defer cancel()
	if brew := brewPath(); brew != "" && inApplications(bundle) && caskInstalled(ctx, brew) {
		line := func(l string) { report("installing", l) }
		// brew upgrade refreshes taps only every day or so, so it can miss a
		// release that's hours old.
		report("installing", "brew update")
		err = streamCommand(ctx, line, brew, "update")
		if err == nil {
			report("installing", "brew upgrade --cask "+caskName)
			err = streamCommand(ctx, line, brew, "upgrade", "--cask", caskName)
		}
	} else {
		err = installZip(ctx, u.Version, bundle, func(line string) { report("installing", line) })
	}
	if err != nil {
		return fail(err)
	}
	got := bundleVersion(bundle)
	if got != u.Version {
		return fail(fmt.Errorf("the install finished but %s is still %s, not %s", filepath.Base(bundle), got, u.Version))
	}
	a.mu.Lock()
	a.updateReady = got
	a.mu.Unlock()
	a.emit("update", UpdateProgress{Phase: "ready", Version: got})
	return nil
}

// UpdateReady is the version installed and waiting for a restart, or "".
// That includes one installed outside the app, by brew in Terminal say.
func (a *App) UpdateReady() string {
	a.mu.Lock()
	ready := a.updateReady
	a.mu.Unlock()
	if ready != "" {
		return ready
	}
	if bundle, err := appBundle(); err == nil {
		if v := bundleVersion(bundle); newerVersion(v, version) {
			return v
		}
	}
	return ""
}

// RestartToUpdate reopens the app on the installed update. It refuses while
// a merge train runs, since quitting would stop it part-way.
func (a *App) RestartToUpdate() error {
	a.mu.Lock()
	trains := len(a.trains)
	a.mu.Unlock()
	if trains > 0 {
		return errors.New("a merge train is running: restart once it finishes")
	}
	bundle, err := appBundle()
	if err != nil {
		return err
	}
	// The relaunch outlives this process, so it can't be a child we wait on.
	cmd := exec.Command("/bin/sh", "-c", `sleep 1; open "$0"`, bundle)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// appBundle is the .app the running binary sits in.
func appBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return bundleOf(exe)
}

func bundleOf(exe string) (string, error) {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	bundle := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || filepath.Ext(bundle) != ".app" {
		return "", errors.New("updates need the installed app; this build isn't running from an .app bundle")
	}
	return bundle, nil
}

func bundleVersion(bundle string) string {
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print CFBundleShortVersionString", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// inApplications reports whether bundle is where the cask installs apps, so
// a copy elsewhere never upgrades the cask's.
func inApplications(bundle string) bool {
	home, _ := os.UserHomeDir()
	dir := filepath.Dir(bundle)
	return dir == "/Applications" || dir == filepath.Join(home, "Applications")
}

func brewPath() string {
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func caskInstalled(ctx context.Context, brew string) bool {
	out, err := exec.CommandContext(ctx, brew, "list", "--cask", "--versions", caskName).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// streamCommand runs name, passing each line of its combined output to line.
func streamCommand(ctx context.Context, line func(string), name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	r, w := io.Pipe()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	var last string
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			if t := strings.TrimSpace(s.Text()); t != "" {
				last = t
				line(t)
			}
		}
		close(done)
	}()
	err := cmd.Wait()
	w.Close()
	<-done
	if err != nil && last != "" {
		return fmt.Errorf("%s: %s", filepath.Base(name), last)
	}
	return err
}

// installZip downloads the release's app, checks it against checksums.txt,
// and swaps it in for bundle, keeping the old one until the new one is placed.
func installZip(ctx context.Context, v, bundle string, line func(string)) error {
	zipName := fmt.Sprintf("Tandem_v%s_macos_universal.zip", v)
	dir, err := os.MkdirTemp("", "tandem-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	line("Downloading " + zipName)
	sums, err := download(ctx, releaseDownURL+"v"+v+"/checksums.txt", filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return err
	}
	want, ok := parseChecksums(string(sums))[zipName]
	if !ok {
		return fmt.Errorf("checksums.txt doesn't list %s", zipName)
	}
	zipPath := filepath.Join(dir, zipName)
	data, err := download(ctx, releaseDownURL+"v"+v+"/"+zipName, zipPath)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return errors.New("the download doesn't match its checksum, so it wasn't installed")
	}
	line("Unpacking")
	unpacked := filepath.Join(dir, "unpacked")
	if out, err := exec.CommandContext(ctx, "ditto", "-x", "-k", zipPath, unpacked).CombinedOutput(); err != nil {
		return fmt.Errorf("unpacking: %s", strings.TrimSpace(string(out)))
	}
	fresh := filepath.Join(unpacked, "Tandem.app")
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "Info.plist")); err != nil {
		return errors.New("the download has no Tandem.app")
	}
	line("Installing into " + filepath.Dir(bundle))
	staged := bundle + ".new"
	old := bundle + ".old"
	os.RemoveAll(staged)
	os.RemoveAll(old)
	if out, err := exec.CommandContext(ctx, "ditto", fresh, staged).CombinedOutput(); err != nil {
		return fmt.Errorf("copying the update: %s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, bundle); err != nil {
		os.Rename(old, bundle)
		return err
	}
	os.RemoveAll(old)
	return nil
}

func download(ctx context.Context, url, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", filepath.Base(url), resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, os.WriteFile(path, data, 0o644)
}

// parseChecksums reads shasum output: "<hex>  <file>" per line.
func parseChecksums(text string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(text, "\n") {
		if f := strings.Fields(l); len(f) == 2 {
			out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return out
}
