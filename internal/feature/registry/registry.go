// Package registry is the project-owned list of enabled feature slices.
//
// Add a feature by importing it here and appending it to Features. Remove a
// feature by deleting its package, migration/query config, import, and line here.
package registry

import (
	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/core/jobs"
	"github.com/example/app/internal/core/schedule"
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

// RegisterJobs registers background-job handlers on the worker (started only when
// JOBS_ENABLED=true). Project-owned: add your handlers here. Enqueue work from a
// feature with jobs.New(deps.Pool).Enqueue(ctx, "<kind>", payload).
func RegisterJobs(deps app.Deps, w *jobs.Worker) {
	// Example:
	//   w.Handle("welcome-email", func(ctx context.Context, payload []byte) error {
	//       var p struct{ UserID int64 }
	//       if err := json.Unmarshal(payload, &p); err != nil {
	//           return err
	//       }
	//       return sendWelcome(ctx, p.UserID)
	//   })
}

// RegisterSchedule registers recurring tasks on the scheduler (started only when
// SCHEDULER_ENABLED=true). Project-owned: add your tasks here.
func RegisterSchedule(deps app.Deps, s *schedule.Scheduler) {
	// Example:
	//   s.Every("prune-sessions", time.Hour, func(ctx context.Context) error {
	//       return pruneExpiredSessions(ctx, deps.Pool)
	//   })
}
