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
//	                             do not resolve is downgraded and said so. ADR-043
//	                             rewrote its criterion (not the defence): a citation
//	                             holds because its quote is findable in a verifiable
//	                             source, not because of the `kind` it arrived under.
//	                             That closed the one path — `artifact` — that used
//	                             to walk around this defence entirely.
//	defence 4  content separated  the labelled data block is the Python side's job;
//	                             what this file controls is what gets in at all.
//
// The judge can invent a sentence. It cannot invent an event id the platform will
// find, and that is the part this file enforces.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	// The NFC half of ADR-043 §4's normalisation. Unicode normalisation is a table
	// this repo is not going to carry a hand-rolled copy of, and x/text is already
	// in the module graph.
	"golang.org/x/text/unicode/norm"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// The input budget of evaluation-design §6.3. The cost of a judgement is almost
// entirely input, so the ceiling is set by truncation here and not by hope.
// Every cut is named in the request's `truncation` list, and a criterion that
// depends on a cut field may then only be answered `undetermined`.
const (
	maxFinalOutput = 40000 // one-number: maxFinalOutput - CONTENT-005's existing review threshold
	maxCriteria    = 20    // one-number: maxCriteria
	// 2000 until 2026-08-23, when 04 丙-47 measured what that cost: over 164 runs
	// with a trace, 95 of them - 58% - had at least one citable payload trimmed,
	// and every one of those runs then reported incomplete evidence and had its
	// dependent criteria answered `undetermined`. What gets trimmed is not filler:
	// the largest payload seen was 24624 characters of which 24322 were a Write
	// call's `arguments`, i.e. the whole document the run produced, which is
	// exactly what a content rubric is about.
	//
	// 8000 takes the affected runs to 19.5% for 43% more digest characters -
	// well under a cent on a judgement that costs $0.0136. It is not larger
	// because this constant's job is the worst case, not the median: the ceiling
	// is maxDigestCount * maxDigestEntry, so 8000 buys a bound of 800k characters
	// (~200k tokens) per request where 2000 bought 200k. Lifting it far enough to
	// cut nothing at all (25000, measured) would put that ceiling at 2.5M
	// characters, and a budget that cannot be exceeded is the point of having one.
	//
	// The measurement's own limit, because it decides how far this number can be
	// trusted: all 164 runs come from one workspace's synthetic corpus of writing
	// Skills. Something that emits a large CSV is not in the sample.
	maxDigestEntry  = 8000 // one-number: maxDigestEntry
	maxDigestCount  = 100  // one-number: maxDigestCount
	maxArtifactRows = 500  // one-number: maxArtifactRows
	excerptLimit    = 1000 // one-number: excerptLimit - what a stored EvidenceRef keeps of its source
)

// judge assembles the request, calls apps/llm, and verifies everything that
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
	if resp.Usage != nil && resp.Usage.CostUSD != nil && resp.Usage.CostSource != "gateway" {
		// The internal contract has no estimated source. An unrecognised label is
		// unreported accounting, not permission to relabel it as an estimate.
		resp.Usage.CostUSD = nil
		resp.Usage.CostSource = ""
	}
	if err := s.recordModelUsage(ctx, ev.ID, ev.WorkspaceID, "judge",
		resp.Model, resp.PromptVersion, resp.Usage); err != nil {
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

	// A manifest row this evaluation cannot read is a hole in what the judge was
	// handed, which is what `truncation` means on this wire: a criterion that
	// depends on a cut field may only be answered `undetermined`. It is named
	// apart from the budget cut above because the two are not the same hole, and
	// because an empty `artifacts` with nothing said about it is precisely what
	// lets a judge answer every criterion believing the run produced nothing
	// (02:EVAL-001 2026-08-23, 04 丙-13). Saying it here also attaches the two
	// consequences the platform already gives a hole: the stored row's
	// evidence_complete goes false, and merge() refuses to record a `passed`.
	if m.absent.Any() {
		truncation = append(truncation, "artifacts.unreadable")
	}

	entries, digest, cuts := buildDigest(m.advanced)
	if m.advanced.EvaluationTruncated {
		truncation = append(truncation, "trace_events")
	}
	// Two names, because they are two different holes and the finding message is
	// built by joining this list. `entries` means events are missing; the second
	// means every event is here and one of them stops mid-payload. Both still set
	// evidenceComplete false - whether a trimmed tail deserves the same
	// conservatism as a missing event is a judgement about 丙-1's rule and is not
	// decided by renaming it.
	if cuts.DroppedEvents {
		truncation = append(truncation, "trace_digest.entries")
	}
	if cuts.TrimmedExcerpts {
		truncation = append(truncation, "trace_digest.entries[].excerpt")
	}

	req := llmclient.JudgeRunRequest{
		RunID:        pgconv.UUIDString(m.run.ID),
		EvaluationID: pgconv.UUIDString(ev.ID),
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
// digestCuts is what buildDigest had to leave out, kept apart because the two
// are not the same size of hole.
//
// They used to be one bool, and 04 丙-47 measured what that cost: over 164 runs
// the entry cap has never once been reached (the busiest run has 55 citable
// events against a cap of 100), while 95 of them - 58% - had at least one
// payload trimmed. So every truncation this platform has ever reported was the
// second kind while the label named the first, and a reader chasing
// `trace_digest.entries` went looking for events that were never dropped.
type digestCuts struct {
	// DroppedEvents: whole events never reached the judge (maxDigestCount).
	DroppedEvents bool
	// TrimmedExcerpts: an event is present but its payload lost its tail
	// (maxDigestEntry). Mostly `tool_call.arguments` - a Write call carrying the
	// document the run produced, which is exactly what a content rubric is about.
	TrimmedExcerpts bool
}

func buildDigest(view trace.AdvancedView) ([]llmclient.TraceDigestEntry, map[string]trace.EventView, digestCuts) {
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

	entries := make([]llmclient.TraceDigestEntry, 0, len(citable))
	digest := make(map[string]trace.EventView, len(citable))
	for _, e := range citable {
		excerpt, cutExcerpt := cut(string(e.Payload), maxDigestEntry)
		entries = append(entries, llmclient.TraceDigestEntry{
			TraceEventID: e.EventID,
			OccurredAt:   e.OccurredAt,
			Type:         e.Type,
			Excerpt:      excerpt,
		})
		if cutExcerpt {
			cuts.TrimmedExcerpts = true
		}
		digest[e.EventID] = e
	}
	return entries, digest, cuts
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
	// Which criteria the frozen rubric demands quoted evidence for. Read off the
	// snapshot's rubric, like everything else about what this run was measured
	// against (iron rule 4), and keyed by criterion id because a rubric item's id
	// *is* the criterion it strengthens (writing-rubrics.md §2.1).
	evidenceRequired := map[string]bool{}
	if m.rubric != nil {
		for _, it := range m.rubric.Items {
			evidenceRequired[it.ID] = it.EvidenceRequired
		}
	}

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
		if result.Result != ResultUndetermined && len(result.Evidence) == 0 {
			result.Result = ResultUndetermined
			result.Reason = "the judge returned no verifiable evidence for this verdict. " +
				"The judge's own reasoning was: " + cv.Reason
		}
		// ADR-043 §3. A rubric item with `evidence_required` is asking for a quote
		// that was checked against something. An `artifact` citation is a real
		// citation — the path is on the manifest and a model cannot invent that —
		// but all it can support is "this file exists, this big": the bytes are
		// never opened here, so no quote of it is ever verified, and verify()
		// stamps it `not_found` to say so. That is the G7 hole: 6 of the A round's
		// 9 `passed` rested on citations of exactly this shape, and nothing had
		// compared their quotes to anything.
		//
		// It is deliberately not the general rule. Outside `evidence_required` an
		// artifact citation still supports a verdict, because "the file is there"
		// is a fact the platform checked; what ADR-043 forbids is letting it stand
		// in for a quote somebody asked to see. The prefix is the one 丙-10's UI
		// reads to say "證據無法回驗" rather than "the judge was unsure" — this is
		// that same downgrade, so it wears the same label.
		if evidenceRequired[c.ID] && result.Result != ResultUndetermined && !hasVerifiedQuote(result.Evidence) {
			result.Result = ResultUndetermined
			result.Reason = "evidence_unverifiable: this rubric item requires quoted evidence, and " +
				"none of the citations offered has a quote this platform could find in the run's " +
				"verifiable sources (an artifact citation proves the file exists, not what is in it). " +
				"The judge's own reasoning was: " + cv.Reason
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

// hasVerifiedQuote answers whether any of these citations rests on a quote the
// platform found somewhere, whichever source it ended up filed under.
func hasVerifiedQuote(refs []EvidenceRef) bool {
	for _, r := range refs {
		if verifiedQuote(r.Match) {
			return true
		}
	}
	return false
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
// ADR-043 is the criterion this implements, in one sentence: **a citation holds
// if and only if its quote is findable in a verifiable source of this run.**
// `ref.Kind` is a hint from the model, not a credential. It is still tried first,
// because it is the only path that can carry a precise address (the trace event
// id, the char range), but when the named source cannot produce the quote the
// platform looks in the other verifiable sources before concluding anything.
//
// That one rule closes both halves of 04 乙-13, which is why it had to be one
// rule: G7 (an `artifact` citation waved through with its quote never compared to
// anything, so a fabricated sentence could satisfy `evidence_required`) and G8 (a
// verbatim-correct quote with a trailing `}],` lost to a bare substring search,
// taking two correct `failed` verdicts down with it). Splitting them would have
// meant two different rulers.
//
// Ranges are computed here, never taken from the model: the request carries no
// offsets for it to return, and an offset a model computed would be one more thing
// to get wrong.
func verify(
	ref llmclient.JudgeEvidenceRef, m material, digest map[string]trace.EventView,
) (EvidenceRef, string) {
	// Why the named source could not produce the quote, per kind. Held so that a
	// citation which then fails everywhere is refused with the specific reason.
	var namedFailure string

	switch ref.Kind {
	case KindTraceEvent:
		id := derefString(ref.TraceEventID)
		event, inDigest := digest[id]
		switch {
		case !inDigest:
			if ref.Quote == "" {
				// An id the digest does not carry, with no quote to look for, is a
				// citation of nothing: there is no content to re-attribute.
				return EvidenceRef{}, fmt.Sprintf("cited trace event %q was not in the digest", id)
			}
			namedFailure = fmt.Sprintf("cited trace event %q was not in the digest", id)
		case ref.Quote == "":
			// The citation is the event itself. The excerpt is the platform's own
			// payload, so there is nothing in it a model could have invented.
			out := traceRef(event, event.Type)
			out.Match = MatchExact
			return out, ""
		default:
			if match, _, ok := locate(traceSearchText(event.Payload), ref.Quote); ok {
				return traceQuoteRef(event, ref.Quote, match), ""
			}
			namedFailure = fmt.Sprintf("the quote cited from trace event %q is not in it", id)
		}

	case KindAgentOutput:
		if ref.Quote == "" {
			return EvidenceRef{}, "an agent output reference was cited with no quote to locate"
		}
		if match, idx, ok := locate(m.summary.FinalOutput, ref.Quote); ok {
			return outputRef(m.summary.FinalOutput, ref.Quote, idx, match), ""
		}
		namedFailure = "the quote cited from the agent's final output is not in it"

	case KindArtifact:
		// Nothing to try on the named source: no artifact bytes were sent
		// (buildRequest ships manifest rows only), and reading them would mean
		// unarchiving inside the control plane, which is the line
		// evaluation-design §2.2 draws. So an artifact citation goes straight to
		// the content search, and failing that proves existence and no more.
		namedFailure = "an artifact citation's quote is verified against nothing"

	default:
		return EvidenceRef{}, fmt.Sprintf("reference kind %q is not one this platform can resolve", ref.Kind)
	}

	// The named source did not produce the quote. Before calling it fabricated,
	// look where the quote could actually be (ADR-043 §2). This is the step that
	// tells mis-filed from invented, and the A round is why it is worth taking:
	// every quote sampled from the 6 `passed` verdicts resting on `artifact`
	// citations was present verbatim in that run's trace_events. The model had
	// read the trace and written the wrong label on it.
	if ref.Quote != "" {
		if src, idx, match, ok := findQuote(ref.Quote, verifiableSources(m, digest)); ok {
			var out EvidenceRef
			if src.kind == KindTraceEvent {
				out = traceQuoteRef(src.event, ref.Quote, match)
			} else {
				out = outputRef(src.text, ref.Quote, idx, match)
			}
			out.ReattributedFrom = ref.Kind
			return out, ""
		}
	}

	if ref.Kind == KindArtifact {
		path := derefString(ref.ArtifactPath)
		for _, a := range m.artifacts {
			if a.FileName == path {
				// The path is what is checkable here, and it is the part a model
				// cannot invent — so the citation is kept rather than thrown away
				// (ADR-043 §3). What it supports is "this file exists, this big",
				// and `not_checked` is how it says its quote, if it had one,
				// was verified against nothing — not that we looked and it was
				// absent, which is a different and harsher claim. merge() is where that stops being
				// enough for a rubric item that demands evidence.
				out := artifactRef(a)
				out.Match = MatchNotChecked
				return out, ""
			}
		}
		return EvidenceRef{}, fmt.Sprintf("cited artifact %q is not in this run's manifest", path)
	}
	return EvidenceRef{}, namedFailure + ", and it is in no other verifiable source of this run"
}

// source is one place a quote may honestly be found. There are two of them, and
// artifact bytes are deliberately not a third — see the artifact branch of
// verify(). The third candidate, the test case's input snapshot, is ADR-043's
// open question 2 and is not wired here: "the skill quoted my own input back at
// me" must not quietly become a way to satisfy an evidence requirement.
type source struct {
	kind  string
	text  string
	event trace.EventView // set only when kind is KindTraceEvent
}

// verifiableSources lists them in search order: the final output first, because a
// hit there carries a char range, then the trace entries.
//
// Trace order, not map order. digest is a map and ranging it would make the event
// a report cites depend on Go's hash seed; the same evaluation has to cite the
// same event twice running. Only ids the digest carries are searched, which keeps
// the citation set exactly what buildDigest sent.
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

// findQuote searches every verifiable source, exact hits before normalized ones.
// Two passes on purpose: an exact hit in the last source is a stronger fact than a
// normalized hit in the first, and the stronger fact is the one to record.
//
// The returned index is a byte offset into that source's text for an exact hit,
// and -1 for a normalized one.
func findQuote(quote string, sources []source) (source, int, string, bool) {
	for _, s := range sources {
		if i := strings.Index(s.text, quote); i >= 0 {
			return s, i, MatchExact, true
		}
	}
	nq := normalizeQuote(quote)
	if utf8.RuneCountInString(nq) < minNormalizedQuote {
		return source{}, -1, "", false
	}
	for _, s := range sources {
		if strings.Contains(normalizeQuote(s.text), nq) {
			return s, -1, MatchNormalized, true
		}
	}
	return source{}, -1, "", false
}

// traceSearchText is what a citation is checked against: the event payload as
// stored, plus every string value inside it decoded.
//
// Both halves, because a judge can legitimately quote either. The digest entry it
// was shown is `string(e.Payload)` - raw JSON - so a model that copies characters
// off the screen produces the first, and a model that reads `\\n` as a line break
// and writes a line break produces the second. Under the old check only the first
// could ever match, and the second is what the B round actually saw: all five
// downgraded rubric items cited real text whose only fault was being decoded
// (04 丙-48, report-judge-regression §14.4).
//
// The property defence 3 exists for is unchanged - the quote must still appear
// verbatim (or under ADR-043 §4 normalisation) in bytes this platform stored -
// because a decoded leaf is not new material, it is the same field with its
// escapes resolved. What stops being possible is a citation failing for the
// encoding it happened to be written in.
//
// Leaves are joined with NUL, which no JSON string can contain (it encodes as
// \\u0000) and which normalizeQuote does not collapse. So a quote can never match
// by spanning two fields: the separator is unquotable by construction rather than
// merely unlikely.
func traceSearchText(payload json.RawMessage) string {
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return string(payload) // unparseable: the raw form is all there is
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
			// Keys are field names the platform chose, not content; only values
			// can carry something a judge would quote.
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

// locate is findQuote for a single text: the same two attempts in the same order.
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

// minNormalizedQuote is ADR-043 §4's floor: below this length a quote is accepted
// only on an exact hit, never on a normalized one.
//
// Normalisation widens matching by construction, and a short string in a widened
// comparison hits by accident — `"ok"}` finds itself in almost any payload. A
// length floor is the cheapest compensation that does not require inventing a
// second notion of similarity.
//
// The number is a derivation and not a measurement. Nothing was counted to arrive
// at 12; it is sized to be longer than the fragments that collide and shorter than
// a real cited sentence. The B round (04 丙-8, `EVAL-013` — five real Runs, so a
// batch that costs money) is what would calibrate it against actual citations, and
// per §4 it may only ever move upward: lowering it would let through matches this
// threshold has already been asserted to stop.
//
// Runes, not bytes, for the same reason cut() counts runes: the product's own
// language writes 12 characters in 36 bytes.
const minNormalizedQuote = 12

// normalizeQuote is the bounded normalisation of ADR-043 §4, and the bound is part
// of the criterion rather than an implementation detail:
//
//  1. Unicode NFC;
//  2. runs of whitespace (full-width spaces, newlines, tabs) collapsed to one
//     half-width space;
//  3. leading and trailing structural punctuation trimmed.
//
// Ends only, never the middle: punctuation inside a quote is content. And nothing
// else — no lowercasing, no stripping of internal punctuation, no fuzzy or
// edit-distance matching. Each of those was rejected for one reason: they widen
// the match by an amount nobody can state, and a defence whose looseness cannot be
// stated is not a defence. What is here widens it by exactly the shapes G8 lost to.
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

// structuralPunctuation is §4's list, verbatim, plus the space that trimming a
// closer can expose. These are the characters a model's own serialisation leaks
// into a string value — G8's quote was correct and ended `}],` — and none of them
// carries meaning at the edge of a citation. Openers are not in the set: §4 lists
// closers, and a citation that begins mid-structure is not the failure that was
// measured.
const structuralPunctuation = "}]),;\"'`「」『』 "

// traceQuoteRef cites a trace event for a quote found in its payload. No char
// range: a trace citation is addressed by (event id, occurred_at), and on a
// normalized match an offset would point into a normalized copy that is not what
// the report stores.
func traceQuoteRef(e trace.EventView, quote, match string) EvidenceRef {
	out := traceRef(e, e.Type)
	out.Excerpt, out.ExcerptTruncated = cut(verifiedText(quote, match), excerptLimit)
	out.Match = match
	return out
}

// outputRef cites the agent's final output. idx is a byte offset for an exact hit
// and -1 for a normalized one; a normalized hit gets no char range, because the
// offset that was found addresses the normalized copy, and pointing a reader at it
// is worse than pointing them at nothing.
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

// verifiedText is what the excerpt stores: the string that was actually checked.
// On a normalized match that is the normalized quote and not what the model sent,
// so a reader is never shown a form of the quote that nothing verified. ADR-043's
// clean-up of `excerpt` semantics is exactly this — it is always the verified
// text, and never the platform's own manifest line standing in for one.
func verifiedText(quote, match string) string {
	if match == MatchNormalized {
		return normalizeQuote(quote)
	}
	return quote
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
