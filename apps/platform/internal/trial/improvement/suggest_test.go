package eval

// Unit coverage for the parts of EVAL-002 that decide what may be stored and what
// may be written: the path rule, the value domain, the evidence a suggestion is
// allowed to carry, and the archive rewrite that a new version is built from.
//
// The rules under test are the ones a model's output has to get past. They are
// tested without a database because none of them asks one anything.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestSuggestionDigestNeverShowsEvidenceOutsideItsAllowlist(t *testing.T) {
	v := verdict{overall: OverallNotMet, findings: []Finding{{
		Category: CategoryEffect, Severity: SeverityWarning, Message: "needs work",
	}}}
	for i := 0; i < maxDigestEvidence+1; i++ {
		v.findings[0].Evidence = append(v.findings[0].Evidence, EvidenceRef{
			Kind: KindAgentOutput, Excerpt: strings.Repeat(string(rune('a'+i)), 20), Available: true,
		})
	}
	digest, refs := suggestionDigest(material{}, v)
	if len(refs) != maxDigestEvidence {
		t.Fatalf("allowlist has %d refs, want %d", len(refs), maxDigestEvidence)
	}
	if strings.Contains(digest, strings.Repeat(string(rune('a'+maxDigestEvidence)), 20)) {
		t.Fatal("prompt exposed evidence that was not in the verification allowlist")
	}
}

func TestTargetPathsThatLeaveThePackageAreRefusedNotRepaired(t *testing.T) {
	for _, in := range []string{
		"", "   ", "/etc/passwd", "../secrets.txt", "docs/../../out", "./SKILL.md",
		"scripts//run.py", "..", ".", `scripts\run.py`,
	} {
		if got, ok := cleanTargetPath(in); ok {
			t.Errorf("cleanTargetPath(%q) = %q, accepted; a path that escapes or needed "+
				"repairing is not the path the proposal meant", in, got)
		}
	}
	for _, in := range []string{"SKILL.md", "scripts/run.py", "docs/a/b.md"} {
		if got, ok := cleanTargetPath(in); !ok || got != in {
			t.Errorf("cleanTargetPath(%q) = %q, %v; want it unchanged and accepted", in, got, ok)
		}
	}
}

func TestOnlyProposalsThePlatformCanActOnAreStored(t *testing.T) {
	ok := llmclient.ImprovementProposal{
		Category: "skill", Problem: "the description does not mention xlsx",
		TargetPath: "SKILL.md", ProposedContent: "new", ExpectedImpact: "activation improves",
	}
	if !storable(ok) {
		t.Fatal("a complete, in-bounds proposal must be storable")
	}

	cases := map[string]func(p *llmclient.ImprovementProposal){
		// `mcp` is a placeholder the MVP never acts on: storing one would ask a user
		// to decide about something that could not be applied either way.
		"mcp category":     func(p *llmclient.ImprovementProposal) { p.Category = "mcp" },
		"unknown category": func(p *llmclient.ImprovementProposal) { p.Category = "prompt" },
		"escaping path":    func(p *llmclient.ImprovementProposal) { p.TargetPath = "../x" },
		"no problem":       func(p *llmclient.ImprovementProposal) { p.Problem = "  " },
		"no impact":        func(p *llmclient.ImprovementProposal) { p.ExpectedImpact = "" },
		"no content":       func(p *llmclient.ImprovementProposal) { p.ProposedContent = "" },
	}
	for name, mutate := range cases {
		p := ok
		mutate(&p)
		if storable(p) {
			t.Errorf("%s: proposal was accepted for storage", name)
		}
	}
}

func TestSuggestionEvidenceIsAlwaysMintedByThePlatform(t *testing.T) {
	refs := []EvidenceRef{
		{Kind: KindTraceEvent, TraceEventID: eventID, Excerpt: "tool_call bash exited 1", Available: true},
		{Kind: KindArtifact, ArtifactPath: "output.xlsx", Excerpt: "output.xlsx (4096 bytes)", Available: true},
	}

	// A quote the platform can find in its own verified material keeps that
	// reference, pointer and all.
	raw, err := suggestionEvidence(llmclient.ImprovementProposal{Evidence: "bash exited 1"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	var got []EvidenceRef
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TraceEventID != eventID {
		t.Fatalf("a quote found in a verified reference keeps it, got %+v", got)
	}

	// A quote the platform cannot find does not borrow unrelated citations.
	raw, _ = suggestionEvidence(llmclient.ImprovementProposal{
		Evidence: "the model is quite sure something went wrong",
	}, refs)
	if raw != nil {
		t.Fatalf("an unmatched quote must make the proposal unstorable, got %s", raw)
	}
	raw, _ = suggestionEvidence(llmclient.ImprovementProposal{Evidence: "e"}, refs)
	if raw != nil {
		t.Fatalf("a generic one-character substring must not become evidence: %s", raw)
	}

	// An evaluation with no references cannot support a stored suggestion.
	raw, _ = suggestionEvidence(llmclient.ImprovementProposal{Evidence: "x"}, nil)
	if raw != nil {
		t.Errorf("no evidence available must make the proposal unstorable, got %s", raw)
	}
}

func TestAdviceIsNotPaidForOnARunThatMetEverything(t *testing.T) {
	met := verdict{overall: OverallMet, findings: []Finding{{Severity: SeverityInfo}}}
	if worthSuggesting(met) {
		t.Error("a met verdict with nothing but disclosures has nothing to improve")
	}
	if !worthSuggesting(verdict{overall: OverallMet, findings: []Finding{{Severity: SeverityWarning}}}) {
		t.Error("a met verdict with a warning still deserves advice")
	}
	if !worthSuggesting(verdict{overall: OverallNotMet}) {
		t.Error("an unmet verdict is exactly the case suggestions exist for")
	}
}

// --- the archive rewrite -------------------------------------------------------

// zipWithRoot builds an archive shaped like a GitHub download: everything inside
// one top-level directory, which is the case a second root-finding rule gets
// wrong by writing the new file beside the package instead of into it.
func zipWithRoot(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, body := range files {
		w, err := zw.Create(root + path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPatchingKeepsThePackageRootAndEveryUntouchedFile(t *testing.T) {
	const skillMD = "---\nname: demo\ndescription: A demo.\nlicense: MIT\n---\n\nOld body.\n"
	original := zipWithRoot(t, "demo-main/", map[string]string{
		"SKILL.md":       skillMD,
		"scripts/run.py": "print('hello')\n",
	})

	patched, err := patchArchive(original, map[string]string{
		"SKILL.md":    skillMD + "\nNew paragraph.\n",
		"docs/new.md": "added by a suggestion\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	fsys, err := skillpkg.PackageFS(patched)
	if err != nil {
		t.Fatalf("the patched archive is no longer a readable package: %v", err)
	}
	body, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil || !bytes.Contains(body, []byte("New paragraph.")) {
		t.Fatalf("the replaced file did not land inside the package root: %v / %q", err, body)
	}
	added, err := fs.ReadFile(fsys, "docs/new.md")
	if err != nil || string(added) != "added by a suggestion\n" {
		t.Fatalf("a file the package did not have must be added inside the root: %v / %q", err, added)
	}
	untouched, err := fs.ReadFile(fsys, "scripts/run.py")
	if err != nil || string(untouched) != "print('hello')\n" {
		t.Fatalf("an untouched file changed: %v / %q", err, untouched)
	}

	// The original bytes are still the original bytes: patching returns a new
	// archive and never edits the stored one (iron rule 4 in the small).
	if same, _ := skillpkg.PackageFS(original); same != nil {
		if before, _ := fs.ReadFile(same, "SKILL.md"); string(before) != skillMD {
			t.Error("patching modified the archive it was given")
		}
	}
}

func TestPatchingIsDeterministicForTheSameInput(t *testing.T) {
	original := zipWithRoot(t, "", map[string]string{"SKILL.md": "a\n", "b.md": "b\n"})
	patches := map[string]string{"SKILL.md": "changed\n", "z.md": "z\n", "a.md": "a\n"}
	first, err := patchArchive(original, patches)
	if err != nil {
		t.Fatal(err)
	}
	second, err := patchArchive(original, patches)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two identical applications produced different archives, so INGEST-005's " +
			"content-hash dedupe would never recognise a re-application")
	}
}
