package httpx

import (
	"net/http"

	"github.com/a-h/templ"
)

// RenderHTML streams a templ component straight into the response writer (no
// intermediate buffer), setting the HTML content type and status first. Setting
// Content-Type before WriteHeader also lets the gzip middleware compress it.
//
// If rendering fails after the header is written the status cannot change; the
// caller should log the returned error.
func RenderHTML(w http.ResponseWriter, r *http.Request, status int, c templ.Component) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	return c.Render(r.Context(), w)
}
