package layout

import "strings"

// NavItem is a single sidebar link (Icon is a lucide icon name; see
// internal/view/component/icons.go for the available names).
type NavItem struct {
	Href  string
	Label string
	Icon  string
}

// NavGroup is a labelled group of sidebar links.
type NavGroup struct {
	Label string
	Items []NavItem
}

// ShellData is the per-request data AppShell renders. Build it in your handler
// (Path from r.URL.Path, UserName from the session) and pass it to AppShell.
type ShellData struct {
	Title       string // <title> (defaults to "App")
	Path        string // request path, for active-link highlighting + breadcrumbs
	UserName    string // shown above sign-out; omit to hide
	DisplayName string // brand name in the sidebar (defaults to "App")
}

// Active reports whether a nav href matches the current path (exact for "/",
// prefix otherwise).
func (d ShellData) Active(href string) bool {
	if href == "/" {
		return d.Path == "/"
	}
	return d.Path == href || strings.HasPrefix(d.Path, href+"/")
}

// NavGroups returns the sidebar groups. This is project-owned scaffolding — edit
// it to match your app's sections (the defaults point at the example/auth routes).
func NavGroups() []NavGroup {
	return []NavGroup{
		{Label: "Workspace", Items: []NavItem{
			{Href: "/", Label: "Home", Icon: "Home"},
			{Href: "/account", Label: "Account", Icon: "Users"},
		}},
	}
}

// Crumb is one breadcrumb entry; Last entries are rendered unlinked.
type Crumb struct {
	Label string
	Href  string
	Last  bool
}

// Crumbs derives breadcrumb entries from the request path. Returns nil for the
// root or a single segment so the shell hides the trail there.
func Crumbs(path string) []Crumb {
	segs := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(segs) <= 1 {
		return nil
	}
	out := make([]Crumb, 0, len(segs))
	href := ""
	for i, s := range segs {
		href += "/" + s
		out = append(out, Crumb{Label: crumbLabel(s), Href: href, Last: i == len(segs)-1})
	}
	return out
}

func crumbLabel(seg string) string {
	if seg == "" {
		return ""
	}
	// ID-like segment (long, no spaces) → generic label.
	if len(seg) >= 16 {
		return "Detail"
	}
	return strings.ToUpper(seg[:1]) + seg[1:]
}

func orDefaultShell(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
