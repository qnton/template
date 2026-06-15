// Package assets serves the embedded static tree and provides the helpers the
// layout needs for a strict-CSP, no-bundler frontend: content-hashed URLs for
// cache-busting, Subresource-Integrity (SRI) hashes for vendored scripts, and a
// generated import map for lazily-loaded JS islands.
//
// Hashes are computed once at startup from the embedded bytes, so serving and
// URL/integrity lookups are allocation-light and require no disk access.
package assets

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// URLPrefix is where the static handler is mounted.
const URLPrefix = "/static/"

func init() {
	// ES modules require a JavaScript MIME type or browsers refuse to execute
	// them. Register explicitly so behavior is independent of the host's mime DB.
	_ = mime.AddExtensionType(".mjs", "text/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".css", "text/css; charset=utf-8")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
}

// Manager serves assets and answers URL/integrity/import-map queries.
type Manager struct {
	fsys   fs.FS             // rooted at the static/ tree
	sha256 map[string]string // path -> hex digest (ETag + cache-busting fingerprint)
	sha384 map[string]string // path -> base64 digest (SRI)
}

// NewManager hashes every embedded file once. fsys must be rooted at static/
// (e.g. fs.Sub(app.StaticFS, "static")).
func NewManager(fsys fs.FS) (*Manager, error) {
	m := &Manager{
		fsys:   fsys,
		sha256: make(map[string]string),
		sha384: make(map[string]string),
	}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s2 := sha256.Sum256(data)
		s3 := sha512.Sum384(data)
		m.sha256[p] = hex.EncodeToString(s2[:])
		m.sha384[p] = base64.StdEncoding.EncodeToString(s3[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// URL returns a cache-busting URL for an asset path (relative to static/, e.g.
// "css/app.css") as "/static/<p>?v=<fingerprint>". The ?v= fingerprint lets the
// response be cached immutably.
func (m *Manager) URL(p string) string {
	if h, ok := m.sha256[p]; ok {
		return URLPrefix + p + "?v=" + h[:12]
	}
	return URLPrefix + p
}

// Integrity returns the SRI value ("sha384-…") for a vendored asset, or "" if
// unknown. Used on <script integrity=…> tags for third-party libs.
func (m *Manager) Integrity(p string) string {
	if h, ok := m.sha384[p]; ok {
		return "sha384-" + h
	}
	return ""
}

// Handler serves the embedded static files. Mount it at GET /static/. It sets a
// strong ETag (so conditional GETs return 304) and an immutable Cache-Control
// for fingerprinted (?v=) requests.
func (m *Manager) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(strings.TrimPrefix(r.URL.Path, URLPrefix))
		if p == "." || p == "/" || strings.HasPrefix(p, "..") {
			http.NotFound(w, r)
			return
		}
		h, ok := m.sha256[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"`+h+`"`)
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		// ServeFileFS honors the ETag we set (304) and picks Content-Type by extension.
		http.ServeFileFS(w, r, m.fsys, p)
	})
}
