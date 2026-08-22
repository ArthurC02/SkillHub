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
var secretPatterns = []*regexp.Regexp{
	// OpenAI and Anthropic style keys, including the project/admin variants.
	// {20,} keeps it clear of "sk-" appearing in prose or a file name.
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	// An HTTP Authorization value of any scheme, wherever it was printed.
	regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{16,}`),
	// The gateway credential as the Agent SDK receives it, echoed as an env dump.
	regexp.MustCompile(`(?i)\b(ANTHROPIC_AUTH_TOKEN|ANTHROPIC_API_KEY|OPENAI_API_KEY|SKILLHUB_TRACE_URL)=\S+`),
	// Pre-signed object URLs: the signature is the secret, and it is always a
	// query parameter of one of these names.
	regexp.MustCompile(`(?i)[?&](X-Amz-Signature|X-Amz-Credential|Signature|token|api_key|access_token)=[^&\s"']+`),
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
