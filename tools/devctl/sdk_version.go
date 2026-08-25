package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The Agent SDK version, written down three times and reconciled nowhere.
//
// AGENTS.md says the version string is pinned in
// infra/images/runtime-agent-sdk/Dockerfile's `ARG CLAUDE_AGENT_SDK_VERSION`
// and explicitly not in tools/toolchain.yaml. Both halves are true, and both
// halves stop short of the copies: the same string is compiled into
// apps/sandbox/cmd/sandboxd/main.go as the fallback of
// SKILLHUB_SANDBOX_RUNTIME_VERSION, and written a third time in
// apps/sandbox/README.md's settings table, which is where an operator reads
// what that fallback is.
//
// WHY THE COPIES MATTER RATHER THAN JUST BEING UNTIDY. The Go fallback is what
// sandboxd advertises in ProviderCapability, and dispatch freezes the advertised
// version into runs.runtime_snapshot as the run's permanent runtime_version
// (I-05). Bump the ARG alone and every subsequent Run is stamped with a version
// that never ran — silently, because a wrong string is still a valid string and
// nothing downstream can tell. The revalidation trail ADR-023 exists to protect
// (§2's four measurements, §4's UPGRADES.md ledger) is a trail from a Run back
// to the build it ran on, and a mislabelled Run is exactly the thing it cannot
// survive.
//
// WHAT THIS DOES NOT CLAIM. ADR-023 決策 1 is unchanged and is the reason this
// check is one-directional: 「最終事實來源是 image digest，不是任何一個檔案裡的版
// 本字串。」 So the digest still decides what actually ran; nothing here can see
// it, and a node handed a digest whose SDK differs from every string in this
// repository is a deployment error this cannot detect. What is checkable from
// the repository is that the copies of the pinned string agree with the pin,
// which is what the same ADR's 「sandboxd 的同名預設值…須與 Image 內一致」
// (apps/sandbox/README.md's own wording) already asks of a human. This is
// bookkeeping, not isolation.
//
// WHY NOT one-number. That mechanism is the natural first thought and it does
// not reach here, for three reasons that are each fatal on their own: its value
// pattern reads the last integer on the line, so `0.3.233` and `1.0.233` are
// both "233"; its roots are apps/contracts/db/tools and its extensions are
// source files, so neither infra/…/Dockerfile nor a .md table is scanned; and
// its markers are `//` or `#` comments, which a markdown table cell has no way
// to carry. Widening it five ways to hold one value would trade a strictness
// its own comment defends for a value that does not need it — one-number
// compares peers, and these sites are not peers. The Dockerfile is declared
// upstream of the other two, so the copies are derived and compared, and there
// is no marker for anyone to remember to add.
const sdkVersionDockerfile = "infra/images/runtime-agent-sdk/Dockerfile"

// The pin itself. Anchored to the ARG line, not to the value: `${…}`
// interpolations of the same name appear twice further down the Dockerfile
// (the npm install and the image's own SKILLHUB_RUNTIME_VERSION env) and must
// not be mistaken for a second author of it.
var sdkVersionARG = regexp.MustCompile(`^\s*ARG\s+CLAUDE_AGENT_SDK_VERSION\s*=\s*(\S+)`)

type sdkVersionSite struct {
	file string
	// what the site is, in the failure message, phrased to finish the sentence
	// "… says <value> (<what>)".
	what    string
	pattern *regexp.Regexp
}

// The copies. Each pattern is anchored to the environment variable's name,
// which is the part of these lines that is stable — the surrounding Go call and
// the markdown table around it are not.
var sdkVersionSites = []sdkVersionSite{
	{
		file:    "apps/sandbox/cmd/sandboxd/main.go",
		what:    "the compiled default sandboxd advertises in ProviderCapability, which dispatch freezes into runs.runtime_snapshot as the run's permanent runtime_version, I-05",
		pattern: regexp.MustCompile(`SKILLHUB_SANDBOX_RUNTIME_VERSION"\s*,\s*"([^"]*)"`),
	},
	{
		file:    "apps/sandbox/README.md",
		what:    "the default an operator reads out of the settings table",
		pattern: regexp.MustCompile("`SKILLHUB_SANDBOX_RUNTIME_VERSION`\\s*\\|\\s*`([^`]*)`"),
	},
}

// infra/images/runtime-agent-sdk/UPGRADES.md is deliberately not a site: it is
// ADR-023 §4's append-only ledger, so its older rows are supposed to name older
// versions and a check that demanded agreement would demand the history be
// rewritten. Neither are the `0.3.233` literals in apps/sandbox/internal/**
// tests: those are fixtures asserting on a value they also supply, and pinning
// them here would make this check argue with a test about what it is testing.

func sdkVersionProblems(root string) []string {
	pinned, pinnedLine, problems := sdkVersionAt(
		root, sdkVersionDockerfile, sdkVersionARG, "ARG CLAUDE_AGENT_SDK_VERSION")
	if len(problems) > 0 {
		return problems
	}
	// A range is not a version, and comparing one against the exact strings the
	// copies carry would report a difference on every future upgrade while
	// meaning nothing. ADR-023 決策 1 bars these from the build path outright.
	if strings.ContainsAny(pinned, "^~*>=<") || strings.Contains(pinned, "latest") {
		return []string{fmt.Sprintf(
			"sdk-version: %s:%d pins CLAUDE_AGENT_SDK_VERSION=%s, which is a range rather than an exact "+
				"version; ADR-023 決策 1 bars ^, ~ and latest from the image build path, and a range cannot "+
				"be what the copies of it say",
			sdkVersionDockerfile, pinnedLine, pinned)}
	}

	for _, site := range sdkVersionSites {
		value, line, siteProblems := sdkVersionAt(root, site.file, site.pattern, "SKILLHUB_SANDBOX_RUNTIME_VERSION")
		if len(siteProblems) > 0 {
			problems = append(problems, siteProblems...)
			continue
		}
		if value == pinned {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"sdk-version: %s:%d pins the Agent SDK at %s but %s:%d says %s (%s). ADR-023 決策 1 makes the "+
				"image the source of truth and the copies follow it, so bump them in the same commit — and "+
				"if this is an upgrade, ADR-023 §2's four measurements and the %s row are due with it",
			sdkVersionDockerfile, pinnedLine, pinned, site.file, line, value, site.what,
			"infra/images/runtime-agent-sdk/UPGRADES.md"))
	}
	return problems
}

// sdkVersionAt reads the one line in a file that states the version. Every way
// of not finding exactly one is loud: this repository's recurring failure is a
// check that quietly stops finding its subject and passes, and a copy that has
// been reworded is precisely a copy nothing is comparing any more.
func sdkVersionAt(root, relative string, pattern *regexp.Regexp, subject string) (string, int, []string) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", 0, []string{fmt.Sprintf("sdk-version: cannot read %s: %v", relative, err)}
	}
	var values []string
	var lines []int
	for i, line := range strings.Split(string(data), "\n") {
		if m := pattern.FindStringSubmatch(line); m != nil {
			values = append(values, m[1])
			lines = append(lines, i+1)
		}
	}
	switch len(values) {
	case 1:
		return values[0], lines[0], nil
	case 0:
		return "", 0, []string{fmt.Sprintf(
			"sdk-version: %s no longer states %s in a shape this check recognises; the Agent SDK version is "+
				"still written there and nothing is comparing it with the pin any more. This check has lost "+
				"part of its subject", relative, subject)}
	default:
		return "", 0, []string{fmt.Sprintf(
			"sdk-version: %s states %s %d times (lines %v); the pinned SDK version cannot have two authors "+
				"in one file", relative, subject, len(values), lines)}
	}
}
