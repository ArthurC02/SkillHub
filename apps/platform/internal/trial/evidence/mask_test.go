package trace

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

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

func TestMaskerCoversEveryShapeThePackageScannerBlocks(t *testing.T) {
	for _, shape := range scannerShapes {
		masked := (&Masker{}).redact("before " + shape.sample + " after")
		if strings.Contains(masked, shape.sample) {
			t.Errorf("%s survived the trace masker: %s", shape.name, masked)
		}
	}
}

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

func TestMaskerRedactsShapesNeitherDetectorHad(t *testing.T) {
	for _, sample := range []string{
		"AI" + "za" + strings.Repeat("0", 35),
		"AWS_SESSION_TOKEN=" + strings.Repeat("0", 40),

		"SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED=" + strings.Repeat("0", 40),
		"SKILLHUB_SANDBOX_TOKEN=" + strings.Repeat("0", 40),
		"DATABASE_URL=postgres://skillhub:hunter2@db:5432/skillhub?sslmode=disable",
		"SKILLHUB_TEST_DATABASE_URL=postgres://skillhub:hunter2@db:5432/skillhub_test",
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

func TestMaskerLeavesQueryParameterNamesAloneOutsideAQueryString(t *testing.T) {
	for _, sample := range []string{
		"the api_key field is required",

		"set DATABASE_URL before starting the worker",
		"SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED is missing for this provider",
		"sorted by key=name descending",
		"auth=basic is not supported for this endpoint",
	} {
		if masked := (&Masker{}).redact(sample); masked != sample {
			t.Errorf("ordinary text was redacted: %q became %q", sample, masked)
		}
	}
}

func TestMaskerCanaryPassesOnAnIntactMasker(t *testing.T) {
	if survived := MaskerCanary(); len(survived) != 0 {
		t.Errorf("the masker canary reports these shapes unredacted: %v", survived)
	}
}

func TestEveryMaskerPatternHasACanarySample(t *testing.T) {
	for _, re := range allPatterns() {
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
	if len(canaryShapes) != len(allPatterns()) {
		t.Errorf("%d canary samples for %d patterns: one each, or a sample is covering for a rule that has none",
			len(canaryShapes), len(allPatterns()))
	}
}

func TestMaskerCanaryNamesTheShapeThatStoppedBeingRedacted(t *testing.T) {
	original := secretPatterns
	t.Cleanup(func() { secretPatterns = original })
	secretPatterns = original[1:]

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

func TestMaskerRedactsCredentialsInAUrlAuthorityAndKeepsTheRest(t *testing.T) {
	for _, tc := range []struct{ what, in, want string }{
		{
			"a postgres DSN in a connection error",
			"failed to connect to postgres://skillhub:hunter2@localhost:5432/skillhub?sslmode=disable",
			"failed to connect to postgres://" + Placeholder + "@localhost:5432/skillhub?sslmode=disable",
		},
		{
			"a token in the userinfo of a clone url",
			"cloning https://sometokenvalue@github.com/x/y.git",
			"cloning https://" + Placeholder + "@github.com/x/y.git",
		},
		{
			"a broker url",
			"amqp://user:s3cr3t@broker:5672/",
			"amqp://" + Placeholder + "@broker:5672/",
		},
	} {
		t.Run(tc.what, func(t *testing.T) {

			if got := (&Masker{}).redact(tc.in); got != tc.want {
				t.Errorf("redact(%q)\n = %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMaskerLeavesOrdinaryUrlsAlone(t *testing.T) {
	for _, sample := range []string{
		"fetched https://example.invalid/skills/a@b/manifest.json",
		"see http://127.0.0.1:8080/runs/1234",
		"git@github.com:owner/repo.git",
		"mailto:someone@example.invalid",
	} {
		if masked := (&Masker{}).redact(sample); masked != sample {
			t.Errorf("an ordinary URL was redacted: %q became %q", sample, masked)
		}
	}
}
