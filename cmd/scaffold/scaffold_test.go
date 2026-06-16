package main

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"posts", "notes", "a", "user2", "x9y"}
	for _, n := range valid {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{"", "Posts", "2posts", "my-feature", "my_feature", "po sts"}
	for _, n := range invalid {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q) = nil, want error", n)
		}
	}
}

func TestValidateMigrationAndIslandNames(t *testing.T) {
	if err := validateMigrationName("create_posts"); err != nil {
		t.Errorf("migration create_posts: %v", err)
	}
	if err := validateMigrationName("Create-Posts"); err == nil {
		t.Error("migration Create-Posts should be rejected")
	}
	if err := validateIslandName("theme-toggle"); err != nil {
		t.Errorf("island theme-toggle: %v", err)
	}
	if err := validateIslandName("theme_toggle"); err == nil {
		t.Error("island theme_toggle should be rejected")
	}
}

func TestNextMigrationVersion(t *testing.T) {
	dir := t.TempDir()
	if v, err := nextMigrationVersion(dir); err != nil || v != 1 {
		t.Fatalf("empty dir: got (%d, %v), want (1, nil)", v, err)
	}
	for _, name := range []string{"00001_a.sql", "00007_b.sql", "README.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if v, err := nextMigrationVersion(dir); err != nil || v != 8 {
		t.Fatalf("got (%d, %v), want (8, nil)", v, err)
	}
}

func TestInsertIntoRegistry(t *testing.T) {
	const src = `package registry

import (
	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/feature/example"
)

func Features(deps app.Deps) []app.Feature {
	return []app.Feature{
		example.New(deps),
	}
}
`
	out, err := insertIntoRegistry([]byte(src), "github.com/example/app", "posts")
	if err != nil {
		t.Fatalf("insertIntoRegistry: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `"github.com/example/app/internal/feature/posts"`) {
		t.Errorf("missing import:\n%s", got)
	}
	if !strings.Contains(got, "posts.New(deps),") {
		t.Errorf("missing constructor line:\n%s", got)
	}
	// Output must be valid, gofmt-clean Go.
	if _, err := format.Source(out); err != nil {
		t.Errorf("output is not valid Go: %v", err)
	}
	// Idempotent: a second insert is a no-op.
	out2, err := insertIntoRegistry(out, "github.com/example/app", "posts")
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !bytes.Equal(out, out2) {
		t.Error("insertIntoRegistry is not idempotent")
	}
}

// TestGoTemplatesCompile renders the Go templates and confirms they format (and
// therefore parse) cleanly — a guard against drift breaking generated code.
func TestGoTemplatesCompile(t *testing.T) {
	data := tmplData{Name: "posts", Title: "Posts", Module: "github.com/example/app"}
	for name, tmpl := range map[string]string{
		"handler":   featureHandlerTmpl,
		"test":      featureTestTmpl,
		"handlerDB": featureHandlerDBTmpl,
		"testDB":    featureTestDBTmpl,
	} {
		b, err := render(name, tmpl, data)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if _, err := format.Source(b); err != nil {
			t.Errorf("%s template does not produce valid Go: %v\n%s", name, err, b)
		}
	}
}

func TestTitle(t *testing.T) {
	for in, want := range map[string]string{"posts": "Posts", "a": "A", "user2": "User2"} {
		if got := title(in); got != want {
			t.Errorf("title(%q) = %q, want %q", in, got, want)
		}
	}
}
