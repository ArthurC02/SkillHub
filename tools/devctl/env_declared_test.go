package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestEveryEnvironmentVariableTheServicesReadIsInTheTemplate(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := envDeclaredProblems(root); len(problems) > 0 {
		t.Fatalf("settings missing from .env.example:\n%s", strings.Join(problems, "\n"))
	}
}

const envFixtureWrappers = `package envx

import "os"

func OnUnlessOff(fallback bool, key string) bool {
	return Or(key, "") != "off" && fallback
}

func Or(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
`

const envFixtureMain = `package main

import (
	"os"

	"example/envx"
)

const releaseFile = "FIXTURE_RELEASES"

func main() {
	_ = os.Getenv("FIXTURE_DIRECT")
	_, _ = os.LookupEnv("FIXTURE_LOOKUP")
	_ = envx.Or("FIXTURE_WRAPPED", "x")
	_ = envx.OnUnlessOff(true, "FIXTURE_TWICE_WRAPPED")
	_ = os.Getenv(releaseFile)
	_ = os.Getenv("FIXTURE_TOKEN_" + suffix)
	_ = os.Getenv("lowercase_name")
	_ = strings.Replace("FIXTURE_NOT_A_READ", "", "", 1)
}
`

const envFixtureTest = `package main

import "os"

func TestOnly() { _ = os.Getenv("FIXTURE_TEST_ONLY") }
`

const envFixturePython = `import os

MODEL = os.getenv("FIXTURE_PY_GETENV") or "m"
TOKEN = os.environ.get("FIXTURE_PY_GET", "")
KEY = os.environ["FIXTURE_PY_INDEX"]
`

func writeEnvFixture(t *testing.T, extraMain string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "apps/platform/internal/foundation/envx/envx.go", envFixtureWrappers)
	writeAt(t, root, "apps/platform/cmd/api/main.go", envFixtureMain)
	writeAt(t, root, "apps/platform/cmd/api/main_test.go", envFixtureTest)
	writeAt(t, root, "apps/sandbox/cmd/sandboxd/main.go", "package main\n\nimport \"os\"\n\nfunc main() { _ = os.Getenv(\"FIXTURE_SANDBOX\")\n"+extraMain+"}\n")
	writeAt(t, root, "apps/llm/src/skillhub_llm/app.py", envFixturePython)
	return root
}

func TestTheEnvironmentScanFollowsWrappersConstantsAndPythonButNotTestsOrBuiltNames(t *testing.T) {
	t.Parallel()
	read, err := envVarsRead(writeEnvFixture(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"FIXTURE_DIRECT", "FIXTURE_LOOKUP", "FIXTURE_PY_GET", "FIXTURE_PY_GETENV", "FIXTURE_PY_INDEX",
		"FIXTURE_RELEASES", "FIXTURE_SANDBOX", "FIXTURE_TWICE_WRAPPED", "FIXTURE_WRAPPED",
	}
	if got := sortedKeys(read); !reflect.DeepEqual(got, want) {
		t.Fatalf("variables read = %q, want %q", got, want)
	}
	if read["FIXTURE_SANDBOX"] != "apps/sandbox/cmd/sandboxd/main.go" || read["FIXTURE_PY_INDEX"] != "apps/llm/src/skillhub_llm/app.py" {
		t.Fatalf("reads are not attributed to their files: %v", read)
	}
}

func manyEnvReads(n int) (reads, template string) {
	var r, e strings.Builder
	for i := range n {
		fmt.Fprintf(&r, "\t_ = os.Getenv(\"FIXTURE_MANY_%d\")\n", i)
		fmt.Fprintf(&e, "FIXTURE_MANY_%d=\n", i)
	}
	return r.String(), e.String()
}

const envFixtureTemplate = "FIXTURE_DIRECT=\nFIXTURE_LOOKUP=\nFIXTURE_PY_GET=\nFIXTURE_PY_GETENV=\n" +
	"FIXTURE_PY_INDEX=\nFIXTURE_RELEASES=\nFIXTURE_SANDBOX=\nFIXTURE_TWICE_WRAPPED=\nFIXTURE_WRAPPED=\n"

func TestAReadSettingMissingFromTheTemplateIsNamedWithTheFileThatReadsIt(t *testing.T) {
	t.Parallel()
	reads, many := manyEnvReads(envReadFloor)
	root := writeEnvFixture(t, reads)

	writeAt(t, root, ".env.example", envFixtureTemplate+many)
	if problems := envDeclaredProblems(root); len(problems) != 0 {
		t.Fatalf("a template listing every read setting was rejected: %v", problems)
	}

	writeAt(t, root, ".env.example", strings.Replace(envFixtureTemplate, "FIXTURE_TWICE_WRAPPED=\n", "# FIXTURE_TWICE_WRAPPED=\n", 1)+many)
	problems := envDeclaredProblems(root)
	want := "env-declared: apps/platform/cmd/api/main.go reads FIXTURE_TWICE_WRAPPED, which .env.example does not list"
	if len(problems) != 1 || !strings.HasPrefix(problems[0], want) {
		t.Fatalf("want exactly the FIXTURE_TWICE_WRAPPED problem, got %v", problems)
	}
}

func TestTheEnvironmentScanSaysSoWhenItFindsTooFewReads(t *testing.T) {
	t.Parallel()
	reads, many := manyEnvReads(envReadFloor - len(strings.Split(strings.TrimSpace(envFixtureTemplate), "\n")) - 1)
	root := writeEnvFixture(t, reads)
	writeAt(t, root, ".env.example", envFixtureTemplate+many)
	problems := envDeclaredProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "the scan is broken rather than the settings gone") {
		t.Fatalf("a scan one read below the floor was accepted: %v", problems)
	}

	reads, many = manyEnvReads(envReadFloor - len(strings.Split(strings.TrimSpace(envFixtureTemplate), "\n")))
	root = writeEnvFixture(t, reads)
	writeAt(t, root, ".env.example", envFixtureTemplate+many)
	if problems := envDeclaredProblems(root); len(problems) != 0 {
		t.Fatalf("a scan exactly at the floor was rejected: %v", problems)
	}
}
