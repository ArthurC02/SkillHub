package eval

// The task-effect leg (EVAL-001), and the four defences of ADR-026 on the Go side.
//
//	defence 1  output structure  the response schema is fixed in llm-internal.yaml;
//	                             anything outside the value domain is read here as
//	                             `undetermined`.
//	defence 2  no capability     nothing in this file gives the judge a tool, a
//	                             write, a workspace or a run id it can act on.
//	defence 3  verifiable refs   verify() re-resolves every reference against the
//	                             platform's own data; a criterion whose references
//	                             do not resolve is downgraded and said so.
//	defence 4  content separated  the labelled data block is the Python side's job;
//	                             what this file controls is what gets in at all.
//
// The judge can invent a sentence. It cannot invent an event id the platform will
// find, and that is the part this file enforces.

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

// The input budget of evaluation-design §6.3. The cost of a judgement is almost
// entirely input, so the ceiling is set by truncation here and not by hope.
// Every cut is named in the request's `truncation` list, and a criterion that
// depends on a cut field may then only be answered `undetermined`.
const (
	maxFinalOutput  = 40000 // CONTENT-005's existing review threshold
	maxCriteria     = 20
	maxDigestEntry  = 2000
	maxDigestCount  = 100 // llm-internal.yaml TraceDigest.entries maxItems
	maxArtifactRows = 500
	excerptLimit    = 1000 // what a stored EvidenceRef keeps of its source
)

// judge assembles the request, calls services/llm, and verifies everything that
// comes back before any of it becomes a stored verdict.
func (s *Service) judge(ctx context.Context, m material, ev gen.Evaluation) (verdict, error) {
	if s.Judge == nil {
		return verdict{}, fmt.Errorf("no judge service is configured for this deployment")
	}

	req, digest, truncation, dropped := s.buildRequest(m, ev)

	// The internal call carries the caller's deadline and cancellation (iron rule
	// 7). llmclient has no timeout of its own, so this is the only one.
	callCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	resp, err := s.Judge.JudgeRun(callCtx, req)
	if err != nil {
		return verdict{}, err
	}

	results := s.merge(m, resp.Verdict, digest, len(truncation) > 0)
	v := verdict{
		overall:          overallFrom(results),
		summary:          resp.Verdict.Summary,
		results:          results,
		evidenceComplete: true,
		model:            orUnknown(resp.Model),
		promptVersion:    orUnknown(resp.PromptVersion),
	}
	// What the row records is the rubric that was actually in force, which is why
	// it is read off the request rather than off the snapshot: a rubric whose
	// items all named criteria this run does not have reached the judge as
	// nothing, and recording its version would claim a strengthening that never
	// applied. The `evaluation_started` event declares the intent (eval.go begin);
	// where the two differ, the row is the authority — same rule as judge_model.
	if req.Rubric != nil && m.rubric != nil {
		v.rubricVersion = m.rubric.Version
	}
	if resp.Usage != nil {
		v.costUSD, v.costSource = resp.Usage.CostUSD, resp.Usage.CostSource
	}
	// A rubric item addressed to a criterion that was not sent has nowhere for its
	// verdict to be stored — merge() drops any id it did not ask about, exactly as
	// /match-reasons drops an unrequested skill_id. Silently dropping it would
	// leave a rubric the user can read and the platform quietly ignores, so it is
	// said out loud rather than inferred from a missing line in the report.
	if len(dropped) > 0 {
		v.findings = append(v.findings, Finding{
			Category: CategoryEffect, Severity: SeverityWarning,
			Message: "these rubric items name no acceptance criterion of this run's snapshot and " +
				"were not sent for judgement (" + strings.Join(dropped, ", ") + "); a rubric item's " +
				"id is the criterion it strengthens",
			Evidence: []EvidenceRef{},
		})
	}
	// A judgement made on cut material is a judgement with a hole in it, and the
	// stored row has to say so rather than leave it to a reader to notice.
	if len(truncation) > 0 {
		v.evidenceComplete = false
		v.findings = append(v.findings, Finding{
			Category: CategoryEffect, Severity: SeverityWarning,
			Message: "the material sent for judgement was truncated (" +
				strings.Join(truncation, ", ") + "); criteria depending on it are undetermined rather than passed",
			Evidence: []EvidenceRef{},
		})
	}
	return v, nil
}

// buildRequest cuts the material to the §6.3 budget and returns the request, the
// digest keyed by event id (the citation set verification checks against), and the
// list of fields that were cut.
// The fourth return is the rubric items that named no criterion this request
// sends, which the caller turns into a warning.
func (s *Service) buildRequest(
	m material, ev gen.Evaluation,
) (llmclient.JudgeRunRequest, map[string]trace.EventView, []string, []string) {
	truncation := []string{}

	final, cutOutput := cut(m.summary.FinalOutput, maxFinalOutput)
	if cutOutput {
		truncation = append(truncation, "final_output")
	}

	criteria := make([]llmclient.JudgeCriterion, 0, maxCriteria)
	for _, c := range m.criteria {
		if len(criteria) == maxCriteria {
			truncation = append(truncation, "criteria")
			break
		}
		criteria = append(criteria, llmclient.JudgeCriterion{ID: c.ID, Text: c.Text})
	}

	artifacts := make([]llmclient.JudgeArtifact, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		if len(artifacts) == maxArtifactRows {
			truncation = append(truncation, "artifacts")
			break
		}
		// Manifest rows only, never text_excerpt: an attempt's output is one
		// archive at one object key (grantsFor mints a single write grant), so
		// reading a file's text would mean opening the archive in the control
		// plane. The contract allows exactly this — a row with no excerpt — and
		// the absence is a fact about the request, not about the file.
		artifacts = append(artifacts, llmclient.JudgeArtifact{
			Path: a.FileName, SizeBytes: a.SizeBytes, ContentType: a.ContentType,
		})
	}

	entries, digest, cutDigest := buildDigest(m.advanced)
	if cutDigest {
		truncation = append(truncation, "trace_digest.entries")
	}

	req := llmclient.JudgeRunRequest{
		RunID:        uuidString(m.run.ID),
		EvaluationID: uuidString(ev.ID),
		UserPrompt:   m.snapshot.UserPrompt,
		Criteria:     criteria,
		FinalOutput:  final,
		Artifacts:    artifacts,
		TraceDigest: llmclient.TraceDigest{
			Complete: m.advanced.Complete,
			Entries:  entries,
		},
		Truncation: truncation,
	}
	if m.skill.Name != "" {
		req.Skill = &llmclient.JudgeSkill{Name: m.skill.Name, Summary: derefString(m.skill.Summary)}
	}
	rubric, dropped := rubricFor(m.rubric, criteria)
	req.Rubric = rubric
	return req, digest, truncation, dropped
}

// rubricFor turns the snapshot's frozen rubric into the request's, keeping only
// the items whose id is one of the criteria actually being sent.
//
// The filter is not defensive tidying, it is the data semantics: /judge-run
// answers one verdict per criterion id, and merge() drops any id it did not ask
// about, so an item addressed to something else can never produce a stored
// verdict (writing-rubrics.md §2.1). Sending it anyway would spend input budget
// asking the model for an answer with nowhere to go — and, worse, would put a
// standard in the prompt that no line of the report is ever measured against.
//
// The rubric's own version is not sent: llm-internal.yaml's Rubric is
// additionalProperties:false and has only `items`. The version is the platform's
// record of which wording was in force, and lives on the evaluations row.
func rubricFor(r *testlab.Rubric, criteria []llmclient.JudgeCriterion) (*llmclient.Rubric, []string) {
	if r == nil || len(r.Items) == 0 {
		return nil, nil
	}
	sent := make(map[string]bool, len(criteria))
	for _, c := range criteria {
		sent[c.ID] = true
	}
	items := make([]llmclient.RubricItem, 0, len(r.Items))
	var dropped []string
	for _, it := range r.Items {
		if !sent[it.ID] {
			dropped = append(dropped, it.ID)
			continue
		}
		items = append(items, llmclient.RubricItem{
			ID: it.ID, Text: it.Text, Weight: it.Weight, EvidenceRequired: it.EvidenceRequired,
		})
	}
	if len(items) == 0 {
		return nil, dropped
	}
	return &llmclient.Rubric{Items: items}, dropped
}

// buildDigest selects the citable trace entries. Selection matters twice: it is
// the input budget, and it is the citation set — a trace_event reference may only
// name an id that appeared here.
//
// The tail is kept rather than the head. A run's interesting events (the final
// output, the errors, the usage roll-up) are at the end, and a head-first cut on a
// chatty run would hand the judge nothing but the warm-up.
func buildDigest(view trace.AdvancedView) ([]llmclient.TraceDigestEntry, map[string]trace.EventView, bool) {
	citable := make([]trace.EventView, 0, len(view.Events))
	for _, e := range view.Events {
		switch e.Type {
		case trace.TypeSkillActivation, trace.TypeResourceRead, trace.TypeToolCall,
			trace.TypeScriptLog, trace.TypeAgentOutput, trace.TypeError, trace.TypeUsage:
			citable = append(citable, e)
		}
	}
	truncated := false
	if len(citable) > maxDigestCount {
		citable = citable[len(citable)-maxDigestCount:]
		truncated = true
	}

	entries := make([]llmclient.TraceDigestEntry, 0, len(citable))
	digest := make(map[string]trace.EventView, len(citable))
	for _, e := range citable {
		excerpt, _ := cut(string(e.Payload), maxDigestEntry)
		entries = append(entries, llmclient.TraceDigestEntry{
			TraceEventID: e.EventID,
			OccurredAt:   e.OccurredAt,
			Type:         e.Type,
			Excerpt:      excerpt,
		})
		digest[e.EventID] = e
	}
	return entries, digest, truncated
}

// merge turns the model's answer into stored criterion results.
//
// Every criterion in the snapshot gets exactly one entry, whether or not the model
// answered it: a criterion the judge skipped is `undetermined` and says so, rather
// than disappearing from the report. Every entry is `source: model` — the response
// has no field through which the model could claim otherwise.
func (s *Service) merge(
	m material, v llmclient.JudgeVerdict, digest map[string]trace.EventView, truncated bool,
) []CriterionResult {
	answers := make(map[string]llmclient.CriterionVerdict, len(v.CriterionResults))
	for _, cv := range v.CriterionResults {
		// An id that was not sent is dropped, exactly as an unrequested skill_id is
		// dropped from /match-reasons: it answers a question nobody asked.
		answers[cv.CriterionID] = cv
	}

	out := make([]CriterionResult, 0, len(m.criteria))
	for _, c := range m.criteria {
		result := CriterionResult{
			CriterionID: c.ID, Text: c.Text,
			Result: ResultUndetermined, Source: SourceModel, Evidence: []EvidenceRef{},
			Reason: "the judge returned no verdict for this criterion",
		}
		cv, answered := answers[c.ID]
		if !answered {
			out = append(out, result)
			continue
		}

		result.Result = normaliseResult(cv.Result)
		result.Reason = cv.Reason

		var unverifiable []string
		for _, ref := range cv.EvidenceRefs {
			verified, why := verify(ref, m, digest)
			if why != "" {
				unverifiable = append(unverifiable, why)
				continue
			}
			result.Evidence = append(result.Evidence, verified)
		}

		// Defence 3. A reference that does not resolve is not a smaller verdict, it
		// is an unverified one, and `undetermined` is the safe default.
		if len(unverifiable) > 0 && result.Result != ResultUndetermined {
			result.Result = ResultUndetermined
			result.Reason = "evidence_unverifiable: " + strings.Join(unverifiable, "; ") +
				". The judge's own reasoning was: " + cv.Reason
		}
		// 丙-1 and the truncation rule of §6.3: evidence that may be missing cannot
		// support a pass. It can still support a failure — a criterion contradicted
		// by what *is* there does not become uncertain because something else is
		// absent.
		if result.Result == ResultPassed && (!m.advanced.Complete || truncated) {
			result.Result = ResultUndetermined
			result.Reason = "judged on incomplete evidence (the trace has gaps or the input " +
				"was truncated), so a pass cannot be recorded. The judge's own reasoning was: " + cv.Reason
		}
		out = append(out, result)
	}
	return out
}

// normaliseResult is defence 1's Go half: three values, no fourth. Anything else -
// including a value a non-conforming gateway let through - reads as undetermined.
func normaliseResult(r string) string {
	switch r {
	case ResultPassed, ResultFailed, ResultUndetermined:
		return r
	default:
		return ResultUndetermined
	}
}

// verify re-resolves one claimed reference against the platform's own data and
// returns the stored form. A non-empty second return is why it did not resolve.
//
// Ranges are computed here, never taken from the model: the request carries no
// offsets for it to return, and an offset a model computed would be one more thing
// to get wrong.
func verify(
	ref llmclient.JudgeEvidenceRef, m material, digest map[string]trace.EventView,
) (EvidenceRef, string) {
	switch ref.Kind {
	case KindTraceEvent:
		id := derefString(ref.TraceEventID)
		event, ok := digest[id]
		if !ok {
			return EvidenceRef{}, fmt.Sprintf("cited trace event %q was not in the digest", id)
		}
		if ref.Quote != "" && !strings.Contains(string(event.Payload), ref.Quote) {
			return EvidenceRef{}, fmt.Sprintf("the quote cited from trace event %q is not in it", id)
		}
		out := traceRef(event, event.Type)
		if ref.Quote != "" {
			out.Excerpt, out.ExcerptTruncated = cut(ref.Quote, excerptLimit)
		}
		return out, ""

	case KindArtifact:
		path := derefString(ref.ArtifactPath)
		for _, a := range m.artifacts {
			if a.FileName == path {
				// The path is what is checkable here, and it is the part a model
				// cannot invent. The quote is not matched: no artifact bytes were
				// sent (buildRequest ships manifest rows only), so there was
				// nothing for the model to quote and the platform's own manifest
				// line is the honest excerpt. No byte_range for the same reason.
				return artifactRef(a), ""
			}
		}
		return EvidenceRef{}, fmt.Sprintf("cited artifact %q is not in this run's manifest", path)

	case KindAgentOutput:
		if ref.Quote == "" {
			return EvidenceRef{}, "an agent output reference was cited with no quote to locate"
		}
		idx := strings.Index(m.summary.FinalOutput, ref.Quote)
		if idx < 0 {
			return EvidenceRef{}, "the quote cited from the agent's final output is not in it"
		}
		excerpt, truncated := cut(ref.Quote, excerptLimit)
		start := utf8.RuneCountInString(m.summary.FinalOutput[:idx])
		return EvidenceRef{
			Kind:             KindAgentOutput,
			CharRange:        &Range{Start: start, End: start + utf8.RuneCountInString(ref.Quote)},
			Excerpt:          excerpt,
			ExcerptTruncated: truncated,
			Available:        true,
		}, ""

	default:
		return EvidenceRef{}, fmt.Sprintf("reference kind %q is not one this platform can resolve", ref.Kind)
	}
}

// cut truncates to a character budget and says whether it had to. Runes, not
// bytes: cutting mid-rune would store a broken character in a report.
func cut(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
