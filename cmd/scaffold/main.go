// Command scaffold generates conventional template artifacts — feature slices,
// migrations, JS islands, and view components — that pass `make structure` out of
// the box. It is the inverse of cmd/structurecheck: whatever the checker requires,
// scaffold emits (handler+view+test, registry wiring, sqlc block).
//
// It is a BUILD tool (stdlib only) like structurecheck, invoked via the Makefile:
//
//	make new-feature NAME=posts        # internal/feature/posts/ + registry wiring
//	make new-feature NAME=notes DB=1   # also queries.sql + migration + sqlc block
//	make new-migration NAME=add_x      # migrations/000NN_add_x.sql
//	make new-island NAME=chart         # static/js/islands/chart.mjs
//	make new-component NAME=avatar      # internal/view/component/avatar.templ
//
// Generated Go is run through go/format before writing, so output is gofmt-clean
// with sorted imports.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

func main() {
	db := false
	var pos []string
	for _, a := range os.Args[1:] {
		if a == "--db" {
			db = true
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 2 {
		usage()
		os.Exit(2)
	}
	kind, name := pos[0], pos[1]

	var err error
	switch kind {
	case "feature":
		err = genFeature(name, db)
	case "migration":
		err = genMigration(name)
	case "island":
		err = genIsland(name)
	case "component":
		err = genComponent(name)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: scaffold <feature|migration|island|component> <name> [--db]")
}

// ── Generators ───────────────────────────────────────────────────────────────

func genFeature(name string, db bool) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join("internal", "feature", name)
	if exists(dir) {
		return fmt.Errorf("feature %q already exists at %s", name, dir)
	}
	mod, err := modulePath()
	if err != nil {
		return err
	}
	data := tmplData{Name: name, Title: title(name), Module: mod}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	handlerTmpl, viewTmpl, testTmpl := featureHandlerTmpl, featureViewTmpl, featureTestTmpl
	if db {
		handlerTmpl, viewTmpl, testTmpl = featureHandlerDBTmpl, featureViewDBTmpl, featureTestDBTmpl
	}
	if err := renderGo(filepath.Join(dir, "handler.go"), handlerTmpl, data); err != nil {
		return err
	}
	if err := renderGo(filepath.Join(dir, "handler_test.go"), testTmpl, data); err != nil {
		return err
	}
	if err := renderPlain(filepath.Join(dir, "view.templ"), viewTmpl, data); err != nil {
		return err
	}

	if db {
		if err := renderPlain(filepath.Join(dir, "queries.sql"), queriesTmpl, data); err != nil {
			return err
		}
		migPath, err := genMigrationNamed("create_"+name, migrationCreateTableTmpl, data)
		if err != nil {
			return err
		}
		if err := appendSQLC(name); err != nil {
			return err
		}
		fmt.Printf("created %s (+ %s, queries.sql, sqlc.yaml block)\n", dir, migPath)
		fmt.Println("next: `make generate` (sqlc+templ) then `make build`.")
	} else {
		fmt.Printf("created %s\n", dir)
		fmt.Println("next: `make generate` (templ) then `make build`.")
	}

	if err := registerFeature(mod, name); err != nil {
		return err
	}
	return nil
}

func genMigration(name string) error {
	if err := validateMigrationName(name); err != nil {
		return err
	}
	path, err := genMigrationNamed(name, migrationStubTmpl, tmplData{Name: name})
	if err != nil {
		return err
	}
	fmt.Printf("created %s\n", path)
	fmt.Println("next: edit the migration, then `make migrate`.")
	return nil
}

func genIsland(name string) error {
	if err := validateIslandName(name); err != nil {
		return err
	}
	path := filepath.Join("static", "js", "islands", name+".mjs")
	if exists(path) {
		return fmt.Errorf("island already exists at %s", path)
	}
	if err := renderPlain(path, islandTmpl, tmplData{Name: name}); err != nil {
		return err
	}
	fmt.Printf("created %s\n", path)
	fmt.Printf("next: add data-island=%q to an element (the import map auto-updates on `make generate`).\n", name)
	return nil
}

func genComponent(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := filepath.Join("internal", "view", "component", name+".templ")
	if exists(path) {
		return fmt.Errorf("component already exists at %s", path)
	}
	if err := renderPlain(path, componentTmpl, tmplData{Name: name, Title: title(name)}); err != nil {
		return err
	}
	fmt.Printf("created %s\n", path)
	fmt.Println("next: `make templ` then use @component." + title(name) + " in a view.")
	return nil
}

// genMigrationNamed writes a migration with the next zero-padded version number.
func genMigrationNamed(name, tmpl string, data tmplData) (string, error) {
	v, err := nextMigrationVersion("migrations")
	if err != nil {
		return "", err
	}
	path := filepath.Join("migrations", fmt.Sprintf("%05d_%s.sql", v, name))
	if exists(path) {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err := renderPlain(path, tmpl, data); err != nil {
		return "", err
	}
	return path, nil
}

// ── Wiring edits ─────────────────────────────────────────────────────────────

func registerFeature(modulePath, name string) error {
	path := filepath.Join("internal", "feature", "registry", "registry.go")
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := insertIntoRegistry(src, modulePath, name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// insertIntoRegistry adds the feature import and constructor line to registry.go,
// then runs the whole file through go/format (which sorts imports). It is
// idempotent: an already-registered feature is returned unchanged.
func insertIntoRegistry(src []byte, modulePath, name string) ([]byte, error) {
	importPath := modulePath + "/internal/feature/" + name
	s := string(src)
	if strings.Contains(s, `"`+importPath+`"`) {
		return src, nil
	}

	impClose := strings.Index(s, "\n)")
	if !strings.Contains(s, "import (") || impClose == -1 {
		return nil, fmt.Errorf("registry.go: could not find import block")
	}
	s = s[:impClose+1] + "\t\"" + importPath + "\"\n" + s[impClose+1:]

	marker := "return []app.Feature{\n"
	idx := strings.Index(s, marker)
	if idx == -1 {
		return nil, fmt.Errorf("registry.go: could not find Features slice")
	}
	pos := idx + len(marker)
	s = s[:pos] + "\t\t" + name + ".New(deps),\n" + s[pos:]

	return format.Source([]byte(s))
}

// appendSQLC appends a feature's sql block to sqlc.yaml (idempotent).
func appendSQLC(name string) error {
	path := "sqlc.yaml"
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Contains(string(src), "internal/feature/"+name+"/queries.sql") {
		return nil
	}
	block, err := render("sqlc", sqlcBlockTmpl, tmplData{Name: name})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(src, block...), 0o644)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

type tmplData struct {
	Name   string // lowercase identifier / package / table name
	Title  string // Title-cased name (headings, type/method names)
	Module string // go.mod module path
}

var (
	reIdent     = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	reIsland    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	reMigration = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	migPrefix   = regexp.MustCompile(`^(\d+)_`)
)

func validateName(name string) error {
	if !reIdent.MatchString(name) {
		return fmt.Errorf("name %q must match ^[a-z][a-z0-9]*$ (a valid Go package name)", name)
	}
	return nil
}

func validateIslandName(name string) error {
	if !reIsland.MatchString(name) {
		return fmt.Errorf("island name %q must match ^[a-z][a-z0-9-]*$", name)
	}
	return nil
}

func validateMigrationName(name string) error {
	if !reMigration.MatchString(name) {
		return fmt.Errorf("migration name %q must match ^[a-z][a-z0-9_]*$", name)
	}
	return nil
}

func nextMigrationVersion(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := migPrefix.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

func modulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("go.mod: no module path declared")
}

func render(name, tmpl string, data tmplData) ([]byte, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderGo(path, tmpl string, data tmplData) error {
	b, err := render(filepath.Base(path), tmpl, data)
	if err != nil {
		return err
	}
	formatted, err := format.Source(b)
	if err != nil {
		return fmt.Errorf("format %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0o644)
}

func renderPlain(path, tmpl string, data tmplData) error {
	b, err := render(filepath.Base(path), tmpl, data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
