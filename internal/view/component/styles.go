// Package component holds reusable, parameterized templ building blocks (button,
// text input, alert, theme toggle, …) that features compose instead of
// copy-pasting markup. Tailwind class strings live in these helpers so the
// variants stay in one place.
package component

import "strconv"

func attrInt(n int) string { return strconv.Itoa(n) }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func buttonClasses(variant string) string {
	const base = "inline-flex items-center justify-center rounded-md px-3 py-2 text-sm " +
		"font-medium shadow-sm transition focus:outline-none focus-visible:ring-2 " +
		"focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
	switch variant {
	case "danger":
		return base + " bg-red-600 text-white hover:bg-red-500 focus-visible:ring-red-600"
	case "secondary":
		return base + " bg-slate-100 text-slate-900 hover:bg-slate-200 " +
			"dark:bg-slate-800 dark:text-slate-100 dark:hover:bg-slate-700"
	default: // primary
		return base + " bg-indigo-600 text-white hover:bg-indigo-500 focus-visible:ring-indigo-600"
	}
}

func alertClasses(kind string) string {
	const base = "rounded-md border px-4 py-3 text-sm"
	switch kind {
	case "error":
		return base + " border-red-200 bg-red-50 text-red-800 " +
			"dark:border-red-900 dark:bg-red-950 dark:text-red-200"
	case "success":
		return base + " border-green-200 bg-green-50 text-green-800 " +
			"dark:border-green-900 dark:bg-green-950 dark:text-green-200"
	default:
		return base + " border-slate-200 bg-slate-50 text-slate-700 " +
			"dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300"
	}
}

const inputClasses = "block w-full rounded-md border border-slate-300 px-3 py-2 text-sm " +
	"shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 " +
	"dark:border-slate-700 dark:bg-slate-900"
