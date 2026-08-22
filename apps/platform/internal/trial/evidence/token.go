package trace

// The ingestion credential (TRACE-002).
//
// contracts/openapi/sandbox-provider.yaml gives a run exactly one place to carry
// its trace destination: TracePolicy.ingestion_url. There is no token field, and
// the contract is frozen for this batch, so the credential travels inside the URL
// itself - self-describing, self-verifying, and needing no server-side table of
// issued tokens.
//
// What the token authorizes is deliberately tiny: "append events to this one
// (run_id, attempt)". It cannot read anything, cannot name a workspace, and
// cannot be replayed against another run. The platform resolves workspace_id
// from run_id under its own authority (iron rule 3); the token is not asked.
//
// Cost of putting it in a URL, stated rather than hidden: the URL is visible to
// the workload (it is an environment variable inside the sandbox) and can land
// in an access log. Both are bounded by the token being short-lived and scoped
// to one attempt of one run. The masker treats it as a known secret value, so a
// workload that echoes its own SKILLHUB_TRACE_URL into a trace event has the
// token redacted before storage.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var (
	// ErrBadToken covers malformed, unsigned and wrongly-signed tokens alike.
	// One error for all three on purpose: telling a caller which part it got
	// wrong is a hint it should not have.
	ErrBadToken = errors.New("invalid trace ingestion token")
	// ErrTokenExpired is separate because it is the one failure an operator has
	// to be able to distinguish - it means a run outlived its own credential.
	ErrTokenExpired = errors.New("trace ingestion token has expired")
)

// Grant is what a verified token proves.
type Grant struct {
	RunID   pgtype.UUID
	Attempt int
}

// Signer mints and verifies ingestion tokens. A zero Signer (empty Secret) mints
// nothing: TracePolicy.ingestion_url stays empty, the provider is told no
// destination, and no events are produced. That is the honest state for a
// deployment that has not configured a secret - better than an endpoint anyone
// can post to.
type Signer struct {
	Secret []byte
	// TTL bounds how long a minted token stays usable. Zero means DefaultTTL.
	TTL time.Duration
}

// DefaultTTL covers the PDM-005 hard wall clock (900s) plus room for the last
// batch of a run that ended at its limit, and for the late events TRACE-008
// requires the platform to keep accepting after a terminal state.
const DefaultTTL = 2 * time.Hour

// Enabled reports whether this signer can mint anything.
func (s *Signer) Enabled() bool { return s != nil && len(s.Secret) > 0 }

func (s *Signer) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return DefaultTTL
}

// Mint returns the token for one attempt.
func (s *Signer) Mint(runID pgtype.UUID, attempt int, now time.Time) string {
	if !s.Enabled() {
		return ""
	}
	body := fmt.Sprintf("%s.%d.%d", pgconv.UUIDString(runID), attempt, now.Add(s.ttl()).Unix())
	return body + "." + s.sign(body)
}

// IngestionURL is the value that goes into TracePolicy.ingestion_url. base is
// the platform's internally reachable origin (SKILLHUB_TRACE_INGEST_URL); an
// empty base or a disabled signer yields an empty URL, which the provider reads
// as "nothing is collecting", and it emits no events.
func (s *Signer) IngestionURL(base string, runID pgtype.UUID, attempt int, now time.Time) string {
	token := s.Mint(runID, attempt, now)
	if base == "" || token == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + IngestPath + token
}

// IngestPath is the route prefix the token is appended to.
const IngestPath = "/internal/trace/"

// Verify checks the signature and the expiry and returns what the token grants.
// The signature is compared before anything in the token is parsed as meaningful,
// so an attacker cannot use parse behaviour as an oracle.
func (s *Signer) Verify(token string, now time.Time) (Grant, error) {
	if !s.Enabled() {
		return Grant{}, ErrBadToken
	}
	// Split on the *last* dot: the signed body contains dots of its own.
	i := strings.LastIndex(token, ".")
	if i <= 0 || i == len(token)-1 {
		return Grant{}, ErrBadToken
	}
	body, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(s.sign(body))) {
		return Grant{}, ErrBadToken
	}

	parts := strings.Split(body, ".")
	if len(parts) != 3 {
		return Grant{}, ErrBadToken
	}
	var runID pgtype.UUID
	if err := runID.Scan(parts[0]); err != nil {
		return Grant{}, ErrBadToken
	}
	attempt, err := strconv.Atoi(parts[1])
	if err != nil || attempt < 1 {
		return Grant{}, ErrBadToken
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Grant{}, ErrBadToken
	}
	if now.After(time.Unix(exp, 0)) {
		return Grant{}, ErrTokenExpired
	}
	return Grant{RunID: runID, Attempt: attempt}, nil
}

func (s *Signer) sign(body string) string {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
