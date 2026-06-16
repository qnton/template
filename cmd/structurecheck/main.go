// Command structurecheck validates the template's application structure.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	featureRoot      = "internal/feature"
	serverMainPath   = "cmd/server/main.go"
	sqlcPath         = "sqlc.yaml"
	registryDirName  = "registry"
	requiredHandler  = "handler.go"
	requiredView     = "view.templ"
	requiredHTTPTest = "handler_test.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var errs []string
	modulePath, err := modulePath()
	if err != nil {
		errs = append(errs, err.Error())
	}
	errs = append(errs, checkServerImports(modulePath)...)

	features, fsErrs := featureDirs()
	errs = append(errs, fsErrs...)

	sqlcData, err := os.ReadFile(sqlcPath)
	if err != nil {
		errs = append(errs, fmt.Sprintf("read %s: %v", sqlcPath, err))
	}

	for _, feature := range features {
		errs = append(errs, checkFeature(feature, sqlcData)...)
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("structure check failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	fmt.Println("structure check passed")
	return nil
}

func modulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("go.mod must declare a module path")
}

func checkServerImports(modulePath string) []string {
	if modulePath == "" {
		return nil
	}
	registryImport := modulePath + "/internal/feature/registry"
	featureImport := modulePath + "/internal/feature/"

	file, err := parser.ParseFile(token.NewFileSet(), serverMainPath, nil, parser.ImportsOnly)
	if err != nil {
		return []string{fmt.Sprintf("parse %s: %v", serverMainPath, err)}
	}

	var hasRegistry bool
	var errs []string
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == registryImport {
			hasRegistry = true
			continue
		}
		if strings.HasPrefix(path, featureImport) {
			errs = append(errs, fmt.Sprintf("%s imports concrete feature %q; import only %q", serverMainPath, path, registryImport))
		}
	}
	if !hasRegistry {
		errs = append(errs, fmt.Sprintf("%s must import %q", serverMainPath, registryImport))
	}
	return errs
}

func featureDirs() ([]string, []string) {
	entries, err := os.ReadDir(featureRoot)
	if err != nil {
		return nil, []string{fmt.Sprintf("read %s: %v", featureRoot, err)}
	}

	var features []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == registryDirName || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		features = append(features, entry.Name())
	}
	sort.Strings(features)
	return features, nil
}

func checkFeature(name string, sqlcData []byte) []string {
	dir := filepath.Join(featureRoot, name)
	var errs []string

	for _, file := range []string{requiredHandler, requiredView, requiredHTTPTest} {
		if !exists(filepath.Join(dir, file)) {
			errs = append(errs, fmt.Sprintf("%s must contain %s", dir, file))
		}
	}

	handlerPath := filepath.Join(dir, requiredHandler)
	if exists(handlerPath) {
		errs = append(errs, checkHandlerShape(name, handlerPath)...)
	}

	queriesPath := filepath.Join(dir, "queries.sql")
	if exists(queriesPath) {
		errs = append(errs, checkDBFeature(dir, sqlcData)...)
	}
	return errs
}

func checkHandlerShape(name, path string) []string {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return []string{fmt.Sprintf("parse %s: %v", path, err)}
	}

	var hasModule, hasNew, hasRoutes bool
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == "Module" {
					hasModule = true
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == "New" {
				hasNew = true
			}
			if d.Recv != nil && d.Name.Name == "Routes" {
				hasRoutes = true
			}
		}
	}

	var errs []string
	if !hasModule {
		errs = append(errs, fmt.Sprintf("%s must define type Module", path))
	}
	if !hasNew {
		errs = append(errs, fmt.Sprintf("%s must define func New(deps app.Deps) *Module", path))
	}
	if !hasRoutes {
		errs = append(errs, fmt.Sprintf("%s must define (*Module).Routes", path))
	}
	if file.Name.Name != name {
		errs = append(errs, fmt.Sprintf("%s package name must be %q", path, name))
	}
	return errs
}

func checkDBFeature(dir string, sqlcData []byte) []string {
	var errs []string
	storeDir := filepath.Join(dir, "store")
	for _, file := range []string{"db.go", "models.go", "queries.sql.go"} {
		if !exists(filepath.Join(storeDir, file)) {
			errs = append(errs, fmt.Sprintf("%s has queries.sql, so %s must exist", dir, filepath.Join("store", file)))
		}
	}

	if len(sqlcData) == 0 {
		return errs
	}
	queriesWant := filepath.ToSlash(filepath.Join(dir, "queries.sql"))
	outWant := filepath.ToSlash(storeDir)
	if !containsYAMLString(sqlcData, "queries", queriesWant) {
		errs = append(errs, fmt.Sprintf("%s must be listed as a sqlc queries path", queriesWant))
	}
	if !containsYAMLString(sqlcData, "out", outWant) {
		errs = append(errs, fmt.Sprintf("%s must be listed as a sqlc output path", outWant))
	}
	if !containsFeatureSQLBlock(sqlcData, queriesWant, outWant) {
		errs = append(errs, fmt.Sprintf("sqlc.yaml must keep %s and %s in the same sql entry", queriesWant, outWant))
	}
	return errs
}

func containsYAMLString(data []byte, key, value string) bool {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `:\s*"?` + regexp.QuoteMeta(value) + `"?\s*(?:#.*)?$`)
	return re.Match(data)
}

func containsFeatureSQLBlock(data []byte, queries, out string) bool {
	blocks := bytes.Split(data, []byte("\n  - "))
	for _, block := range blocks {
		if bytes.Contains(block, []byte("queries: "+queries)) &&
			bytes.Contains(block, []byte("out: "+out)) {
			return true
		}
		if bytes.Contains(block, []byte(`queries: "`+queries+`"`)) &&
			bytes.Contains(block, []byte(`out: "`+out+`"`)) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
