// Package app is the module root. Its sole job is to embed the static asset
// tree into the binary so the server ships as a single self-contained artifact.
//
// The directive must live here at the repo root because go:embed cannot
// reference parent directories, and static/ lives at the root. Only
// cmd/server/main.go imports this package; it hands StaticFS to the assets
// manager, which keeps internal/core/assets free of an embed dependency.
package app

import "embed"

// StaticFS holds everything under static/. Access paths are prefixed with
// "static/" (e.g. "static/css/app.css"); callers typically fs.Sub it.
//
//go:embed static
var StaticFS embed.FS
