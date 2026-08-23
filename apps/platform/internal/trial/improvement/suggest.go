package eval

// The proposal leg of EVAL-002: asking apps/llm what to change, and storing
// only what survives Go's own checks.
//
// It runs after the verdict is stored, not instead of part of it. A suggestion is
// an opinion about a completed evaluation, so a gateway failure here loses the
// opinion and nothing else — the evaluation stays `completed` and is not retried,
// because re-running it would pay for a second judgement to reach the verdict
// that is already written (llm-internal.yaml: a 502 from this endpoint is not
// retried).
//
// What comes back is a proposal and nothing more (iron rule 6). This file decides
// the value domain and the path rules; the user decides acceptance; apply.go
// decides whether the accepted ones can become a version.

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// The decision vocabulary of 0024 (EVAL-002 clause 3).
const (
	DecisionPending  = "pending"
	DecisionAccepted = "accepted"
	DecisionRejected = "rejected"
)

// The problem classes of EVAL-002 clause 2. `mcp` exists in the column's CHECK as
// a type placeholder and is deliberately absent here: remote MCP is out of the
// first release, so a proposal in that class could not be acted on even if a model
// produced one (same treatment TRACE-003 gives its MCP events).
var suggestionCategories = map[string]bool{
	"skill": true, "runtime": true, "tool": true, "dataset": true,
}

// Caps for one suggestion request, from llm-internal.yaml's schema and
// evaluation-design §6.3's reasoning: the cost of these calls is almost entirely
// input, so it is bounded by cutting rather than by hoping.
const (
	suggestTimeout      = 120 * time.Second
	maxDigestChars      = 20000 // SuggestImprovementsRequest.evaluation_digest
	maxFileTreeEntries  = 500   // .file_tree maxItems
	maxTargetFiles      = 5     // contract allows 10; five files is what one round of advice needs
	maxTargetFileChars  = 60000 // TargetFile.content maxLength
	maxStoredEvidence   = 2     // evidence refs carried onto one stored suggestion
	maxDigestEvidence   = 8     // refs quoted into the digest, and the set the above draws from
	maxSuggestionsStore = 10    // SuggestImprovementsResponse.suggestions maxItems
)

// Suggester is the improvement-proposal leg. One implementation (llmclient.Client)
// plus the test double, same seam as Judge.
type Suggester interface {
	SuggestImprovements(ctx context.Context, req llmclient.SuggestImprovementsRequest) (
		*llmclient.SuggestImprovementsResponse, error)
}

// suggest proposes improvements for a completed evaluation and stores what passes.
//
// Failures are logged and dropped: an evaluation with no suggestions is a complete
// evaluation, while an error returned here would fail a job whose real work — the
// verdict — is already committed and would be redone on the retry.
func (s *Service) suggest(ctx context.Context, m material, ev gen.Evaluation, v verdict) {
	if s.Suggester == nil || !worthSuggesting(v) {
		return
	}

	digest, refs := suggestionDigest(m, v)
	tree, files := s.packageFiles(ctx, m)

	// The internal call carries this caller's deadline and cancellation (iron rule
	// 7); llmclient has no timeout of its own.
	callCtx, cancel := context.WithTimeout(ctx, suggestTimeout)
	defer cancel()
	resp, err := s.Suggester.SuggestImprovements(callCtx, llmclient.SuggestImprovementsRequest{
		EvaluationID:     pgconv.UUIDString(ev.ID),
		EvaluationDigest: digest,
		FileTree:         tree,
		TargetFiles:      files,
	})
	if err != nil {
		slog.Warn("evaluation suggestions unavailable",
			"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)
		return
	}
	if resp.Usage != nil && resp.Usage.CostUSD != nil && resp.Usage.CostSource != "gateway" {
		resp.Usage.CostUSD = nil
		resp.Usage.CostSource = ""
	}
	if err := s.recordModelUsage(ctx, ev.ID, ev.WorkspaceID, "suggest",
		resp.Model, resp.PromptVersion, resp.Usage); err != nil {
		slog.Warn("evaluation suggestion usage not stored",
			"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)
		// The paid model result is still useful. Accounting availability is not
		// permission to discard proposals after a successful external call.
	}

	q := s.queries()
	// The counters exist because 04 丙-38 asked "what fraction of proposals gets
	// thrown away", and the answer was unobservable: every drop was a slog.Warn
	// and nothing counted them, so the only recoverable number was how many
	// survived. A rate needs its denominator, and the denominator was being
	// discarded one line at a time.
	stored, noEvidence, unstorable, writeFailed, overCap := 0, 0, 0, 0, 0
	for _, p := range resp.Suggestions {
		if stored == maxSuggestionsStore {
			overCap = len(resp.Suggestions) - stored - noEvidence - unstorable - writeFailed
			break
		}
		evidence, err := suggestionEvidence(p, refs)
		if err != nil || len(evidence) == 0 {
			slog.Warn("improvement proposal has no matching verified evidence",
				"evaluation_id", pgconv.UUIDString(ev.ID), "category", p.Category)
			noEvidence++
			continue
		}
		if !storable(p) {
			// Dropped rather than stored with a note: a proposal whose path escapes
			// the package, or whose class this platform cannot act on, is not
			// something a user should be asked to decide about.
			slog.Warn("improvement proposal refused before storage",
				"evaluation_id", pgconv.UUIDString(ev.ID), "category", p.Category)
			unstorable++
			continue
		}
		target, _ := cleanTargetPath(p.TargetPath)
		if _, err := q.CreateEvaluationSuggestion(ctx, gen.CreateEvaluationSuggestionParams{
			WorkspaceID:     ev.WorkspaceID,
			EvaluationID:    ev.ID,
			Category:        p.Category,
			Problem:         strings.TrimSpace(p.Problem),
			Evidence:        evidence,
			TargetPath:      target,
			ProposedContent: p.ProposedContent,
			ExpectedImpact:  strings.TrimSpace(p.ExpectedImpact),
		}); err != nil {
			slog.Warn("improvement proposal not stored",
				"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)
			writeFailed++
			continue
		}
		stored++
	}
	// One line per evaluation, always — including the all-clear. A counter that
	// is only emitted when something was dropped cannot distinguish "nothing was
	// dropped" from "this never ran", which is the same defect as a scan whose
	// absence reads as a pass.
	slog.Info("improvement proposals",
		"evaluation_id", pgconv.UUIDString(ev.ID),
		"proposed", len(resp.Suggestions),
		"stored", stored,
		"dropped_no_evidence", noEvidence,
		"dropped_unstorable", unstorable,
		"dropped_write_failed", writeFailed,
		"dropped_over_cap", overCap)
}

// worthSuggesting keeps the platform from paying a model to improve a run that met
// everything asked of it and produced no warning. Not a quality judgement: a met
// verdict with a warning still gets suggestions.
func worthSuggesting(v verdict) bool {
	if v.overall != OverallMet {
		return true
	}
	for _, f := range v.findings {
		if f.Severity != SeverityInfo {
			return true
		}
	}
	return false
}

// storable is the value domain, applied before anything reaches the database.
func storable(p llmclient.ImprovementProposal) bool {
	if !suggestionCategories[p.Category] {
		return false
	}
	if strings.TrimSpace(p.Problem) == "" || strings.TrimSpace(p.ExpectedImpact) == "" {
		return false
	}
	if p.ProposedContent == "" {
		return false
	}
	_, ok := cleanTargetPath(p.TargetPath)
	return ok
}

// suggestionEvidence gives a stored suggestion its EvidenceRefs.
//
// The model's own `evidence` is prose, and a sentence is not a citation: it is
// stored only when it can be matched to a reference the platform itself verified
// while judging. An unmatched sentence gets no citation; unrelated verified
// references elsewhere in the evaluation do not support this proposal.
func suggestionEvidence(p llmclient.ImprovementProposal, refs []EvidenceRef) ([]byte, error) {
	out := make([]EvidenceRef, 0, maxStoredEvidence)
	if quote := strings.TrimSpace(p.Evidence); utf8.RuneCountInString(quote) >= 12 {
		for _, ref := range refs {
			if strings.Contains(ref.Excerpt, quote) {
				out = append(out, ref)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return json.Marshal(out)
}

// suggestionDigest renders the verdict for the model and returns the references
// that rendering rests on.
//
// Go's own rendering, not the stored row: the same low-privilege reading rule the
// judge's trace digest follows. Everything in it has already been through the
// masker on its way into the trace (iron rule 11).
func suggestionDigest(m material, v verdict) (string, []EvidenceRef) {
	var b strings.Builder
	refs := make([]EvidenceRef, 0, maxDigestEvidence)
	addRefs := func(in []EvidenceRef) {
		for _, r := range in {
			if len(refs) >= maxDigestEvidence {
				return
			}
			refs = append(refs, r)
			if b.Len() < maxDigestChars {
				excerpt, _ := cut(r.Excerpt, 400)
				fmt.Fprintf(&b, "    evidence (%s): %s\n", r.Kind, excerpt)
			}
		}
	}

	fmt.Fprintf(&b, "run %s finished as %s; task verdict: %s\n",
		pgconv.UUIDString(m.run.ID), m.run.Status, v.overall)
	fmt.Fprintf(&b, "user prompt: %s\n", firstChars(m.snapshot.UserPrompt, 1000))
	if v.summary != "" {
		fmt.Fprintf(&b, "summary: %s\n", firstChars(v.summary, 1000))
	}

	b.WriteString("\nacceptance criteria that did not pass:\n")
	unmet := 0
	for _, r := range v.results {
		if r.Result == ResultPassed {
			continue
		}
		unmet++
		fmt.Fprintf(&b, "  [%s] %s -> %s: %s\n",
			r.CriterionID, firstChars(r.Text, 500), r.Result, firstChars(r.Reason, 800))
		addRefs(r.Evidence)
	}
	if unmet == 0 {
		b.WriteString("  (none)\n")
	}

	b.WriteString("\nchecks the platform ran on this run:\n")
	for _, f := range v.findings {
		if f.Severity == SeverityInfo {
			continue // disclosure, not a problem
		}
		fmt.Fprintf(&b, "  [%s/%s] %s\n", f.Category, f.Severity, firstChars(f.Message, 800))
		addRefs(f.Evidence)
	}

	digest, _ := cut(b.String(), maxDigestChars)
	return digest, refs
}

// packageFiles reads the file tree and the files a proposal may replace.
//
// Nothing is executed and nothing is parsed by extension (iron rule 1): the
// archive is opened as bytes and read as text. Oversized and binary files are
// skipped rather than truncated — a model shown half a file would propose a
// replacement that silently deletes the other half.
func (s *Service) packageFiles(ctx context.Context, m material) ([]string, []llmclient.TargetFile) {
	if s.Store == nil || m.version.PackageObjectKey == "" {
		return nil, nil
	}
	fsys, _, err := s.readPackage(ctx, m.version.PackageObjectKey)
	if err != nil {
		slog.Warn("suggestion inputs unavailable", "error", err)
		return nil, nil
	}

	var tree []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(tree) >= maxFileTreeEntries {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		tree = append(tree, p)
		return nil
	})
	sort.Strings(tree)

	// SKILL.md first: it is the file most advice is about, and the cap is small.
	ordered := make([]string, 0, len(tree))
	for _, p := range tree {
		if p == "SKILL.md" {
			ordered = append(ordered, p)
		}
	}
	for _, p := range tree {
		if p != "SKILL.md" {
			ordered = append(ordered, p)
		}
	}

	files := make([]llmclient.TargetFile, 0, maxTargetFiles)
	for _, p := range ordered {
		if len(files) == maxTargetFiles {
			break
		}
		content, err := readTarget(fsys, p)
		if err != nil || content == "" || len([]rune(content)) > maxTargetFileChars {
			continue
		}
		files = append(files, llmclient.TargetFile{Path: p, Content: content})
	}
	return tree, files
}

func firstChars(s string, limit int) string {
	out, truncated := cut(s, limit)
	if truncated {
		return out + "…[truncated]"
	}
	return out
}
