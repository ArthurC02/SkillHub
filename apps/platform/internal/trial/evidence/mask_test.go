package trace

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// scannerShapes are the credential shapes internal/shared/skillpkg blocks an
// import on. None is a real credential: the AWS pair is the literal AWS publishes
// in its own documentation, the rest are runs of zeroes. They are assembled at
// run time rather than written as one literal for the reason vendorKey in
// trace_test.go is - the pre-push secret scan greps for the vendor prefixes, and
// a fixture that trips it on every commit trains people to ignore the scan.
var scannerShapes = []struct {
	name   string
	sample string
}{
	{"aws access key id", "AKIA" + "IOSFODNN7" + "EXAMPLE"},
	{"github token", "gh" + "p_" + strings.Repeat("0", 36)},
	{"openai style key", vendorKey},
	{"slack token", "xox" + "b-" + "0000000000-000000000000-notarealslacktoken"},
	{"private key block", "-----BEGIN " + "RSA PRIVATE KEY-----"},
	{"aws secret assignment", "aws_secret_access_key = " + strings.Repeat("0", 40)},
}

// The package scanner refuses an import for these; the masker handles the output
// of code that already ran, so it has strictly less excuse to miss one. The two
// lists did drift - the masker passed an AWS key id, a GitHub token and a Slack
// token straight into the jsonb payload TRACE-007 renders, and because 0019's
// CHECK (masked) only asserts the masker ran, no row looked any different.
func TestMaskerCoversEveryShapeThePackageScannerBlocks(t *testing.T) {
	for _, shape := range scannerShapes {
		masked := (&Masker{}).redact("before " + shape.sample + " after")
		if strings.Contains(masked, shape.sample) {
			t.Errorf("%s survived the trace masker: %s", shape.name, masked)
		}
	}
}

// And the table above stays honest by construction: every pattern the scanner
// compiles must be exercised by one of the samples. Add a pattern there without a
// sample here and this fails; add the sample and the test above fails until the
// masker catches it too.
//
// ponytail: reading the scanner's source is the link, because there is no other
// one available - its pattern list is unexported, so there is nothing to import,
// and ADR-032 keeps trial/evidence out of shared/skillpkg besides. The cost is
// that moving or reshaping that var block fails this test loudly instead of
// silently, which is the correct direction for a drift check.
func TestEveryPackageScannerPatternHasASampleHere(t *testing.T) {
	for _, pattern := range scannerPatternsFromSource(t) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Errorf("cannot compile the scanner's pattern %s: %v", pattern, err)
			continue
		}
		matched := false
		for _, shape := range scannerShapes {
			if re.MatchString(shape.sample) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("the package scanner blocks %s and no sample here exercises it: add one, then make the masker catch it", pattern)
		}
	}
}

// scannerSource is a sibling package's file, read as text rather than imported.
const scannerSource = "../../shared/skillpkg/skillpkg.go"

func scannerPatternsFromSource(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(scannerSource)
	if err != nil {
		t.Fatalf("cannot read the package scanner (moved?): %v", err)
	}
	block := regexp.MustCompile("(?s)secretPatterns = \\[\\]\\*regexp\\.Regexp\\{(.*?)\n\t\\}").FindSubmatch(src)
	if block == nil {
		t.Fatalf("no secretPatterns block in %s: this drift check is dead", scannerSource)
	}
	var out []string
	for _, m := range regexp.MustCompile("regexp\\.MustCompile\\(`([^`]*)`\\)").FindAllStringSubmatch(string(block[1]), -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("no patterns parsed out of %s: this drift check is dead", scannerSource)
	}
	return out
}

// Shapes neither detector had before. The query parameters are matched by name
// because a URL offers no other anchor.
func TestMaskerRedactsShapesNeitherDetectorHad(t *testing.T) {
	for _, sample := range []string{
		"AI" + "za" + strings.Repeat("0", 35),
		"AWS_SESSION_TOKEN=" + strings.Repeat("0", 40),
		"https://example.test/o?key=notarealsignature",
		"https://example.test/o?apikey=notarealsignature",
		"https://example.test/o?api-key=notarealsignature",
		"https://example.test/o?sig=notarealsignature",
		"https://example.test/o?password=notarealpassword",
		"https://example.test/o?auth=notarealsignature",
		"https://example.test/o?secret=notarealsignature",
		"https://example.test/o?client_secret=notarealsignature",
		"https://example.test/o?refresh_token=notarealsignature",
	} {
		if masked := (&Masker{}).redact(sample); masked == sample {
			t.Errorf("nothing was redacted in %q", sample)
		}
	}
}

// The query-parameter names are only credentials inside a query string. Naming
// `key` and `auth` must not turn ordinary prose or a config dump into
// [REDACTED], or the trace stops being readable and people stop opening it.
func TestMaskerLeavesQueryParameterNamesAloneOutsideAQueryString(t *testing.T) {
	for _, sample := range []string{
		"the api_key field is required",
		"sorted by key=name descending",
		"auth=basic is not supported for this endpoint",
	} {
		if masked := (&Masker{}).redact(sample); masked != sample {
			t.Errorf("ordinary text was redacted: %q became %q", sample, masked)
		}
	}
}

// The canary on an intact masker. If this is ever red in CI, the same call is
// red in production and the fleet stops dispatching (02:SEC-010 P1, via
// internal/run's sweep) — which is the point: the assertion and the production
// detector are the same function.
func TestMaskerCanaryPassesOnAnIntactMasker(t *testing.T) {
	if survived := MaskerCanary(); len(survived) != 0 {
		t.Errorf("the masker canary reports these shapes unredacted: %v", survived)
	}
}

// Every pattern must be exercised by a canary sample, the same way
// TestEveryPackageScannerPatternHasASampleHere holds the scanner's list and this
// file's in step. Without it, a pattern added later is one the canary would never
// notice the loss of — and a liveness probe that covers eight of nine rules
// reports "alive" for a masker that is nine-tenths gone.
func TestEveryMaskerPatternHasACanarySample(t *testing.T) {
	for _, re := range secretPatterns {
		matched := false
		for _, shape := range canaryShapes {
			if re.MatchString(shape.sample) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("no canary sample exercises %s: add one, or the canary cannot tell if this rule is gone", re)
		}
	}
	if len(canaryShapes) != len(secretPatterns) {
		t.Errorf("%d canary samples for %d patterns: one each, or a sample is covering for a rule that has none",
			len(canaryShapes), len(secretPatterns))
	}
}

// The failure path, driven by deleting a rule — which is the regression AGENTS.md
// rule 9 keeps catching in this repo (six placeholder rules, three with no positive
// test, all deletable while the suite stayed green). The canary must name the shape
// that stopped being redacted, and must name nothing else: its result is written
// into a halt reason, an audit event and a log line, so a probe that echoed its own
// inputs would put credential-shaped strings exactly where iron rule 11 keeps values
// out of.
func TestMaskerCanaryNamesTheShapeThatStoppedBeingRedacted(t *testing.T) {
	original := secretPatterns
	t.Cleanup(func() { secretPatterns = original })
	secretPatterns = original[1:] // the vendor-key rule, gone

	survived := MaskerCanary()
	if len(survived) != 1 || survived[0] != canaryShapes[0].name {
		t.Fatalf("with the %s rule deleted the canary reported %v, want exactly [%s]",
			canaryShapes[0].name, survived, canaryShapes[0].name)
	}
	for _, reported := range survived {
		for _, shape := range canaryShapes {
			if strings.Contains(reported, shape.sample) {
				t.Errorf("the canary reported a sample value, not a name: %s", shape.name)
			}
		}
	}
}
