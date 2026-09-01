package apiserver

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// GET /readyz — "can this deployment do its work right now", which is the
// question people have been asking GET /healthz (05 R-36 第二段, 04 丙-110/118).
//
// /healthz is unchanged and stays a constant. That is what a liveness probe is
// for and it was never wrong; the defect was that it was the only endpoint that
// looked like it answered this one.
//
// # Two shapes, because the answer names deployment variables
//
// The full table names the variables a capability is missing. Those are names,
// never values, but a list of what a deployment has not configured is still
// reconnaissance, and this route has no session. So:
//
//   - clean test mode: the whole table. It is a single-operator deployment on
//     one machine and the launcher on that machine is the intended reader —
//     R-36's hard condition is that the launcher reads THIS answer rather than
//     keeping a second list of the same preconditions.
//   - everywhere else: capability, readiness, and nothing else. Enough for a
//     load balancer or an operator to see that something is not ready, and the
//     detail is in the boot log where it has always been.
//
// The status code is 200 whatever the table says. A readiness endpoint that
// 503s when an OPTIONAL capability is off would make "packaging is not
// configured" indistinguishable from "the process is broken", and this whole
// item exists because two different facts were sharing one signal.
func readinessHandler(reg *envx.Registry, clean bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if reg == nil {
			// Honest rather than empty: a build with no table has not measured
			// anything, and saying "ready" here would be the original defect
			// reintroduced at the endpoint written to prevent it.
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"ready":        false,
				"capabilities": []envx.Status{},
				"detail":       "這個 build 沒有能力表，所以這裡量不到任何東西。",
			})
			return
		}
		// Bounded: /readyz is polled, and a probe against a hung dependency must
		// not hold the request open. A probe that overruns its deadline reports
		// Broken with the deadline as its reason, which is the truthful answer.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		rows := reg.Report(ctx, os.Getenv)
		if !clean {
			for i := range rows {
				rows[i].Missing = nil
				rows[i].Detail = ""
				rows[i].Without = ""
				rows[i].Fix = ""
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"ready":        envx.AllReady(rows),
			"capabilities": rows,
		})
	}
}
