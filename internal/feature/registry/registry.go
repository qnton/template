// Package registry is the project-owned list of enabled feature slices.
//
// Add a feature by importing it here and appending it to Features. Remove a
// feature by deleting its package, migration/query config, import, and line here.
package registry

import (
	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/feature/auth"
	"github.com/example/app/internal/feature/example"
)

// Features returns the enabled feature slices in registration order.
func Features(deps app.Deps) []app.Feature {
	return []app.Feature{
		auth.New(deps),
		example.New(deps),
	}
}
