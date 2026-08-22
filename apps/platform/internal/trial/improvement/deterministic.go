package eval

// The deterministic leg of EVAL-001: five of the six problem classes, decided by
// rules over records the platform wrote itself. Nothing here calls a model, and
// nothing here executes anything — a package is bytes handed to skillpkg.Validate,
// a trace event is stored JSON, an artifact is a manifest row (evaluation-design
// §2.5). Because the inputs are the platform's own, these five cannot be argued
// with by anything that ran inside the sandbox.
//
// The sixth class, `effect`, is in judge.go. It is the only one no rule can answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// deterministicFindings runs all five rule checks. Order is the order EVAL-001
// clause 1 lists the classes in, so the report reads the way the spec does.
func (s *Service) deterministicFindings(_ context.Context, m material) []Finding {
	out := []Finding{}
	out = append(out, specFindings(m)...)
	out = append(out, activationFindings(m)...)
	out = append(out, executionFindings(m)...)
	out = append(out, artifactFindings(m)...)
	out = append(out, compatibilityFindings(m)...)
	out = append(out, costFindings(m)...)
	return out
}

// specFindings reports what static validation says about the version that ran.
// Re-validated rather than read from a stored verdict for the same reason
// internal/run's gate B re-validates: the only persisted projection is a
// warning-count for the catalogue UI, and a gate reading it would be reading a
// cache of a decision instead of the decision.
func specFindings(m material) []Finding {
	if !m.reportOK {
		// SEC-002's rule, restated: a check that could not be performed is not a
		// check that passed. An unreadable package is not a clean one.
		return []Finding{{
			Category: CategorySpec, Severity: SeverityWarning,
			Message: "the skill package could not be read back, so its specification " +
				"could not be re-validated for this evaluation; this is not the same as a clean result",
			Evidence: []EvidenceRef{},
		}}
	}
	out := []Finding{}
	for _, f := range m.report.Findings {
		if f.Severity == skillpkg.SeverityInfo {
			// Disclosure, not a problem: scripts, URLs and dependency files are
			// already shown by the import and permission surfaces.
			continue
		}
		message := f.Code + ": " + f.Message
		if f.Path != "" {
			message = f.Code + " (" + f.Path + "): " + f.Message
		}
		out = append(out, Finding{
			Category: CategorySpec, Severity: string(f.Severity),
			Message: message, Evidence: []EvidenceRef{},
		})
	}
	return out
}

// activationFindings compares the skills the run mounted against the activations
// the trace shows.
//
// The wording ceiling is a requirement, not a style choice (handoff 丙-4): the
// Agent SDK message stream cannot express "offered and not chosen", so the only
// honest statement about a missing activation is that no activation event
// appeared. This function must never produce a sentence claiming the model saw
// the skill and declined it.
func activationFindings(m material) []Finding {
	activated := map[string]bool{}
	skipped := map[string]bool{}
	var evidence []EvidenceRef
	for _, e := range m.advanced.Events {
		if e.Type != trace.TypeSkillActivation {
			continue
		}
		var p struct {
			SkillVersionID string `json:"skill_version_id"`
			Decision       string `json:"decision"`
		}
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		switch p.Decision {
		case "activated":
			activated[p.SkillVersionID] = true
			evidence = append(evidence, traceRef(e, e.Type))
		case "skipped":
			skipped[p.SkillVersionID] = true
			evidence = append(evidence, traceRef(e, e.Type))
		}
	}

	mounted := pgconv.UUIDString(m.run.SkillVersionID)
	switch {
	case activated[mounted]:
		return []Finding{{
			Category: CategoryActivation, Severity: SeverityInfo,
			Message:  "the skill version mounted for this run was activated",
			Evidence: orEmpty(evidence),
		}}
	case skipped[mounted]:
		return []Finding{{
			Category: CategoryActivation, Severity: SeverityWarning,
			Message:  "the run trace records the skill version as skipped",
			Evidence: orEmpty(evidence),
		}}
	default:
		return []Finding{{
			Category: CategoryActivation, Severity: SeverityWarning,
			Message: "no activation event appeared for the skill version mounted for this run. " +
				"That is all the trace can say: whether the agent saw the skill and used " +
				"something else instead is not observable in the message stream (TRACE-002)",
			Evidence: orEmpty(evidence),
		}}
	}
}

// executionFindings reports how the run itself ended: the state machine's own
// verdict plus every error event the trace carries.
func executionFindings(m material) []Finding {
	out := []Finding{}
	severity := SeverityInfo
	message := "the workload ran to its own end and reported success. That is an " +
		"execution outcome and not a task verdict (ADR-025)"
	if m.run.Status != "succeeded" {
		severity = SeverityError
		message = fmt.Sprintf("the run ended as %s", m.run.Status)
		if m.run.FailureClass != nil && *m.run.FailureClass != "" {
			message += ", classified " + *m.run.FailureClass
		}
		if m.run.StatusReason != nil && *m.run.StatusReason != "" {
			message += ": " + *m.run.StatusReason
		}
	}
	out = append(out, Finding{
		Category: CategoryExecution, Severity: severity,
		Message: message, Evidence: []EvidenceRef{},
	})

	for _, e := range m.advanced.Events {
		if e.Type != trace.TypeError {
			continue
		}
		var p struct {
			Category string `json:"category"`
			Code     string `json:"code"`
			Message  string `json:"message"`
		}
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		out = append(out, Finding{
			Category: CategoryExecution, Severity: SeverityError,
			Message:  fmt.Sprintf("error event (%s/%s): %s", p.Category, p.Code, p.Message),
			Evidence: []EvidenceRef{traceRef(e, e.Type)},
		})
	}

	if !m.advanced.Complete {
		out = append(out, Finding{
			Category: CategoryExecution, Severity: SeverityWarning,
			Message: "the run trace has gaps, so the evidence behind every judgement below " +
				"may be incomplete; no criterion may be recorded as passed on it (ADR-009)",
			Evidence: []EvidenceRef{},
		})
	}
	return out
}

// artifactFindings is handoff 丙-5 made checkable: the concrete M2 case was a run
// that finished `succeeded`, activated its skill, had a complete trace, and wrote
// nothing to /out/artifacts — its final reply was a question back to the user.
//
// Only the manifest is read. Nothing is unpacked and nothing is parsed by file
// extension (evaluation-design §2.2).
//
// Filed under `execution` and not `effect`, although it is plainly about effect:
// public.yaml reserves `effect` for the one category a model produces, and
// deterministic_findings carries no `source` field, so a rule-written `effect`
// entry would read as a model verdict. Whether the task needed a file is the
// judge's question; that a successful run wrote none is an execution fact.
func artifactFindings(m material) []Finding {
	if len(m.artifacts) == 0 {
		if m.run.Status != "succeeded" {
			// A run that did not finish has an obvious reason to have produced
			// nothing, and saying so twice adds no information.
			return nil
		}
		return []Finding{{
			Category: CategoryExecution, Severity: SeverityWarning,
			Message: "the run reported success and no output files were recorded for it. " +
				"Whether the task needed a file is for the criteria below to say",
			Evidence: []EvidenceRef{},
		}}
	}
	names := make([]string, 0, len(m.artifacts))
	evidence := make([]EvidenceRef, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		names = append(names, a.FileName)
		evidence = append(evidence, artifactRef(a))
	}
	return []Finding{{
		Category: CategoryExecution, Severity: SeverityInfo,
		Message:  fmt.Sprintf("the run recorded %d output file(s): %s", len(names), strings.Join(names, ", ")),
		Evidence: evidence,
	}}
}

// compatibilityFindings compares the measured (version, runtime image) pair of
// 0022 against the image this run actually ran on.
func compatibilityFindings(m material) []Finding {
	if m.compat == nil {
		return []Finding{{
			Category: CategoryCompatibility, Severity: SeverityInfo,
			Message: "no compatibility measurement exists for this skill version, " +
				"so nothing is claimed about it either way",
			Evidence: []EvidenceRef{},
		}}
	}
	var snapshot struct {
		Runtime struct {
			Runtime        string `json:"runtime"`
			RuntimeVersion string `json:"runtime_version"`
		} `json:"runtime"`
	}
	_ = json.Unmarshal(m.run.RuntimeSnapshot, &snapshot)

	message := fmt.Sprintf("compatibility measured as %q on runtime image %s (%s)",
		m.compat.Capability, m.compat.RuntimeImage, m.compat.Runtime)
	severity := SeverityInfo
	if m.compat.Capability != "activated" {
		severity = SeverityWarning
	}
	if ran := snapshot.Runtime.Runtime; ran != "" && ran != m.compat.Runtime {
		severity = SeverityWarning
		message += fmt.Sprintf("; this run used runtime %s %s, so the measurement does not cover it",
			ran, snapshot.Runtime.RuntimeVersion)
	}
	return []Finding{{
		Category: CategoryCompatibility, Severity: severity,
		Message: message, Evidence: []EvidenceRef{},
	}}
}

// costFindings reports what the run's own usage events add up to — and that the
// total is a lower bound (handoff 丙-3).
//
// This is the run's cost, never the evaluation's. The two are spent by different
// workloads under different keys and are never summed into one number; the
// evaluation's own spend lives in evaluations.cost_usd.
func costFindings(m material) []Finding {
	if m.summary.Usage == nil {
		return []Finding{{
			Category: CategoryCost, Severity: SeverityWarning,
			Message: "this run reported no usage at all, so its cost is unknown rather than zero. " +
				"The authoritative figure is the gateway's per-key spend (ADR-017)",
			Evidence: []EvidenceRef{},
		}}
	}
	u := m.summary.Usage
	message := fmt.Sprintf("the run used %d input and %d output tokens on %s",
		u.InputTokens, u.OutputTokens, orUnknown(u.Model))
	if u.CostUSD == nil {
		message += "; no cost was reported, which is unreported and not $0"
	} else {
		message += fmt.Sprintf("; reported cost $%.4f (%s)", *u.CostUSD, orUnknown(u.CostSource))
	}
	message += ". This total is a lower bound: a response still in flight when the " +
		"stream ended is not in it, and the authoritative figure is the gateway's " +
		"per-key spend (ADR-017)"
	return []Finding{{
		Category: CategoryCost, Severity: SeverityInfo,
		Message: message, Evidence: []EvidenceRef{},
	}}
}

// traceRef cites one trace event. The (id, occurred_at) pair is the whole address:
// trace_events is partitioned by time and keyed on both.
func traceRef(e trace.EventView, kind string) EvidenceRef {
	excerpt, truncated := cut(string(e.Payload), excerptLimit)
	return EvidenceRef{
		Kind:             KindTraceEvent,
		TraceEventID:     e.EventID,
		OccurredAt:       e.OccurredAt,
		Excerpt:          kind + " " + excerpt,
		ExcerptTruncated: truncated,
		Available:        true,
	}
}

// artifactRef cites one manifest row. No byte range: the bytes live inside the
// attempt's archive and this pipeline never opens it, so there is no offset that
// would mean anything.
func artifactRef(a ArtifactFacts) EvidenceRef {
	return EvidenceRef{
		Kind:         KindArtifact,
		ArtifactPath: a.FileName,
		Excerpt: fmt.Sprintf("%s (%d bytes, %s, %s)",
			a.FileName, a.SizeBytes, orUnknown(a.ContentType), a.ContentHash),
		Available: true,
	}
}

func orEmpty(refs []EvidenceRef) []EvidenceRef {
	if refs == nil {
		return []EvidenceRef{}
	}
	return refs
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
