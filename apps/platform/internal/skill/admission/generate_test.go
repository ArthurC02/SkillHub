package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// GEN-003. Every case here guards something that fails without a symptom: a
// package that validates but says the wrong thing about its licence, a retry
// that quietly re-runs a prompt it must not, a hash that changes for the same
// answer.

func goodGeneratedSkill() llmclient.GeneratedSkill {
	return llmclient.GeneratedSkill{
		Name:          "scanned-invoice-table",
		Description:   "從掃描的單據影像抽出表格內容並合併成一份檔案。當使用者手上是掃描件時使用。",
		Compatibility: "需要能讀取影像或 PDF 的工具。",
		Metadata:      map[string]string{"category": "documents", "audience": "finance"},
		AllowedTools:  "Read Write",
		Body:          "# 掃描單據轉表格\n\n1. 確認每份檔案是影像還是 PDF。\n2. 逐份抽出表格。\n",
	}
}

func validateGenerated(t *testing.T, g llmclient.GeneratedSkill) skillpkg.Report {
	t.Helper()
	data, err := buildGeneratedPackage(g)
	if err != nil {
		t.Fatalf("buildGeneratedPackage: %v", err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Fatalf("PackageFS: %v", err)
	}
	return skillpkg.Validate(fsys)
}

// The packaged answer has to survive the same validator an upload does — that
// reuse is the whole reason generation lives in this package.
func TestAGeneratedAnswerPassesTheImportValidator(t *testing.T) {
	r := validateGenerated(t, goodGeneratedSkill())
	if r.Blocked {
		t.Fatalf("blocked: %+v", r.Findings)
	}
	if r.Manifest.Name != "scanned-invoice-table" {
		t.Errorf("name = %q", r.Manifest.Name)
	}
	if r.Manifest.Compatibility == "" {
		t.Error("compatibility did not survive serialisation")
	}
}

// ADR-046 決策 5. The endpoint's schema has no licence property, so this is the
// second half of the same rule: even if one arrived by some other route, the
// frontmatter Go writes has nowhere to put it. A generated package whose licence
// read "MIT" would occupy the 已宣告 state, which means a person said so.
func TestGeneratedFrontmatterHasNoLicence(t *testing.T) {
	data, err := buildGeneratedPackage(goodGeneratedSkill())
	if err != nil {
		t.Fatal(err)
	}
	if bytes := string(data); strings.Contains(bytes, "license") {
		t.Error("the archive mentions a license key")
	}
	r := validateGenerated(t, goodGeneratedSkill())
	if r.Manifest.License != "" || r.LicenseExpression != "" {
		t.Errorf("licence leaked: manifest=%q expression=%q", r.Manifest.License, r.LicenseExpression)
	}
}

// Go map order is randomised, and content_hash is what INGEST-005 dedupes on.
// Without the sort this passes ~1 run in 2 and nothing ever reports why.
func TestTheSameAnswerAlwaysProducesTheSameBytes(t *testing.T) {
	first, err := buildGeneratedPackage(goodGeneratedSkill())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := buildGeneratedPackage(goodGeneratedSkill())
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("run %d produced different bytes for the same answer", i)
		}
	}
}

// 02:GEN-003: no warning removed and no risk downgraded because the platform
// wrote it. buildGeneratedPackage deliberately does not filter the path — it
// writes the entry and lets the archive-level check report it, which is the same
// treatment an uploaded package gets.
func TestAnEscapingFilePathIsBlockedNotFiltered(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []llmclient.GeneratedFile{{Path: "../../evil.sh", Content: "echo hi\n"}}
	r := validateGenerated(t, g)
	if !r.Blocked {
		t.Fatal("an escaping entry was accepted")
	}
	if !containsCode(r, "entry-path-escape") {
		t.Errorf("blocked for the wrong reason: %v", blockingCodes(r))
	}
}

// Two entries claiming one name, and which one a reader resolves to is not this
// package's decision to make silently.
func TestASecondSkillMDIsRefused(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []llmclient.GeneratedFile{{Path: "SKILL.md", Content: "---\nname: other\n---\n"}}
	if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
		t.Fatalf("err = %v, want ErrGeneratedPackageInvalid", err)
	}
}

// ADR-048. Delete the exception and this still passes every other test in the
// file: the only visible effect is a second paid call that reproduces the same
// credential-shaped line, which nothing anywhere reports.
func TestPossibleSecretIsNotRetried(t *testing.T) {
	secret := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityError, Code: skillpkg.CodePossibleSecret, Path: "setup.sh"},
	}}
	slip := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityError, Code: "frontmatter-invalid-yaml", Path: "SKILL.md"},
	}}
	both := skillpkg.Report{Findings: append(append([]skillpkg.Finding{}, slip.Findings...), secret.Findings...)}

	if shouldRetry(1, secret) {
		t.Error("a credential-shaped line bought a second attempt")
	}
	if !shouldRetry(1, slip) {
		t.Error("a formatting slip did not get its one retry")
	}
	if shouldRetry(1, both) {
		t.Error("mixed findings retried; 02:GEN-003 says not retrying wins")
	}
	if shouldRetry(generateMaxAttempts, slip) {
		t.Error("retried past the ceiling")
	}
}

// 02:GEN-001: a blank box must not reach the gateway. The service has no pool
// here, so anything that got past the check would panic rather than return —
// which is what makes this a test of the ordering and not only of the message.
func TestBlankTaskDescriptionNeverReachesTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	for _, in := range []string{"", "   ", "\n\t \n"} {
		if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrGenerateBlank) {
			t.Errorf("GenerateSkill(%q) err = %v, want ErrGenerateBlank", in, err)
		}
	}
}

// The generated source row must not be able to take self_supplied by looking
// like an upload — the conflation ADR-047 決策 4 rules against, and one with no
// symptom: both values release a download, so the only thing that changes is
// which question a future publishing path stops to ask.
func TestGeneratedTakesItsOwnRedistributionValue(t *testing.T) {
	ws := identity.Workspace{}
	if got := redistributionFor(ws, sourceMeta{Type: sourceGenerated}); got != "generated" {
		t.Errorf("generated -> %q", got)
	}
	if got := redistributionFor(ws, sourceMeta{Type: "upload"}); got == "generated" {
		t.Error("an upload took the generated value")
	}
	if got := redistributionFor(identity.Workspace{IsCatalog: true}, sourceMeta{Type: sourceGenerated}); got != "" {
		t.Errorf("the catalogue took %q", got)
	}
}

func containsCode(r skillpkg.Report, code string) bool {
	for _, c := range blockingCodes(r) {
		if c == code {
			return true
		}
	}
	return false
}

// The zip reader runs path.Clean over entry names, so these three resolve to
// SKILL.md while looking nothing like it. Missing one does not produce a
// collision anybody is told about: it produces `skill-md-missing` on a package
// that visibly contains SKILL.md, and then a second paid attempt at the same
// thing.
func TestTheSecondSkillMDIsCaughtUnderItsRealName(t *testing.T) {
	for _, p := range []string{"SKILL.md/", ".//SKILL.md", "././SKILL.md", "skill.MD", `.\SKILL.md`} {
		g := goodGeneratedSkill()
		g.Files = []llmclient.GeneratedFile{{Path: p, Content: "---\nname: other\n---\n"}}
		if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
			t.Errorf("path %q: err = %v, want ErrGeneratedPackageInvalid", p, err)
		}
	}
}

// An entry naming the archive root is not an escape, so ArchiveEntryFinding
// passes it — and then it sits inside content_hash and inside the stored archive
// while every disclosure surface skips it: scanTree never opens it, so
// `possible-secret` and the script disclosure never see it, and delivery's
// exporter neither ships it nor lists it as dropped. Model-authored bytes that
// no warning covers is what 02:GEN-003 forbids.
func TestAnEntryThatNamesNoFileIsRefused(t *testing.T) {
	for _, p := range []string{".", "./", "/", "././"} {
		g := goodGeneratedSkill()
		g.Files = []llmclient.GeneratedFile{{Path: p, Content: "AKIA0123456789ABCDEF\n"}}
		if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
			t.Errorf("path %q: err = %v, want ErrGeneratedPackageInvalid", p, err)
		}
	}
}
