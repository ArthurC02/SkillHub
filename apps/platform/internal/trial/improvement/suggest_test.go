package eval

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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

func TestTheDigestKeepsItsOwnLabelsOffTheQuotableLines(t *testing.T) {
	const excerpt = "bash exited 1 while writing output.xlsx"
	v := verdict{overall: OverallNotMet, findings: []Finding{{
		Category: CategoryEffect, Severity: SeverityWarning, Message: "needs work",
		Evidence: []EvidenceRef{{Kind: KindAgentOutput, Excerpt: excerpt, Available: true}},
	}}}
	digest, refs := suggestionDigest(material{}, v)
	if len(refs) != 1 {
		t.Fatalf("this fixture stopped producing one ref: %d", len(refs))
	}

	for _, line := range strings.Split(digest, "\n") {
		if !strings.Contains(line, "evidence (") {
			continue
		}

		if !strings.HasSuffix(strings.TrimRight(line, " "), ":") {
			t.Fatalf("label and content share a line, so a verbatim quote of it "+
				"can never be verified: %q", line)
		}
	}
	if !strings.Contains(digest, excerpt) {
		t.Fatal("the excerpt itself is gone; the split dropped what it was meant to isolate")
	}

	if raw, _ := suggestionEvidence(llmclient.ImprovementProposal{
		Evidence: `"evidence (agent_output): ` + excerpt + `"`,
	}, refs); raw != nil {
		t.Fatal("a quote carrying the platform's own label resolved; if that ever " +
			"becomes true, the label is content and this test is the wrong shape")
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

	raw, _ = suggestionEvidence(llmclient.ImprovementProposal{Evidence: "x"}, nil)
	if raw != nil {
		t.Errorf("no evidence available must make the proposal unstorable, got %s", raw)
	}
}

func TestAProposalMayExplainItselfAroundTheQuoteItCites(t *testing.T) {
	refs := []EvidenceRef{
		{Kind: KindTraceEvent, TraceEventID: eventID, Available: true,
			Excerpt: "artifact manifest: 完整 artifact manifest 明確顯示本次 run 未寫入任何檔案"},
	}

	for _, evidence := range []string{
		`「完整 artifact manifest 明確顯示本次 run 未寫入任何檔案」，所以 /out/artifacts/ 是空的。`,
		`"完整 artifact manifest 明確顯示本次 run 未寫入任何檔案" and the summary agrees.`,
		`“完整 artifact manifest 明確顯示本次 run 未寫入任何檔案” — nothing was saved.`,
	} {
		raw, err := suggestionEvidence(llmclient.ImprovementProposal{Evidence: evidence}, refs)
		if err != nil {
			t.Fatal(err)
		}
		var got []EvidenceRef
		if raw == nil {
			t.Fatalf("a verbatim quote with reasoning around it must still be citable: %s", evidence)
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].TraceEventID != eventID {
			t.Fatalf("must keep the reference the quote was found in, got %+v", got)
		}
	}

	for _, evidence := range []string{
		`「the manifest shows nothing was written at all」 which is the problem.`,
		`no quotation marks here, and none of this text is in any excerpt either`,
		`「too short」`,
	} {
		if raw, _ := suggestionEvidence(llmclient.ImprovementProposal{Evidence: evidence}, refs); raw != nil {
			t.Errorf("unverifiable evidence must stay unstorable, got %s for %s", raw, evidence)
		}
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

type stubSuggester struct {
	resp llmclient.SuggestImprovementsResponse
}

func (s stubSuggester) SuggestImprovements(
	context.Context, llmclient.SuggestImprovementsRequest,
) (*llmclient.SuggestImprovementsResponse, error) {
	r := s.resp
	return &r, nil
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func findRecord(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if rec["msg"] == msg {
			found = append(found, rec)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one %q record, got %d in:\n%s", msg, len(found), buf.String())
	}
	return found[0]
}

func TestTheProposalCountersAreEmittedEvenWhenNothingWasDropped(t *testing.T) {
	const excerpt = "bash exited 1 while writing output.xlsx"
	newVerdict := func() verdict {
		return verdict{overall: OverallNotMet, findings: []Finding{{
			Category: CategoryEffect, Severity: SeverityWarning, Message: "needs work",
			Evidence: []EvidenceRef{{Kind: KindAgentOutput, Excerpt: excerpt, Available: true}},
		}}}
	}

	for _, tc := range []struct {
		name        string
		suggestions []llmclient.ImprovementProposal
		want        map[string]float64
	}{{

		name: "all clear",
		want: map[string]float64{
			"proposed": 0, "stored": 0, "dropped_no_evidence": 0,
			"dropped_unstorable": 0, "dropped_write_failed": 0, "dropped_over_cap": 0,
		},
	}, {

		name: "one refused before storage",
		suggestions: []llmclient.ImprovementProposal{{
			Category: "mcp", Problem: "remote MCP would help", Evidence: excerpt,
			TargetPath: "SKILL.md", ProposedContent: "new", ExpectedImpact: "better",
		}},
		want: map[string]float64{
			"proposed": 1, "stored": 0, "dropped_no_evidence": 0,
			"dropped_unstorable": 1, "dropped_write_failed": 0, "dropped_over_cap": 0,
		},
	}, {

		name: "one uncitable",
		suggestions: []llmclient.ImprovementProposal{{
			Category: "skill", Problem: "the description is vague",
			Evidence:   "the model is quite sure something went wrong somewhere",
			TargetPath: "SKILL.md", ProposedContent: "new", ExpectedImpact: "better",
		}},
		want: map[string]float64{
			"proposed": 1, "stored": 0, "dropped_no_evidence": 1,
			"dropped_unstorable": 0, "dropped_write_failed": 0, "dropped_over_cap": 0,
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLogs(t)
			s := &Service{Suggester: stubSuggester{resp: llmclient.SuggestImprovementsResponse{
				Suggestions: tc.suggestions, Model: "test", PromptVersion: "test",
			}}}
			s.suggest(context.Background(), material{}, gen.Evaluation{}, newVerdict())

			rec := findRecord(t, buf, "improvement proposals")
			for key, want := range tc.want {
				got, ok := rec[key].(float64)
				if !ok {
					t.Errorf("%s is missing from the counters: %v", key, rec)
					continue
				}
				if got != want {
					t.Errorf("%s = %v, want %v", key, got, want)
				}
			}
		})
	}
}
