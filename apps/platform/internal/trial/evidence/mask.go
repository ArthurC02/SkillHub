package trace

// TRACE-005: secret masking, run before storage.
//
// Iron rule 11 and NFR-002 say secrets must not appear in an unmasked trace. The
// only way to make that true rather than aspirational is to mask on the way *in*:
// once a plaintext key is in a jsonb column it has already leaked, backups
// included, and 0019's CHECK (masked) exists so no code path can skip this.
//
// Two kinds of match, in this order:
//
//  1. Known values. Strings this run's own control plane handed out - the
//     ingestion token above all, and later the LiteLLM Virtual Key and the
//     pre-signed object URLs once SBX-008 mints them. Exact, no false negatives,
//     and the only category that catches a credential with no recognisable shape.
//  2. Patterns. Credential shapes that arrive from somewhere the platform did not
//     issue: a user's own key pasted into a prompt, a token printed by a tool.
//     Necessarily incomplete, which is why (1) exists.
//
// Replacement is the literal `[REDACTED]`, never a length-preserving or partial
// mask: keeping the length or the prefix leaks entropy about the secret
// (contract README §6d).
//
// Scope is every string in the payload, not only the fields the schema marks
// `sensitivity: secret_bearing`. That is stricter than the README's minimum and
// cheaper to keep correct: the unmarked fields are platform-generated ids, enums
// and counts, which no pattern here can match, so walking everything costs
// nothing and removes a second list that could drift out of step with the schema.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Placeholder is the only thing a redacted value is ever replaced with.
const Placeholder = "[REDACTED]"

// secretPatterns are credential shapes, not field names. Each one is anchored on
// something structural (a vendor prefix, an auth scheme, a signature parameter)
// rather than on a word like "token", so ordinary prose does not trip it.
//
// This list is the floor under the package scanner's: internal/shared/skillpkg
// blocks an *import* for a credential shape, and a trace carries the output of
// code that already ran, so anything block-worthy there is leak-worthy here.
// TestMaskerCoversEveryShapeThePackageScannerBlocks holds the two in step.
//
// ponytail: leaf-wise masking, so a secret split across two JSON values, or
// base64-encoded before it was printed, is not reachable by any pattern here.
// The known-values pass is what covers those for credentials the platform
// issued; for a user's own key it is an accepted ceiling, not a gap to widen
// the patterns for - decoding candidate blobs would cost false positives on
// every checksum and id in the payload.
var secretPatterns = []*regexp.Regexp{
	// OpenAI and Anthropic style keys, including the project/admin variants.
	// {20,} keeps it clear of "sk-" appearing in prose or a file name.
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	// Vendor prefixes with a fixed shape, all three high-confidence enough that
	// the package scanner blocks an import on them.
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	// Google API keys: a fixed 39-character shape that neither detector had, and
	// the one a script dumping its own config is likeliest to print.
	regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`),
	// An HTTP Authorization value of any scheme, wherever it was printed.
	regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{16,}`),
	// SKILLHUB_SANDBOX_TOKEN* and *DATABASE_URL are here for the same reason
	// SKILLHUB_TRACE_URL is: they are this platform's own credentials, they carry
	// no vendor shape any pattern above would catch, and a script that dumps its
	// environment prints them exactly like this. The sandbox token is the control
	// plane's bearer credential for a node; a Postgres URL carries its password in
	// the authority. Either one in a trace is iron rule 11 broken.
	//
	// Written as the families they are rather than the one spelling in
	// .env.example: the token is SKILLHUB_SANDBOX_TOKEN_<PROVIDER> in every real
	// deployment (provider.go's NewRegistryFromEnv builds the name), and the URL
	// appears as both DATABASE_URL and SKILLHUB_TEST_DATABASE_URL.
	// Credential-bearing assignments, as an env dump or a config line prints
	// them. `[ \t]` rather than `\s` around the `=`: a bare NAME= at the end of a
	// line must not swallow the next line's unrelated value.
	regexp.MustCompile(`(?i)\b(ANTHROPIC_AUTH_TOKEN|ANTHROPIC_API_KEY|OPENAI_API_KEY|SKILLHUB_TRACE_URL|SKILLHUB_SANDBOX_TOKEN[A-Z_]*|[A-Z_]*DATABASE_URL|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)[ \t]*=[ \t]*\S+`),
	// Pre-signed object URLs and anything else carrying a credential as a query
	// value: the parameter name is the only structural anchor a URL offers, so
	// this one is a name list by necessity. A false positive costs one query
	// value reading [REDACTED] in the trace view, which is the cheap side.
	regexp.MustCompile(`(?i)[?&](X-Amz-Signature|X-Amz-Credential|Signature|access_token|refresh_token|id_token|client_secret|api_key|apikey|api-key|password|passwd|token|secret|auth|sig|key)=[^&\s"']+`),
	// A private key block pasted into a prompt or written by a script.
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

// Masker redacts one event. Known holds the exact values this run was issued;
// it is per-run, so a Masker is built per ingestion request and not shared.
type Masker struct {
	Known []string
}

// Result reports what a masking pass did. Fields are JSON Pointers relative to
// the payload root, which is what the contract's `masked_fields` carries; an
// empty slice means "ran, found nothing", which is not the same as "did not run"
// - the `masked` boolean carries that distinction.
type Result struct {
	Payload json.RawMessage
	Fields  []string
}

// Mask walks the payload and redacts every string value that matches. Non-string
// leaves are left alone: numbers, booleans and nulls cannot carry a credential,
// and redacting a token count would corrupt the trace to no purpose.
func (m *Masker) Mask(payload json.RawMessage) (Result, error) {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Result{}, err
	}
	fields := make([]string, 0)
	walked := m.walk(decoded, "", &fields)
	encoded, err := json.Marshal(walked)
	if err != nil {
		return Result{}, err
	}
	// Sorted so masked_fields is stable across runs and diffable in a test.
	sort.Strings(fields)
	return Result{Payload: encoded, Fields: fields}, nil
}

func (m *Masker) walk(node any, pointer string, fields *[]string) any {
	switch v := node.(type) {
	case string:
		masked := m.redact(v)
		if masked != v {
			*fields = append(*fields, pointer)
		}
		return masked
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			out[key] = m.walk(child, pointer+"/"+escapePointer(key), fields)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = m.walk(child, pointer+"/"+strconv.Itoa(i), fields)
		}
		return out
	default:
		return node
	}
}

// redact replaces known values first. A known value that also matches a pattern
// would be redacted either way; doing exact matches first means the placeholder
// count is not inflated by the same secret being found twice.
func (m *Masker) redact(s string) string {
	for _, known := range m.Known {
		// A short "known" value would carpet-redact ordinary text. Sixteen is
		// above anything the platform issues that is not actually a secret.
		if len(known) < 16 {
			continue
		}
		s = strings.ReplaceAll(s, known, Placeholder)
	}
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, Placeholder)
	}
	return s
}

// escapePointer applies RFC 6901: `~` becomes `~0` and `/` becomes `~1`, so a
// key containing a slash does not read as two path segments.
func escapePointer(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// --- the masker's own liveness ------------------------------------------------
//
// 02:SEC-010's `TraceMaskingStopped` was only ever measurable one way: watch real
// traffic and conclude from a run of zero redactions that the rules stopped
// matching. That inference needs traffic it does not have. The whole dev corpus is
// 2,444 sandbox events with a masked total of zero (runbook §5) — synthetic data
// carrying no secrets — so on the only corpus that exists, the premise the
// inference rests on ("normal traffic almost certainly contains something worth
// redacting") is false, and the criterion could not be evaluated at all until real
// beta traffic arrived.
//
// A canary does not need anybody's traffic. Feed the masker a value the platform
// made up on the spot and assert it came back redacted: a failure is direct
// evidence rather than an inference, and it reads the same under zero traffic,
// synthetic traffic and real traffic.

// canaryShapes are synthetic, non-secret values — one per entry in
// secretPatterns, held in step by TestEveryMaskerPatternHasACanarySample. None is
// a credential: they are runs of one character behind a vendor prefix, assembled
// from fragments at run time for the reason mask_test.go's scannerShapes are —
// a literal vendor prefix sitting in the tree trains people to ignore secret
// scans, and this file is not a test.
var canaryShapes = []struct{ name, sample string }{
	{"openai style key", "sk-" + "proj-" + strings.Repeat("A", 28)},
	{"aws access key id", "AKIA" + strings.Repeat("Q", 16)},
	{"github token", "gh" + "p_" + strings.Repeat("0", 36)},
	{"slack token", "xox" + "b-" + strings.Repeat("0", 10) + "-notarealslacktoken"},
	{"google api key", "AI" + "za" + strings.Repeat("0", 35)},
	{"authorization header", "Bearer " + strings.Repeat("0", 32)},
	{"credential assignment", "OPENAI_API_KEY=" + strings.Repeat("0", 32)},
	{"credential query parameter", "https://example.invalid/o?X-Amz-Signature=" + strings.Repeat("0", 32)},
	{"private key block", "-----BEGIN " + "RSA PRIVATE KEY-----"},
}

// canaryKnownName is the exact-match arm, which no pattern covers: it is how a
// credential the platform issued (the ingestion token above all) gets redacted,
// and a Masker whose Known pass stopped working would still pass every shape
// above.
const canaryKnownName = "platform-issued value"

// MaskerCanary runs one synthetic secret of every shape through the real masker
// and returns the names of the ones that survived. Empty means the masker is
// intact.
//
// Names only. The samples are not secrets, but the value fed through the Known
// arm is generated per call and this result is written to a halt reason, an audit
// event and a log line — a probe that reports what it fed itself is a probe that
// prints values into the places iron rule 11 exists to keep values out of.
//
// It goes through [Masker.Mask] rather than the unexported redact, because Mask
// is what [Service.Ingest] calls: the walk, the pointer bookkeeping and
// [Result.Fields] are all part of "the masker works", and Fields is what lands in
// `masked_fields` — the column the traffic-based half of this criterion counts.
//
// No database, no model call, no Run. Nine regexes and one string replace, which
// is why it can ride a 30-second sweep.
func MaskerCanary() []string {
	known, err := canaryKnownValue()
	if err != nil {
		return []string{canaryKnownName + " (no random value could be generated: " + err.Error() + ")"}
	}
	probes := make([]struct{ name, sample string }, 0, len(canaryShapes)+1)
	probes = append(probes, canaryShapes...)
	probes = append(probes, struct{ name, sample string }{canaryKnownName, known})

	payload := make(map[string]string, len(probes))
	for _, probe := range probes {
		// Embedded in surrounding text, because that is how a secret reaches a
		// trace: printed inside a command line or a log message, not alone in a
		// field of its own.
		payload[probe.name] = "canary " + probe.sample + " canary"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return []string{"the canary payload could not be encoded: " + err.Error()}
	}
	result, err := (&Masker{Known: []string{known}}).Mask(encoded)
	if err != nil {
		return []string{"the masker returned an error: " + err.Error()}
	}
	var masked map[string]string
	if err := json.Unmarshal(result.Payload, &masked); err != nil {
		return []string{"the masker returned something that is no longer an object of strings: " + err.Error()}
	}

	survived := make([]string, 0)
	for _, probe := range probes {
		if strings.Contains(masked[probe.name], probe.sample) {
			survived = append(survived, probe.name)
		}
	}
	sort.Strings(survived)
	// Redacting every probe while reporting none of them is the same failure one
	// step later: `masked_fields` is what a reader is shown and what the
	// traffic-based half of the criterion counts, so a masker that redacts
	// silently has still stopped telling the truth about what it did.
	if len(survived) == 0 && len(result.Fields) != len(probes) {
		return []string{fmt.Sprintf("masked_fields (every probe was redacted but %d of %d were reported)",
			len(result.Fields), len(probes))}
	}
	return survived
}

// canaryKnownValue is a fresh 32 hex characters. Random per call so no literal
// that looks like a credential is ever committed, and hex so it cannot be
// redacted by one of the patterns instead — this probe has to fail when the
// exact-match arm alone is what broke. Comfortably over redact's 16-character
// floor.
func canaryKnownValue() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
