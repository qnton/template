package component

import "github.com/a-h/templ"

// JSONScript renders a <script type="application/json" id="…"> data block holding
// jsonStr, for islands to read via document.getElementById(id).textContent. This
// is the CSP-safe way to hand server data to an island (no inline JS, no data
// attributes too large for HTML).
//
// templ does not render child expressions inside a <script> element, so the whole
// tag is emitted as raw output. jsonStr MUST come from encoding/json.Marshal,
// whose default HTML escaping turns <, >, & into </>/& — so a value
// can never contain a literal "</script>" and break out of the block. id is a
// developer-controlled literal.
func JSONScript(id, jsonStr string) templ.Component {
	return templ.Raw(`<script type="application/json" id="` + id + `">` + jsonStr + `</script>`)
}
