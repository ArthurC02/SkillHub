package eval

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

const (
	suggestTimeout      = 135 * time.Second // budget-over: evaluate.LLM_TIMEOUT_SECONDS
	maxDigestChars      = 20000             // one-number: suggestMaxDigestChars
	maxFileTreeEntries  = 500               // one-number: suggestMaxFileTreeEntries
	maxTargetFiles      = 5
	maxTargetFileChars  = 60000 // one-number: suggestMaxProposedContent
	maxStoredEvidence   = 2
	maxDigestEvidence   = 8
	maxSuggestionsStore = 10 // one-number: suggestMaxSuggestions
)

// SuggestImprovementsBudget names this call for an operator and bounds what they may set.
var SuggestImprovementsBudget = modelbudget.Endpoint{
	Kind: "suggest-improvements", Deadline: suggestTimeout,
}

func (s *Service) suggest(ctx context.Context, m facts.Material, ev gen.Evaluation, v assessment) {
	if s.Suggester == nil || m.Skill.AccessRestricted || !worthSuggesting(v) {
		return
	}

	digest, refs := suggestionDigest(m, v)

	if len(refs) == 0 {
		slog.Info("improvement proposals skipped: verdict carries no citable evidence",
			"evaluation_id", pgconv.UUIDString(ev.ID))
		return
	}
	tree, files := s.packageFiles(ctx, m)

	callCtx, cancel := context.WithTimeout(ctx, suggestTimeout)
	defer cancel()
	resp, err := s.Suggester.SuggestImprovements(callCtx, modelport.ImprovementRequest{
		EvaluationID:     pgconv.UUIDString(ev.ID),
		EvaluationDigest: digest,
		FileTree:         tree,
		TargetFiles:      files,
		Within:           s.Budgets.Within(ctx, SuggestImprovementsBudget),
	})
	if err != nil {
		slog.Warn("evaluation suggestions unavailable",
			"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)
		return
	}
	ref := evaluationRef{workspaceID: ev.WorkspaceID, evaluationID: ev.ID, runID: m.Run.ID}
	call := modelCall{model: resp.Model, promptVersion: resp.PromptVersion, usage: resp.Usage}
	if err := s.recordModelUsage(ctx, s.queries(), ref, "suggest", call); err != nil {
		slog.Warn("evaluation suggestion usage not stored",
			"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)

	}

	s.recordEvalCost(ctx, s.Pool, credit.KindSuggestion, ref, call)

	s.storeProposals(ctx, ev, resp.Proposals, refs)
}

func (s *Service) storeProposals(ctx context.Context, ev gen.Evaluation, proposals []modelport.ImprovementProposal, refs []verdict.EvidenceRef) {
	q := s.queries()

	stored, noEvidence, unstorable, writeFailed, overCap := 0, 0, 0, 0, 0
	for _, p := range proposals {
		if stored == maxSuggestionsStore {
			overCap = len(proposals) - stored - noEvidence - unstorable - writeFailed
			break
		}
		evidence, err := suggestionEvidence(p, refs)
		if err != nil || len(evidence) == 0 {

			quote := strings.TrimSpace(p.Evidence)
			head, _ := cut(quote, loggedQuoteHeadRunes)
			slog.Warn("improvement proposal has no matching verified evidence",
				"evaluation_id", pgconv.UUIDString(ev.ID), "category", p.Category,
				"quote_runes", utf8.RuneCountInString(quote), "candidate_refs", len(refs),
				"quote_head", head)
			noEvidence++
			continue
		}
		if !storable(p) {

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

	slog.Info("improvement proposals",
		"evaluation_id", pgconv.UUIDString(ev.ID),
		"proposed", len(proposals),
		"stored", stored,
		"dropped_no_evidence", noEvidence,
		"dropped_unstorable", unstorable,
		"dropped_write_failed", writeFailed,
		"dropped_over_cap", overCap)
}

func worthSuggesting(v assessment) bool {
	if v.overall != verdict.OverallMet {
		return true
	}
	for _, f := range v.findings {
		if f.Severity != verdict.SeverityInfo {
			return true
		}
	}
	return false
}

func storable(p modelport.ImprovementProposal) bool {
	if !SuggestionCategory(p.Category).actionable() {
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

func suggestionEvidence(p modelport.ImprovementProposal, refs []verdict.EvidenceRef) ([]byte, error) {
	out := make([]verdict.EvidenceRef, 0, maxStoredEvidence)
	for _, quote := range evidenceQuotes(p.Evidence) {
		for _, ref := range refs {
			if strings.Contains(ref.Excerpt, quote) {
				out = append(out, ref)
				break
			}
		}
		if len(out) > 0 {
			break
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return json.Marshal(out)
}

var quoteMarks = [][2]string{{"「", "」"}, {"“", "”"}, {"\"", "\""}}

func evidenceQuotes(evidence string) []string {
	const minQuoteRunes = 12
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) >= minQuoteRunes {
			out = append(out, s)
		}
	}

	add(evidence)
	for _, pair := range quoteMarks {
		rest := evidence
		for {
			i := strings.Index(rest, pair[0])
			if i < 0 {
				break
			}
			rest = rest[i+len(pair[0]):]
			j := strings.Index(rest, pair[1])
			if j < 0 {
				break
			}
			add(rest[:j])
			rest = rest[j+len(pair[1]):]
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return utf8.RuneCountInString(out[i]) > utf8.RuneCountInString(out[j])
	})
	return out
}

const (
	loggedQuoteHeadRunes     = 80
	digestExcerptRunes       = 400
	digestCriterionTextRunes = 500
	digestExplanationRunes   = 800
)

func suggestionDigest(m facts.Material, v assessment) (string, []verdict.EvidenceRef) {
	var b strings.Builder
	refs := make([]verdict.EvidenceRef, 0, maxDigestEvidence)
	addRefs := func(in []verdict.EvidenceRef) {
		for _, r := range in {
			if len(refs) >= maxDigestEvidence {
				return
			}
			refs = append(refs, r)
			if b.Len() < maxDigestChars {
				excerpt, _ := cut(r.Excerpt, digestExcerptRunes)

				fmt.Fprintf(&b, "    evidence (%s):\n      %s\n", r.Kind, excerpt)
			}
		}
	}

	fmt.Fprintf(&b, "run %s finished as %s; task verdict: %s\n",
		pgconv.UUIDString(m.Run.ID), m.Run.Status, v.overall)
	fmt.Fprintf(&b, "user prompt: %s\n", firstChars(m.Snapshot.UserPrompt, 1000))
	if v.summary != "" {
		fmt.Fprintf(&b, "summary: %s\n", firstChars(v.summary, 1000))
	}

	b.WriteString("\nacceptance criteria that did not pass:\n")
	unmet := 0
	for _, r := range v.results {
		if r.Result == verdict.ResultPassed {
			continue
		}
		unmet++
		fmt.Fprintf(&b, "  [%s] %s -> %s: %s\n",
			r.CriterionID, firstChars(r.Text, digestCriterionTextRunes), r.Result, firstChars(r.Reason, digestExplanationRunes))
		addRefs(r.Evidence)
	}
	if unmet == 0 {
		b.WriteString("  (none)\n")
	}

	b.WriteString("\nchecks the platform ran on this run:\n")
	for _, f := range v.findings {
		if f.Severity == verdict.SeverityInfo {
			continue
		}
		fmt.Fprintf(&b, "  [%s/%s] %s\n", f.Category, f.Severity, firstChars(f.Message, digestExplanationRunes))
		addRefs(f.Evidence)
	}

	digest, _ := cut(b.String(), maxDigestChars)
	return digest, refs
}

func (s *Service) packageFiles(ctx context.Context, m facts.Material) ([]string, []modelport.TargetFile) {
	if s.Store == nil || m.Version.PackageObjectKey == "" {
		return nil, nil
	}
	fsys, _, err := s.readPackage(ctx, m.Version.Stored())
	if err != nil {
		slog.Warn("suggestion inputs unavailable", "error", err)
		return nil, nil
	}

	var tree []string
	skillpkg.EachReadableFile(fsys, func(p string, _ fs.DirEntry) {
		if len(tree) < maxFileTreeEntries {
			tree = append(tree, p)
		}
	})
	sort.Strings(tree)

	files := make([]modelport.TargetFile, 0, maxTargetFiles)
	for _, p := range skillEntryFirst(tree) {
		if len(files) == maxTargetFiles {
			break
		}
		content, err := readTarget(fsys, p)
		if err != nil || content == "" || len([]rune(content)) > maxTargetFileChars {
			continue
		}
		files = append(files, modelport.TargetFile{Path: p, Content: content})
	}
	return tree, files
}

func skillEntryFirst(tree []string) []string {
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
	return ordered
}

func firstChars(s string, limit int) string {
	out, truncated := cut(s, limit)
	if truncated {
		return out + "…[truncated]"
	}
	return out
}
