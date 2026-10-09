package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

const (
	maxFinalOutput = 40000 // one-number: maxFinalOutput - CONTENT-005's existing review threshold
	maxCriteria    = 20    // one-number: maxCriteria

	maxDigestEntry  = 8000 // one-number: maxDigestEntry
	maxDigestCount  = 100  // one-number: maxDigestCount
	maxArtifactRows = 500  // one-number: maxArtifactRows
	excerptLimit    = 1000 // one-number: excerptLimit - what a stored EvidenceRef keeps of its source
)

func (s *Service) judge(ctx context.Context, m material, ev gen.Evaluation) (verdict, error) {
	if s.Judge == nil {
		return verdict{}, fmt.Errorf("no judge service is configured for this deployment")
	}

	req, sent := s.buildRequest(m, ev)
	digest, truncation, dropped, trimmedEvents := sent.digest, sent.truncation, sent.dropped, sent.trimmedEvents
	req.Within = s.Budgets.Within(ctx, JudgeBudget)

	callCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	answers, err := s.judgeAll(callCtx, req)
	if err != nil {
		return verdict{}, err
	}
	cuts := evidenceCuts{batch: batchWideCut(truncation), trimmedEvents: trimmedEvents}
	perMember := make([][]CriterionResult, len(answers))
	for i, answer := range answers {
		perMember[i] = s.merge(m, answer, digest, cuts)
	}
	results, split := vote(perMember)
	v := verdict{
		overall:          overallFrom(results),
		summary:          answers[0].Summary,
		results:          results,
		evidenceComplete: true,
		model:            panelModel(answers),
		promptVersion:    orUnknown(answers[0].PromptVersion),
	}

	if req.Rubric != nil && m.rubric != nil {
		v.rubricVersion = m.rubric.Version
	}
	v.usage = panelUsage(answers)
	v.costUSD = v.usage.ReportedCostUSD()

	if len(split) > 0 {
		v.findings = append(v.findings, Finding{
			Category: CategoryEffect, Severity: SeverityWarning,
			Message: "the judges did not agree on " + strings.Join(split, ", ") +
				"; those criteria are undetermined rather than decided by one judge",
			Evidence: []EvidenceRef{},
		})
	}

	if len(dropped) > 0 {
		v.findings = append(v.findings, Finding{
			Category: CategoryEffect, Severity: SeverityWarning,
			Message: "these rubric items name no acceptance criterion of this run's snapshot and " +
				"were not sent for judgement (" + strings.Join(dropped, ", ") + "); a rubric item's " +
				"id is the criterion it strengthens",
			Evidence: []EvidenceRef{},
		})
	}

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

type sentForJudgement struct {
	digest        map[string]trace.EventView
	truncation    []string
	dropped       []string
	trimmedEvents map[string]bool
}

func (s *Service) buildRequest(m material, ev gen.Evaluation) (JudgeRequest, sentForJudgement) {
	truncation := []string{}

	final, cutOutput := cut(m.summary.FinalOutput, maxFinalOutput)
	if cutOutput {
		truncation = append(truncation, "final_output")
	}

	criteria := make([]JudgeCriterion, 0, maxCriteria)
	for _, c := range m.criteria {
		if len(criteria) == maxCriteria {
			truncation = append(truncation, "criteria")
			break
		}
		criteria = append(criteria, JudgeCriterion{ID: c.ID, Text: c.Text})
	}

	artifacts := make([]JudgeArtifact, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		if len(artifacts) == maxArtifactRows {
			truncation = append(truncation, "artifacts")
			break
		}

		artifacts = append(artifacts, JudgeArtifact{
			Path: a.FileName, SizeBytes: a.SizeBytes, ContentType: a.ContentType,
		})
	}

	if m.absent.Any() {
		truncation = append(truncation, "artifacts.unreadable")
	}

	entries, digest, cuts := buildDigest(m.advanced)
	if m.advanced.EvaluationTruncated {
		truncation = append(truncation, "trace_events")
	}

	if cuts.DroppedEvents {
		truncation = append(truncation, "trace_digest.entries")
	}
	if cuts.TrimmedExcerpts {
		truncation = append(truncation, "trace_digest.entries[].excerpt")
	}

	req := JudgeRequest{
		RunID:        pgconv.UUIDString(m.run.ID),
		EvaluationID: pgconv.UUIDString(ev.ID),
		UserPrompt:   m.snapshot.UserPrompt,
		Criteria:     criteria,
		FinalOutput:  final,
		Artifacts:    artifacts,
		TraceDigest: TraceDigest{
			Complete: m.advanced.Complete,
			Entries:  entries,
		},
		Truncation: truncation,
	}
	if m.skill.Name != "" {
		req.Skill = &JudgedSkill{Name: m.skill.Name, Summary: derefString(m.skill.Summary)}
	}
	rubric, dropped := rubricFor(m.rubric, criteria)
	req.Rubric = rubric
	return req, sentForJudgement{digest: digest, truncation: truncation, dropped: dropped, trimmedEvents: cuts.TrimmedEvents}
}

func rubricFor(r *testlab.Rubric, criteria []JudgeCriterion) (*JudgeRubric, []string) {
	if r == nil || len(r.Items) == 0 {
		return nil, nil
	}
	sent := make(map[string]bool, len(criteria))
	for _, c := range criteria {
		sent[c.ID] = true
	}
	items := make([]JudgeRubricItem, 0, len(r.Items))
	var dropped []string
	for _, it := range r.Items {
		if !sent[it.ID] {
			dropped = append(dropped, it.ID)
			continue
		}
		items = append(items, JudgeRubricItem{
			ID: it.ID, Text: it.Text, Weight: it.Weight, EvidenceRequired: it.EvidenceRequired,
		})
	}
	if len(items) == 0 {
		return nil, dropped
	}
	return &JudgeRubric{Items: items}, dropped
}

type digestCuts struct {
	DroppedEvents bool

	TrimmedExcerpts bool

	TrimmedEvents map[string]bool
}

func buildDigest(view trace.AdvancedView) ([]TraceDigestEntry, map[string]trace.EventView, digestCuts) {
	citable := make([]trace.EventView, 0, len(view.Events))
	for _, e := range view.Events {
		switch e.Type {
		case trace.TypeSkillActivation, trace.TypeResourceRead, trace.TypeToolCall,
			trace.TypeScriptLog, trace.TypeAgentOutput, trace.TypeError, trace.TypeUsage:
			citable = append(citable, e)
		}
	}
	var cuts digestCuts
	if len(citable) > maxDigestCount {
		citable = citable[len(citable)-maxDigestCount:]
		cuts.DroppedEvents = true
	}

	entries := make([]TraceDigestEntry, 0, len(citable))
	digest := make(map[string]trace.EventView, len(citable))
	for _, e := range citable {
		excerpt, cutExcerpt := cut(string(e.Payload), maxDigestEntry)
		entries = append(entries, TraceDigestEntry{
			TraceEventID: e.EventID,
			OccurredAt:   e.OccurredAt,
			Type:         e.Type,
			Excerpt:      excerpt,
		})
		if cutExcerpt {
			cuts.TrimmedExcerpts = true
			if cuts.TrimmedEvents == nil {
				cuts.TrimmedEvents = map[string]bool{}
			}
			cuts.TrimmedEvents[e.EventID] = true
		}
		digest[e.EventID] = e
	}
	return entries, digest, cuts
}

type evidenceCuts struct {
	batch bool

	trimmedEvents map[string]bool
}

func (c evidenceCuts) restsOnATrimmedSource(refs []EvidenceRef) bool {
	for _, r := range refs {
		if r.TraceEventID != "" && c.trimmedEvents[r.TraceEventID] {
			return true
		}
	}
	return false
}

func batchWideCut(truncation []string) bool {
	for _, name := range truncation {
		if name != "trace_digest.entries[].excerpt" {
			return true
		}
	}
	return false
}

func (s *Service) merge(
	m material, v *Judgement, digest map[string]trace.EventView, cuts evidenceCuts,
) []CriterionResult {

	evidenceRequired := map[string]bool{}
	if m.rubric != nil {
		for _, it := range m.rubric.Items {
			evidenceRequired[it.ID] = it.EvidenceRequired
		}
	}

	answers := make(map[string]CriterionVerdict, len(v.Criteria))
	for _, cv := range v.Criteria {

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

		result = withVerifiedEvidence(result, cv, m, digest)

		if evidenceRequired[c.ID] && result.Result != ResultUndetermined && !hasVerifiedQuote(result.Evidence) {
			result.Result = ResultUndetermined
			result.Reason = "evidence_unverifiable: this rubric item requires quoted evidence, and " +
				"none of the citations offered has a quote this platform could find in the run's " +
				"verifiable sources (an artifact citation proves the file exists, not what is in it). " +
				"The judge's own reasoning was: " + cv.Reason
		}

		if result.Result == ResultPassed && (!m.advanced.Complete || cuts.batch || cuts.restsOnATrimmedSource(result.Evidence)) {
			result.Result = ResultUndetermined
			result.Reason = "judged on incomplete evidence (the trace has gaps or the input " +
				"was truncated), so a pass cannot be recorded. The judge's own reasoning was: " + cv.Reason
		}
		out = append(out, result)
	}
	return out
}

func withVerifiedEvidence(
	result CriterionResult, cv CriterionVerdict, m material, digest map[string]trace.EventView,
) CriterionResult {
	result.Result = normaliseResult(cv.Result)
	result.Reason = cv.Reason

	var unverifiable []string
	for _, ref := range cv.Citations {
		verified, why := verify(ref, m, digest)
		if why != "" {
			unverifiable = append(unverifiable, why)
			continue
		}
		result.Evidence = append(result.Evidence, verified)
	}

	if len(unverifiable) > 0 && result.Result != ResultUndetermined {
		result.Result = ResultUndetermined
		result.Reason = "evidence_unverifiable: " + strings.Join(unverifiable, "; ") +
			". The judge's own reasoning was: " + cv.Reason
	}
	if result.Result != ResultUndetermined && len(result.Evidence) == 0 {
		result.Result = ResultUndetermined
		result.Reason = "the judge returned no verifiable evidence for this verdict. " +
			"The judge's own reasoning was: " + cv.Reason
	}
	return result
}

func hasVerifiedQuote(refs []EvidenceRef) bool {
	for _, r := range refs {
		if verifiedQuote(r.Match) {
			return true
		}
	}
	return false
}

func normaliseResult(r string) string {
	switch r {
	case ResultPassed, ResultFailed, ResultUndetermined:
		return r
	default:
		return ResultUndetermined
	}
}

type citationCheck struct {
	ref     EvidenceRef
	why     string
	settled bool
}

func settledCitation(ref EvidenceRef, why string) citationCheck {
	return citationCheck{ref: ref, why: why, settled: true}
}

func unsettledCitation(namedFailure string) citationCheck {
	return citationCheck{why: namedFailure}
}

func verify(
	ref Citation, m material, digest map[string]trace.EventView,
) (EvidenceRef, string) {
	check := checkWhereCited(ref, m, digest)
	if check.settled {
		return check.ref, check.why
	}

	if ref.Quote != "" {
		if out, ok := reattributedQuote(ref, m, digest); ok {
			return out, ""
		}
	}

	if ref.Kind == KindArtifact {
		return artifactInManifest(ref, m)
	}
	return EvidenceRef{}, check.why + ", and it is in no other verifiable source of this run"
}

func checkWhereCited(ref Citation, m material, digest map[string]trace.EventView) citationCheck {
	switch ref.Kind {
	case KindTraceEvent:
		return checkTraceEventCitation(ref, digest)

	case KindAgentOutput:
		if ref.Quote == "" {
			return settledCitation(EvidenceRef{}, "an agent output reference was cited with no quote to locate")
		}
		if match, idx, ok := locate(m.summary.FinalOutput, ref.Quote); ok {
			return settledCitation(outputRef(m.summary.FinalOutput, ref.Quote, idx, match), "")
		}
		return unsettledCitation("the quote cited from the agent's final output is not in it")

	case KindArtifact:

		return unsettledCitation("an artifact citation's quote is verified against nothing")

	default:
		return settledCitation(EvidenceRef{}, fmt.Sprintf("reference kind %q is not one this platform can resolve", ref.Kind))
	}
}

func checkTraceEventCitation(ref Citation, digest map[string]trace.EventView) citationCheck {
	id := derefString(ref.TraceEventID)
	event, inDigest := digest[id]
	switch {
	case !inDigest:
		if ref.Quote == "" {

			return settledCitation(EvidenceRef{}, fmt.Sprintf("cited trace event %q was not in the digest", id))
		}
		return unsettledCitation(fmt.Sprintf("cited trace event %q was not in the digest", id))
	case ref.Quote == "":
		out := traceRef(event, event.Type)
		out.Match = MatchNotChecked
		return settledCitation(out, "")
	default:
		if match, _, ok := locate(traceSearchText(event.Payload), ref.Quote); ok {
			return settledCitation(traceQuoteRef(event, ref.Quote, match), "")
		}
		return unsettledCitation(fmt.Sprintf("the quote cited from trace event %q is not in it", id))
	}
}

func reattributedQuote(ref Citation, m material, digest map[string]trace.EventView) (EvidenceRef, bool) {
	found, ok := findQuote(ref.Quote, verifiableSources(m, digest))
	if !ok {
		return EvidenceRef{}, false
	}
	var out EvidenceRef
	if found.src.kind == KindTraceEvent {
		out = traceQuoteRef(found.src.event, ref.Quote, found.match)
	} else {
		out = outputRef(found.src.text, ref.Quote, found.idx, found.match)
	}
	out.ReattributedFrom = ref.Kind
	return out, true
}

func artifactInManifest(ref Citation, m material) (EvidenceRef, string) {
	path := derefString(ref.ArtifactPath)
	for _, a := range m.artifacts {
		if a.FileName == path {

			out := artifactRef(a)
			out.Match = MatchNotChecked
			return out, ""
		}
	}
	return EvidenceRef{}, fmt.Sprintf("cited artifact %q is not in this run's manifest", path)
}

type source struct {
	kind  string
	text  string
	event trace.EventView
}

func verifiableSources(m material, digest map[string]trace.EventView) []source {
	out := make([]source, 0, len(digest)+1)
	if m.summary.FinalOutput != "" {
		out = append(out, source{kind: KindAgentOutput, text: m.summary.FinalOutput})
	}
	for _, e := range m.advanced.Events {
		if de, ok := digest[e.EventID]; ok {
			out = append(out, source{kind: KindTraceEvent, text: traceSearchText(de.Payload), event: de})
		}
	}
	return out
}

type foundQuote struct {
	src   source
	idx   int
	match string
}

func findQuote(quote string, sources []source) (foundQuote, bool) {
	for _, s := range sources {
		if i := strings.Index(s.text, quote); i >= 0 {
			return foundQuote{src: s, idx: i, match: MatchExact}, true
		}
	}
	nq := normalizeQuote(quote)
	if utf8.RuneCountInString(nq) < minNormalizedQuote {
		return foundQuote{idx: -1}, false
	}
	for _, s := range sources {
		if strings.Contains(normalizeQuote(s.text), nq) {
			return foundQuote{src: s, idx: -1, match: MatchNormalized}, true
		}
	}
	return foundQuote{idx: -1}, false
}

// traceSearchText appends every decoded string leaf of the payload, joined by
// NUL (a byte no JSON string can contain), so a quote cannot match across
// two unrelated fields.
func traceSearchText(payload json.RawMessage) string {
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return string(payload)
	}
	var leaves []string
	var walk func(any)
	walk = func(node any) {
		switch t := node.(type) {
		case string:
			leaves = append(leaves, t)
		case []any:
			for _, item := range t {
				walk(item)
			}
		case map[string]any:

			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(v)
	if len(leaves) == 0 {
		return string(payload)
	}
	return string(payload) + "\x00" + strings.Join(leaves, "\x00")
}

func locate(text, quote string) (string, int, bool) {
	if i := strings.Index(text, quote); i >= 0 {
		return MatchExact, i, true
	}
	nq := normalizeQuote(quote)
	if utf8.RuneCountInString(nq) < minNormalizedQuote {
		return "", -1, false
	}
	if strings.Contains(normalizeQuote(text), nq) {
		return MatchNormalized, -1, true
	}
	return "", -1, false
}

const minNormalizedQuote = 12

// normalizeQuote applies Unicode NFC, collapses runs of whitespace to a single
// space, and trims structural punctuation from both ends only.
func normalizeQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	for _, r := range norm.NFC.String(s) {
		if unicode.IsSpace(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteRune(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), structuralPunctuation)
}

const structuralPunctuation = "}]),;\"'`「」『』 "

func traceQuoteRef(e trace.EventView, quote, match string) EvidenceRef {
	out := traceRef(e, e.Type)
	out.Excerpt, out.ExcerptTruncated = cut(verifiedText(quote, match), excerptLimit)
	out.Match = match
	return out
}

func outputRef(final, quote string, idx int, match string) EvidenceRef {
	excerpt, truncated := cut(verifiedText(quote, match), excerptLimit)
	out := EvidenceRef{
		Kind:             KindAgentOutput,
		Match:            match,
		Excerpt:          excerpt,
		ExcerptTruncated: truncated,
		Available:        true,
	}
	if idx >= 0 {
		start := utf8.RuneCountInString(final[:idx])
		out.CharRange = &Range{Start: start, End: start + utf8.RuneCountInString(quote)}
	}
	return out
}

func verifiedText(quote, match string) string {
	if match == MatchNormalized {
		return normalizeQuote(quote)
	}
	return quote
}

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
