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
	"encoding/json"
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
	// Credential-bearing assignments, as an env dump or a config line prints
	// them. `[ \t]` rather than `\s` around the `=`: a bare NAME= at the end of a
	// line must not swallow the next line's unrelated value.
	regexp.MustCompile(`(?i)\b(ANTHROPIC_AUTH_TOKEN|ANTHROPIC_API_KEY|OPENAI_API_KEY|SKILLHUB_TRACE_URL|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)[ \t]*=[ \t]*\S+`),
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
