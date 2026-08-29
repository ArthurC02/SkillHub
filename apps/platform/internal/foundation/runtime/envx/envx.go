// Package envx reads process environment with defaults. Generic: no domain
// rules. Fail-closed checks on required secrets stay at their call sites.
//
// # What "unset" means, and the four answers this platform gives
//
// Or below is one line of convenience. The thing that actually needs to be
// shared is not code — it is the decision every new variable has to make, which
// today lives only in the comment beside whichever neighbour the author happened
// to read. There is no abstraction here for it on purpose (four call-site
// idioms, each with a reason, is not four implementations of one interface);
// what there is, is a name for each, so the choice is made explicitly.
//
//	offUnlessSet     unset = the feature is off, an unparseable value is also
//	                 off (with a slog.Warn). For data the platform COLLECTS:
//	                 NFR-002 forbids collecting a class before its retention
//	                 period exists, so "nothing configured" must mean "nothing
//	                 kept". ANALYTICS_RETENTION, DOWNLOAD_ARTIFACT_RETENTION,
//	                 FEEDBACK_RETENTION (cmd/api).
//
//	onUnlessOff      unset = enforced with defaults; only the literal `off`
//	                 disables, and it says so in the log. For PROTECTIONS: a
//	                 cost ceiling or a rate limit left unconfigured must not be
//	                 silently absent, so turning it off has to be an action
//	                 somebody wrote down. RUN_QUOTA, GENERATE_QUOTA, RATE_LIMIT
//	                 (cmd/api).
//
//	offUnlessOn      unset = off; only the literal `on` enables, and anything
//	                 else warns and stays off. For ENTRY POINTS: an unconfigured
//	                 feature just is not there yet, and a value like `true` or
//	                 `1` must not silently read as "opened" to whoever typed it.
//	                 GENERATE_SKILL_EXPOSED, SKILLHUB_CLEAN_MODE (cmd/api,
//	                 which accepts only the literal `1`).
//
//	refuseUnlessSet  unset = the process will not start. For jobs that DELETE
//	                 user content on a deadline nobody has ratified: a default
//	                 would have the platform enforce a retention no one agreed
//	                 to, by deleting. AUDIT_RETENTION, TRACE_RETENTION,
//	                 FEEDBACK_RETENTION, SKILL_DELETION_GRACE (cmd/maintenance,
//	                 positiveDuration).
//
// FEEDBACK_RETENTION appears twice on purpose and it is the shape to copy when a
// class is both disclosed and swept: the API reads it offUnlessSet, only to say
// on GET /policy/data-retention what the sweep is configured to do, and refuses
// nothing; cmd/maintenance reads it refuseUnlessSet, because that is the process
// that deletes.
//
// The asymmetry between the first three is the thing most likely to be "tidied
// up" by someone unifying them, so cmd/api/main_test.go pins each one's truth
// table separately.
package envx

import "os"

// Or returns the value of key, or fallback when it is unset or empty.
func Or(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
