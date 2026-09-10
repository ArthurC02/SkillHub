package trace

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

const Placeholder = "[REDACTED]"

var secretPatterns = []*regexp.Regexp{

	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),

	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),

	regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`),

	regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{16,}`),

	regexp.MustCompile(`(?i)\b(ANTHROPIC_AUTH_TOKEN|ANTHROPIC_API_KEY|OPENAI_API_KEY|SKILLHUB_TRACE_URL|SKILLHUB_SANDBOX_TOKEN[A-Z_]*|[A-Z_]*DATABASE_URL|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)[ \t]*=[ \t]*\S+`),

	regexp.MustCompile(`(?i)[?&](X-Amz-Signature|X-Amz-Credential|Signature|access_token|refresh_token|id_token|client_secret|api_key|apikey|api-key|password|passwd|token|secret|auth|sig|key)=[^&\s"']+`),

	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),

	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
}

var urlUserInfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@]+@`)

func allPatterns() []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(secretPatterns)+1)
	out = append(out, secretPatterns...)
	return append(out, urlUserInfo)
}

type Masker struct {
	Known []string
}

type Result struct {
	Payload json.RawMessage
	Fields  []string
}

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

func (m *Masker) redact(s string) string {
	for _, known := range m.Known {

		if len(known) < 16 {
			continue
		}
		s = strings.ReplaceAll(s, known, Placeholder)
	}
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, Placeholder)
	}

	return urlUserInfo.ReplaceAllString(s, "${1}"+Placeholder+"@")
}

func (m *Masker) MaskString(s string) string {
	return m.redact(s)
}

func escapePointer(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

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
	{"json web token", "ey" + "J" + strings.Repeat("A", 12) + "." + strings.Repeat("B", 12) + "." + strings.Repeat("C", 12)},

	{"credential in a url authority", "postgres://" + strings.Repeat("u", 8) + ":" + strings.Repeat("p", 8) + "@example.invalid:5432/db"},
}

const canaryKnownName = "platform-issued value"

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

	if len(survived) == 0 && len(result.Fields) != len(probes) {
		return []string{fmt.Sprintf("masked_fields (every probe was redacted but %d of %d were reported)",
			len(result.Fields), len(probes))}
	}
	return survived
}

func canaryKnownValue() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
