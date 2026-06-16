// Package component holds reusable, parameterized templ building blocks (button,
// badge, card, stat, alert, input, …) that features compose instead of
// copy-pasting markup. Tailwind class strings are centralised in these helpers so
// the variants stay in one place. Every class references a semantic design token
// (bg-primary, text-muted-foreground, border-border, …) defined in
// static/css/input.css — re-theme there, not here.
//
// This package is PROJECT-OWNED scaffolding: the template ships it as a starting
// design system and never overwrites it on upgrade. Add, remove, or restyle
// freely.
package component

import "strconv"

func attrInt(n int) string { return strconv.Itoa(n) }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ── Button ──────────────────────────────────────────────────────────────────

const buttonBase = "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap " +
	"transition-[background-color,color,border-color,box-shadow] duration-150 focus-visible:ring-2 focus-visible:ring-ring " +
	"focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none disabled:pointer-events-none " +
	"disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0"

func buttonVariant(variant string) string {
	switch variant {
	case "secondary":
		return "bg-secondary text-secondary-foreground border border-border/60 hover:bg-accent"
	case "outline":
		return "border border-input bg-background hover:bg-accent hover:text-accent-foreground"
	case "ghost":
		return "hover:bg-accent hover:text-accent-foreground"
	case "destructive", "danger": // "danger" kept as a backwards-compatible alias
		return "bg-destructive text-destructive-foreground hover:bg-destructive/90 focus-visible:ring-destructive"
	case "success":
		return "bg-success text-success-foreground hover:bg-success/90"
	case "link":
		return "text-primary underline-offset-4 hover:underline focus-visible:ring-offset-0"
	default: // "default" / "primary"
		return "bg-primary text-primary-foreground hover:bg-primary/90 active:bg-primary/85"
	}
}

func buttonSize(size string) string {
	switch size {
	case "xs":
		return "h-7 rounded px-2.5 text-xs"
	case "sm":
		return "h-8 rounded-md px-3 text-xs"
	case "lg":
		return "h-11 rounded-md px-6 text-heading-sm"
	case "xl":
		return "h-14 rounded-md px-8 text-base"
	case "icon":
		return "size-9"
	case "iconSm":
		return "size-8"
	default:
		return "h-9 px-4"
	}
}

// buttonClasses is used by the Button templ component.
func buttonClasses(variant, size string) string {
	return buttonBase + " " + buttonVariant(variant) + " " + buttonSize(size)
}

// ButtonClass is the exported variant for styling non-<button> elements (e.g. an
// <a> that should look like a button) in feature templates.
func ButtonClass(variant, size string) string { return buttonClasses(variant, size) }

// ── Badge ───────────────────────────────────────────────────────────────────

const badgeBase = "inline-flex items-center gap-1 rounded-md border font-medium leading-none transition-colors [&>svg]:size-3 [&>svg]:shrink-0"

func badgeVariant(variant string) string {
	switch variant {
	case "outline":
		return "border-border text-foreground"
	case "muted":
		return "border-transparent bg-muted text-muted-foreground"
	case "success":
		return "border-transparent bg-success/10 text-[color:var(--success)] dark:bg-success/15 dark:text-[color:oklch(0.85_0.13_152)]"
	case "warning":
		return "border-warning/25 bg-warning/15 text-[color:var(--warning-foreground)] dark:text-[color:oklch(0.95_0.1_80)]"
	case "destructive":
		return "border-destructive/25 bg-destructive/10 text-destructive"
	case "primary":
		return "border-primary/20 bg-primary/10 text-[color:var(--primary)] dark:text-[color:var(--primary)]"
	default:
		return "border-transparent bg-secondary text-secondary-foreground"
	}
}

func badgeSize(size string) string {
	if size == "sm" {
		return "px-1.5 py-0.5 text-nano"
	}
	return "px-2 py-0.5 text-xs"
}

func badgeClasses(variant, size string) string {
	return badgeBase + " " + badgeVariant(variant) + " " + badgeSize(size)
}

// BadgeClass is the exported helper for inline badge styling in features/islands.
func BadgeClass(variant, size string) string { return badgeClasses(variant, size) }

// ── Alert ─────────────────────────────────────────────────────────────────--

const alertBase = "flex items-start gap-2.5 rounded-md border px-3 py-2 text-sm [&>svg]:mt-0.5 [&>svg]:size-4 [&>svg]:shrink-0"

// alertVariant maps a variant to its classes. "error" is a backwards-compatible
// alias for "destructive".
func alertVariant(variant string) string {
	switch variant {
	case "warning":
		return "border-warning/30 bg-warning/10 text-[color:var(--warning-foreground)] [&>svg]:text-[color:var(--warning)] dark:text-[color:oklch(0.95_0.1_80)]"
	case "success":
		return "border-success/30 bg-success/8 text-foreground [&>svg]:text-success"
	case "destructive", "error":
		return "border-destructive/30 bg-destructive/8 text-destructive [&>svg]:text-destructive"
	default: // info
		return "border-border bg-muted/40 text-foreground [&>svg]:text-muted-foreground"
	}
}

func alertClasses(variant string) string { return alertBase + " " + alertVariant(variant) }

// alertIcon returns the lucide icon name for an alert variant.
func alertIcon(variant string) string {
	switch variant {
	case "warning":
		return "AlertTriangle"
	case "success":
		return "CheckCircle2"
	default: // info / destructive / error
		return "AlertCircle"
	}
}

// ── Input / Label ─────────────────────────────────────────────────────────--

const inputClasses = "flex h-9 w-full min-w-0 rounded-md border border-input bg-card px-3 py-1 text-sm " +
	"shadow-[0_1px_0_rgba(15,23,42,0.02)] transition-[border-color,box-shadow,color] outline-none " +
	"selection:bg-primary/20 selection:text-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent " +
	"file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground/70 disabled:pointer-events-none " +
	"disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30 focus-visible:border-ring focus-visible:ring-2 " +
	"focus-visible:ring-ring/40 aria-invalid:border-destructive aria-invalid:ring-destructive/30"

// InputClass is the exported input styling for feature templates and JS islands
// that build inputs/selects/textareas directly.
const InputClass = inputClasses

const labelClasses = "text-sm font-medium leading-tight text-foreground/90 peer-disabled:cursor-not-allowed peer-disabled:opacity-70"

// LabelClass is the exported label styling.
const LabelClass = labelClasses

// ── Stat (metric chip) ────────────────────────────────────────────────────--

const statChipBase = "flex size-8 shrink-0 items-center justify-center rounded-md border [&>svg]:size-4 [&>svg]:shrink-0"

func statChipTone(tone string) string {
	switch tone {
	case "primary":
		return "border-primary/20 bg-primary/10 text-[color:var(--primary)]"
	case "success":
		return "border-success/25 bg-success/10 text-[color:var(--success)] dark:bg-success/15 dark:text-[color:oklch(0.85_0.13_152)]"
	case "warning":
		return "border-warning/25 bg-warning/15 text-[color:var(--warning-foreground)] dark:text-[color:oklch(0.95_0.1_80)]"
	default:
		return "border-border bg-muted text-muted-foreground"
	}
}

func statChipClasses(tone string) string { return statChipBase + " " + statChipTone(tone) }

// ── Card-link / interactive surface ───────────────────────────────────────--

const cardSurfaceBase = "group relative flex rounded-lg border bg-card transition-colors hover:border-primary/40 " +
	"hover:bg-accent/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"

func cardSurfaceClasses(padding, layout string) string {
	pad := "p-4"
	if padding == "dense" {
		pad = "p-3"
	}
	lay := "items-start gap-3"
	if layout == "stack" {
		lay = "flex-col gap-2"
	}
	return cardSurfaceBase + " " + pad + " " + lay
}

// ── Table (exported class consts for server-rendered tables & islands) ──────--

const (
	TableWrapClass = "relative w-full overflow-x-auto"
	TableClass     = "w-full caption-bottom text-sm"
	TableHeadClass = "[&_tr]:border-b bg-muted/40"
	TableBodyClass = "[&_tr:last-child]:border-0"
	TableRowClass  = "group border-b border-border/60 transition-colors hover:bg-muted/40 data-[state=selected]:bg-muted/60"
	TableThClass   = "h-9 px-3 text-left align-middle font-medium text-muted-foreground/90 text-xs uppercase tracking-wide"
	TableTdClass   = "px-3 py-3 align-middle"
)
