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
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/checks"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

const (
	maxFinalOutput = 40000 // one-number: maxFinalOutput - CONTENT-005's existing review threshold
	maxCriteria    = 20    // one-number: maxCriteria

	maxDigestEntry  = 8000 // one-number: maxDigestEntry
	maxDigestCount  = 100  // one-number: maxDigestCount
	maxArtifactRows = 500  // one-number: maxArtifactRows
)

func (s *Service) judge(ctx context.Context, m facts.Material, ev gen.Evaluation) (assessment, error) {
	if s.Judge == nil {
		return assessment{}, fmt.Errorf("no judge service is configured for this deployment")
	}

	req, sent := s.buildRequest(m, ev)
	digest, truncation, dropped, trimmedEvents := sent.digest, sent.truncation, sent.dropped, sent.trimmedEvents
	req.Within = s.Budgets.Within(ctx, JudgeBudget)

	callCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	answers, err := s.judgeAll(callCtx, req)
	if err != nil {
		return assessment{}, err
	}
	cuts := evidenceCuts{batch: batchWideCut(truncation), trimmedEvents: trimmedEvents}
	perMember := make([][]verdict.CriterionResult, len(answers))
	for i, answer := range answers {
		perMember[i] = s.merge(m, answer, digest, cuts)
	}
	results, split := vote(perMember)
	v := assessment{
		overall:          verdict.OverallFrom(results),
		summary:          answers[0].Summary,
		results:          results,
		evidenceComplete: true,
		model:            panelModel(answers),
		promptVersion:    orUnknown(answers[0].PromptVersion),
	}

	if req.Rubric != nil && m.Rubric != nil {
		v.rubricVersion = m.Rubric.Version
	}
	v.usage = panelUsage(answers)
	v.costUSD = v.usage.ReportedCostUSD()

	if len(split) > 0 {
		v.findings = append(v.findings, verdict.NewFinding(verdict.CategoryEffect, verdict.SeverityWarning,
			"the judges did not agree on "+strings.Join(split, ", ")+
				"; those criteria are undetermined rather than decided by one judge"))
	}

	if len(dropped) > 0 {
		v.findings = append(v.findings, verdict.NewFinding(verdict.CategoryEffect, verdict.SeverityWarning,
			"these rubric items name no acceptance criterion of this run's snapshot and "+
				"were not sent for judgement ("+strings.Join(dropped, ", ")+"); a rubric item's "+
				"id is the criterion it strengthens"))
	}

	if len(truncation) > 0 {
		v.evidenceComplete = false
		v.findings = append(v.findings, verdict.NewFinding(verdict.CategoryEffect, verdict.SeverityWarning,
			"the material sent for judgement was truncated ("+
				strings.Join(truncation, ", ")+"); criteria depending on it are undetermined rather than passed"))
	}
	return v, nil
}

type sentForJudgement struct {
	digest        map[string]trace.EventView
	truncation    []string
	dropped       []string
	trimmedEvents map[string]bool
}

func (s *Service) buildRequest(m facts.Material, ev gen.Evaluation) (JudgeRequest, sentForJudgement) {
	truncation := []string{}

	final, cutOutput := cut(m.Summary.FinalOutput, maxFinalOutput)
	if cutOutput {
		truncation = append(truncation, "final_output")
	}

	criteria := make([]JudgeCriterion, 0, maxCriteria)
	for _, c := range m.Criteria {
		if len(criteria) == maxCriteria {
			truncation = append(truncation, "criteria")
			break
		}
		criteria = append(criteria, JudgeCriterion{ID: c.ID, Text: c.Text, Instructions: m.Instructions[c.ID]})
	}

	artifacts := make([]JudgeArtifact, 0, len(m.Artifacts))
	for _, a := range m.Artifacts {
		if len(artifacts) == maxArtifactRows {
			truncation = append(truncation, "artifacts")
			break
		}

		artifacts = append(artifacts, JudgeArtifact{
			Path: a.FileName, SizeBytes: a.SizeBytes, ContentType: a.ContentType,
		})
	}

	if m.Absent.Any() {
		truncation = append(truncation, "artifacts.unreadable")
	}

	entries, digest, cuts := buildDigest(m.Advanced)
	if m.Advanced.EvaluationTruncated {
		truncation = append(truncation, "trace_events")
	}

	if cuts.DroppedEvents {
		truncation = append(truncation, "trace_digest.entries")
	}
	if cuts.TrimmedExcerpts {
		truncation = append(truncation, "trace_digest.entries[].excerpt")
	}

	req := JudgeRequest{
		RunID:        pgconv.UUIDString(m.Run.ID),
		EvaluationID: pgconv.UUIDString(ev.ID),
		UserPrompt:   m.Snapshot.UserPrompt,
		Criteria:     criteria,
		FinalOutput:  final,
		Artifacts:    artifacts,
		TraceDigest: TraceDigest{
			Complete: m.Advanced.Complete,
			Entries:  entries,
		},
		Truncation: truncation,
	}
	if m.Skill.Name != "" {
		req.Skill = &JudgedSkill{Name: m.Skill.Name, Summary: derefString(m.Skill.Summary)}
	}
	rubric, dropped := rubricFor(m.Rubric, criteria)
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

func (c evidenceCuts) restsOnATrimmedSource(refs []verdict.EvidenceRef) bool {
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
	m facts.Material, v *Judgement, digest map[string]trace.EventView, cuts evidenceCuts,
) []verdict.CriterionResult {

	evidenceRequired := map[string]bool{}
	if m.Rubric != nil {
		for _, it := range m.Rubric.Items {
			evidenceRequired[it.ID] = it.EvidenceRequired
		}
	}

	answers := make(map[string]CriterionVerdict, len(v.Criteria))
	for _, cv := range v.Criteria {

		answers[cv.CriterionID] = cv
	}

	out := make([]verdict.CriterionResult, 0, len(m.Criteria))
	for _, c := range m.Criteria {
		result := checks.NoVerdict(c)
		cv, answered := answers[c.ID]
		if !answered {
			out = append(out, result)
			continue
		}

		result = withVerifiedEvidence(result, cv, m, digest)

		if evidenceRequired[c.ID] && result.Result != verdict.ResultUndetermined && !hasVerifiedQuote(result.Evidence) {
			result.Result = verdict.ResultUndetermined
			result.Reason = "evidence_unverifiable: this rubric item requires quoted evidence, and " +
				"none of the citations offered has a quote this platform could find in the run's " +
				"verifiable sources (an artifact citation proves the file exists, not what is in it). " +
				"The judge's own reasoning was: " + cv.Reason
		}

		if m.CannotRecordPass(result.Result) || (result.Result == verdict.ResultPassed && (cuts.batch || cuts.restsOnATrimmedSource(result.Evidence))) {
			result.Result = verdict.ResultUndetermined
			result.Reason = "judged on incomplete evidence (the trace has gaps or the input " +
				"was truncated), so a pass cannot be recorded. The judge's own reasoning was: " + cv.Reason
		}
		out = append(out, result)
	}
	return out
}

func withVerifiedEvidence(
	result verdict.CriterionResult, cv CriterionVerdict, m facts.Material, digest map[string]trace.EventView,
) verdict.CriterionResult {
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

	if len(unverifiable) > 0 && result.Result != verdict.ResultUndetermined {
		result.Result = verdict.ResultUndetermined
		result.Reason = "evidence_unverifiable: " + strings.Join(unverifiable, "; ") +
			". The judge's own reasoning was: " + cv.Reason
	}
	if result.Result != verdict.ResultUndetermined && len(result.Evidence) == 0 {
		result.Result = verdict.ResultUndetermined
		result.Reason = "the judge returned no verifiable evidence for this verdict. " +
			"The judge's own reasoning was: " + cv.Reason
	}
	return result
}

func hasVerifiedQuote(refs []verdict.EvidenceRef) bool {
	for _, r := range refs {
		if verdict.VerifiedQuote(r.Match) {
			return true
		}
	}
	return false
}

func normaliseResult(r string) string {
	switch r {
	case verdict.ResultPassed, verdict.ResultFailed, verdict.ResultUndetermined:
		return r
	default:
		return verdict.ResultUndetermined
	}
}

type citationCheck struct {
	ref     verdict.EvidenceRef
	why     string
	settled bool
}

func settledCitation(ref verdict.EvidenceRef, why string) citationCheck {
	return citationCheck{ref: ref, why: why, settled: true}
}

func unsettledCitation(namedFailure string) citationCheck {
	return citationCheck{why: namedFailure}
}

func verify(
	ref Citation, m facts.Material, digest map[string]trace.EventView,
) (verdict.EvidenceRef, string) {
	check := checkWhereCited(ref, m, digest)
	if check.settled {
		return check.ref, check.why
	}

	if ref.Quote != "" {
		if out, ok := reattributedQuote(ref, m, digest); ok {
			return out, ""
		}
	}

	if ref.Kind == verdict.KindArtifact {
		return artifactInManifest(ref, m)
	}
	return verdict.EvidenceRef{}, check.why + ", and it is in no other verifiable source of this run"
}

func checkWhereCited(ref Citation, m facts.Material, digest map[string]trace.EventView) citationCheck {
	switch ref.Kind {
	case verdict.KindTraceEvent:
		return checkTraceEventCitation(ref, digest)

	case verdict.KindAgentOutput:
		if ref.Quote == "" {
			return settledCitation(verdict.EvidenceRef{}, "an agent output reference was cited with no quote to locate")
		}
		if match, idx, ok := locate(m.Summary.FinalOutput, ref.Quote); ok {
			return settledCitation(outputRef(m.Summary.FinalOutput, ref.Quote, idx, match), "")
		}
		return unsettledCitation("the quote cited from the agent's final output is not in it")

	case verdict.KindArtifact:

		return unsettledCitation("an artifact citation's quote is verified against nothing")

	default:
		return settledCitation(verdict.EvidenceRef{}, fmt.Sprintf("reference kind %q is not one this platform can resolve", ref.Kind))
	}
}

func checkTraceEventCitation(ref Citation, digest map[string]trace.EventView) citationCheck {
	id := derefString(ref.TraceEventID)
	event, inDigest := digest[id]
	switch {
	case !inDigest:
		if ref.Quote == "" {

			return settledCitation(verdict.EvidenceRef{}, fmt.Sprintf("cited trace event %q was not in the digest", id))
		}
		return unsettledCitation(fmt.Sprintf("cited trace event %q was not in the digest", id))
	case ref.Quote == "":
		out := checks.TraceRef(event, event.Type)
		out.Match = verdict.MatchNotChecked
		return settledCitation(out, "")
	default:
		if match, _, ok := locate(traceSearchText(event.Payload), ref.Quote); ok {
			return settledCitation(traceQuoteRef(event, ref.Quote, match), "")
		}
		return unsettledCitation(fmt.Sprintf("the quote cited from trace event %q is not in it", id))
	}
}

func reattributedQuote(ref Citation, m facts.Material, digest map[string]trace.EventView) (verdict.EvidenceRef, bool) {
	found, ok := findQuote(ref.Quote, verifiableSources(m, digest))
	if !ok {
		return verdict.EvidenceRef{}, false
	}
	var out verdict.EvidenceRef
	if found.src.kind == verdict.KindTraceEvent {
		out = traceQuoteRef(found.src.event, ref.Quote, found.match)
	} else {
		out = outputRef(found.src.text, ref.Quote, found.idx, found.match)
	}
	out.ReattributedFrom = ref.Kind
	return out, true
}

func artifactInManifest(ref Citation, m facts.Material) (verdict.EvidenceRef, string) {
	path := derefString(ref.ArtifactPath)
	for _, a := range m.Artifacts {
		if a.FileName == path {

			out := checks.ArtifactRef(a)
			out.Match = verdict.MatchNotChecked
			return out, ""
		}
	}
	return verdict.EvidenceRef{}, fmt.Sprintf("cited artifact %q is not in this run's manifest", path)
}

type source struct {
	kind  string
	text  string
	event trace.EventView
}

func verifiableSources(m facts.Material, digest map[string]trace.EventView) []source {
	out := make([]source, 0, len(digest)+1)
	if m.Summary.FinalOutput != "" {
		out = append(out, source{kind: verdict.KindAgentOutput, text: m.Summary.FinalOutput})
	}
	for _, e := range m.Advanced.Events {
		if de, ok := digest[e.EventID]; ok {
			out = append(out, source{kind: verdict.KindTraceEvent, text: traceSearchText(de.Payload), event: de})
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
			return foundQuote{src: s, idx: i, match: verdict.MatchExact}, true
		}
	}
	nq := normalizeQuote(quote)
	if utf8.RuneCountInString(nq) < minNormalizedQuote {
		return foundQuote{idx: -1}, false
	}
	for _, s := range sources {
		if strings.Contains(normalizeQuote(s.text), nq) {
			return foundQuote{src: s, idx: -1, match: verdict.MatchNormalized}, true
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
		return verdict.MatchExact, i, true
	}
	nq := normalizeQuote(quote)
	if utf8.RuneCountInString(nq) < minNormalizedQuote {
		return "", -1, false
	}
	if strings.Contains(normalizeQuote(text), nq) {
		return verdict.MatchNormalized, -1, true
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

func traceQuoteRef(e trace.EventView, quote, match string) verdict.EvidenceRef {
	out := checks.TraceRef(e, e.Type)
	out.Excerpt, out.ExcerptTruncated = verdict.Excerpt(verifiedText(quote, match))
	out.Match = match
	return out
}

func outputRef(final, quote string, idx int, match string) verdict.EvidenceRef {
	excerpt, truncated := verdict.Excerpt(verifiedText(quote, match))
	out := verdict.EvidenceRef{
		Kind:             verdict.KindAgentOutput,
		Match:            match,
		Excerpt:          excerpt,
		ExcerptTruncated: truncated,
		Available:        true,
	}
	if idx >= 0 {
		start := utf8.RuneCountInString(final[:idx])
		out.CharRange = &verdict.Range{Start: start, End: start + utf8.RuneCountInString(quote)}
	}
	return out
}

func verifiedText(quote, match string) string {
	if match == verdict.MatchNormalized {
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
