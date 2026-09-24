package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/graph"
)

// Editor choices are an app ID from knownEditors, or "cmd:<command>" for a
// command run with the repo path ("cmd:" alone uses $TANDEM_EDITOR, then code).
const cmdPrefix = "cmd:"

var knownEditors = []struct{ id, name, bundle string }{
	{"goland", "GoLand", "com.jetbrains.goland"},
	{"phpstorm", "PhpStorm", "com.jetbrains.PhpStorm"},
	{"webstorm", "WebStorm", "com.jetbrains.WebStorm"},
	{"idea", "IntelliJ IDEA", "com.jetbrains.intellij"},
	{"idea-ce", "IntelliJ IDEA CE", "com.jetbrains.intellij.ce"},
	{"rustrover", "RustRover", "com.jetbrains.rustrover"},
	{"pycharm", "PyCharm", "com.jetbrains.pycharm"},
	{"vscode", "Visual Studio Code", "com.microsoft.VSCode"},
	{"cursor", "Cursor", "com.todesktop.230313mzl4w4u92"},
	{"zed", "Zed", "dev.zed.Zed"},
	{"sublime", "Sublime Text", "com.sublimetext.4"},
}

var defaultEditors = map[string]string{"go": "goland", "php": "phpstorm", "js": "webstorm", "other": cmdPrefix}

// Languages are the keys of Settings.Editors, in the order Settings shows them.
var languages = []string{"go", "php", "js", "other"}

type EditorApp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
}

// findApp locates an app by bundle ID through Spotlight, falling back to the
// usual install folders (JetBrains Toolbox uses ~/Applications).
var findApp = func(bundle, name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mdfind", "kMDItemCFBundleIdentifier == '"+bundle+"'").Output()
	if err == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if strings.HasSuffix(line, ".app") {
				return line
			}
		}
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
		p := filepath.Join(dir, name+".app")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var (
	editorsOnce sync.Once
	editorApps  []EditorApp
)

// Editors lists the known editors, found once per launch.
func (a *App) Editors() []EditorApp {
	editorsOnce.Do(func() {
		editorApps = make([]EditorApp, len(knownEditors))
		var wg sync.WaitGroup
		for i, k := range knownEditors {
			wg.Go(func() {
				p := findApp(k.bundle, k.name)
				editorApps[i] = EditorApp{ID: k.id, Name: k.name, Path: p, Installed: p != ""}
			})
		}
		wg.Wait()
	})
	return editorApps
}

func (a *App) editorApp(id string) (EditorApp, bool) {
	for _, e := range a.Editors() {
		if e.ID == id {
			return e, true
		}
	}
	return EditorApp{}, false
}

// editorCommand is how to open dir: the editor chosen for its language, else
// the "other" choice when that app is missing, else $TANDEM_EDITOR or code.
func (a *App) editorCommand(dir string) ([]string, error) {
	editors := a.Settings().Editors
	lang := graph.Language(dir)
	if lang == "" {
		lang = "other"
	}
	var missing string
	for _, key := range []string{lang, "other"} {
		choice := editors[key]
		if strings.HasPrefix(choice, cmdPrefix) {
			return commandFor(strings.TrimPrefix(choice, cmdPrefix), dir)
		}
		app, ok := a.editorApp(choice)
		if ok && app.Installed {
			return []string{"open", "-a", app.Path, dir}, nil
		}
		if missing == "" && ok {
			missing = app.Name
		}
	}
	if missing != "" {
		return nil, fmt.Errorf("%s isn't installed: choose another editor in Settings → Editors", missing)
	}
	return commandFor("", dir)
}

func commandFor(cmd, dir string) ([]string, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		cmd = os.Getenv("TANDEM_EDITOR")
	}
	if cmd == "" {
		cmd = "code"
	}
	parts := strings.Fields(cmd)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("editor %q not found: set one in Settings → Editors", parts[0])
	}
	return append(append([]string{bin}, parts[1:]...), dir), nil
}
