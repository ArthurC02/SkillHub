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
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	DecisionPending  = "pending"
	DecisionAccepted = "accepted"
	DecisionRejected = "rejected"
)

var suggestionCategories = map[string]bool{
	"skill": true, "runtime": true, "tool": true, "dataset": true,
}

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

type Suggester interface {
	SuggestImprovements(ctx context.Context, req llmclient.SuggestImprovementsRequest) (
		*llmclient.SuggestImprovementsResponse, error)
}

func (s *Service) suggest(ctx context.Context, m material, ev gen.Evaluation, v verdict) {
	if s.Suggester == nil || !worthSuggesting(v) {
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
	if err := s.recordModelUsage(ctx, s.queries(), ev.ID, ev.WorkspaceID, "suggest",
		resp.Model, resp.PromptVersion, resp.Usage); err != nil {
		slog.Warn("evaluation suggestion usage not stored",
			"evaluation_id", pgconv.UUIDString(ev.ID), "error", err)

	}

	s.recordEvalCost(ctx, s.Pool, credit.KindSuggestion, ev.ID, ev.WorkspaceID, m.run.ID,
		resp.Model, resp.PromptVersion, resp.Usage)

	q := s.queries()

	stored, noEvidence, unstorable, writeFailed, overCap := 0, 0, 0, 0, 0
	for _, p := range resp.Suggestions {
		if stored == maxSuggestionsStore {
			overCap = len(resp.Suggestions) - stored - noEvidence - unstorable - writeFailed
			break
		}
		evidence, err := suggestionEvidence(p, refs)
		if err != nil || len(evidence) == 0 {

			quote := strings.TrimSpace(p.Evidence)
			head, _ := cut(quote, 80)
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
		"proposed", len(resp.Suggestions),
		"stored", stored,
		"dropped_no_evidence", noEvidence,
		"dropped_unstorable", unstorable,
		"dropped_write_failed", writeFailed,
		"dropped_over_cap", overCap)
}

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

func suggestionEvidence(p llmclient.ImprovementProposal, refs []EvidenceRef) ([]byte, error) {
	out := make([]EvidenceRef, 0, maxStoredEvidence)
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

				fmt.Fprintf(&b, "    evidence (%s):\n      %s\n", r.Kind, excerpt)
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
			continue
		}
		fmt.Fprintf(&b, "  [%s/%s] %s\n", f.Category, f.Severity, firstChars(f.Message, 800))
		addRefs(f.Evidence)
	}

	digest, _ := cut(b.String(), maxDigestChars)
	return digest, refs
}

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
