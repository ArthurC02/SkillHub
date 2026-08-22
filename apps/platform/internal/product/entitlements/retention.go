package policy

import (
	"errors"
	"time"
)

// ErrRetentionNotConfigured keeps Download Artifact creation fail-closed while
// the retention period remains an unratified deployment decision (PDM-006).
var ErrRetentionNotConfigured = errors.New("download artifact retention is not configured")

// DownloadRetention is how long a Download Artifact lives, as a deployment
// configures it — and the rule that an unset one produces no artifact at all.
//
// The rule is the point, and it is the close of debt ledger GOV-RETENTION-001.
// The original code applied a compiled-in 90 days when the environment gave
// nothing or gave nonsense, which made "已定值" and "已被追認" the same thing:
// PDM-006's 90 days is a proposal, and a default silently applying it would have
// written an unratified period onto every artifact's expires_at. So the unset
// case is not a default, it is a refusal — no ratified value, no artifact.
//
// A duration and not a struct because there is exactly one number here; when
// billing gives this context a second retention class, that is the day it grows
// a name.
type DownloadRetention time.Duration

// Period is the configured lifetime, or ErrRetentionNotConfigured. Callers must
// ask before writing anything: a zero or negative value is "this deployment has
// no ratified period", never "expires immediately".
func (r DownloadRetention) Period() (time.Duration, error) {
	if r <= 0 {
		return 0, ErrRetentionNotConfigured
	}
	return time.Duration(r), nil
}
