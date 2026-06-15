package assets

import (
	"encoding/json"
	"html"
	"path"
	"strings"
)

// islandPrefix is the bare-specifier namespace for JS islands. core.mjs resolves
// an island by name via `import("@islands/" + name)`, which the import map maps
// to the fingerprinted module URL.
const islandPrefix = "@islands/"

// ImportMap returns the import-map JSON describing every island module under
// static/js/islands/*.mjs. Adding an island file is enough to register it — no
// manual list to maintain.
func (m *Manager) ImportMap() string {
	imports := make(map[string]string)
	for p := range m.sha256 {
		if strings.HasPrefix(p, "js/islands/") && strings.HasSuffix(p, ".mjs") {
			name := strings.TrimSuffix(path.Base(p), ".mjs")
			imports[islandPrefix+name] = m.URL(p)
		}
	}
	// encoding/json sorts string-map keys, so the output is deterministic.
	b, _ := json.Marshal(struct {
		Imports map[string]string `json:"imports"`
	}{Imports: imports})
	return string(b)
}

// ImportMapScript returns the complete <script type="importmap"> element carrying
// the per-request CSP nonce. The import map is the only inline script in the app;
// everything else loads as external 'self'. The JSON contains only generated
// asset paths, so it is safe to embed (no untrusted input, no </script>).
func (m *Manager) ImportMapScript(nonce string) string {
	return `<script type="importmap" nonce="` + html.EscapeString(nonce) + `">` + m.ImportMap() + `</script>`
}
