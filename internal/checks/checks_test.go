package checks

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(cs []Check) []string {
	var out []string
	for _, c := range cs {
		n := c.Name
		if c.Skip != "" {
			n += " (skip)"
		}
		out = append(out, n)
	}
	return out
}

func TestDetect(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  []string
	}{
		"go root and pnpm subdir": {
			files: map[string]string{
				"go.mod":                    "module x\n",
				"web/package.json":          `{"scripts":{"lint":"x","build":"x","test":"x"}}`,
				"web/pnpm-lock.yaml":        "",
				"node_modules/package.json": `{"scripts":{"test":"x"}}`,
				".hidden/go.mod":            "module y\n",
			},
			want: []string{"go build -o /dev/null ./...", "go vet ./...", "go test ./...", "web: pnpm run lint (skip)", "web: pnpm run test (skip)"},
		},
		"php with vendor tools": {
			files: map[string]string{
				"composer.json":         "{}",
				"phpunit.xml.dist":      "",
				"phpstan.neon":          "",
				"vendor/bin/phpunit":    "",
				"vendor/bin/phpstan":    "",
				"vendor/x/package.json": `{"scripts":{"test":"x"}}`,
			},
			want: []string{"phpstan", "phpunit"},
		},
		".tandem.yml replaces detection": {
			files: map[string]string{
				"go.mod":      "module x\n",
				".tandem.yml": "checks:\n  - name: unit\n    run: make test\n  - run: make lint\n    dir: sub\n",
			},
			want: []string{"unit", "make lint"},
		},
		"empty .tandem.yml keeps detection": {
			files: map[string]string{"go.mod": "module x\n", ".tandem.yml": "# nothing yet\n"},
			want:  []string{"go build -o /dev/null ./...", "go vet ./...", "go test ./..."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for p, data := range tc.files {
				write(t, filepath.Join(root, p), data)
			}
			got, err := Detect(root)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(names(got), tc.want) {
				t.Errorf("Detect = %q, want %q", names(got), tc.want)
			}
		})
	}
}

func TestDetectBadConfig(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".tandem.yml"), "checks: [\n")
	if _, err := Detect(root); err == nil {
		t.Error("a malformed .tandem.yml was accepted")
	}
}
