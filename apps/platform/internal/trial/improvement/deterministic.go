package eval

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func (s *Service) deterministicFindings(m material) []Finding {
	out := []Finding{}
	out = append(out, specFindings(m)...)
	out = append(out, activationFindings(m)...)
	out = append(out, executionFindings(m)...)
	out = append(out, artifactFindings(m)...)
	out = append(out, compatibilityFindings(m)...)
	out = append(out, costFindings(m)...)
	return out
}

func specFindings(m material) []Finding {
	if !m.reportOK {

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

func artifactFindings(m material) []Finding {
	var out []Finding
	if m.absent.Any() {
		out = append(out, Finding{
			Category: CategoryExecution, Severity: SeverityWarning,
			Message: fmt.Sprintf("this run's output manifest has %d file(s) this evaluation cannot read (%s). "+
				"The run recorded them, so this is not a run that produced nothing, and nothing below "+
				"may be judged from their absence (02:EVAL-001, NFR-002a)",
				m.absent.Deleted+m.absent.Expired, absenceReasons(m.absent)),
			Evidence: []EvidenceRef{},
		})
	}
	if len(m.artifacts) == 0 {
		if m.absent.Any() || m.run.Status != "succeeded" {

			return out
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
	return append(out, Finding{
		Category: CategoryExecution, Severity: SeverityInfo,
		Message:  fmt.Sprintf("the run recorded %d readable output file(s): %s", len(names), strings.Join(names, ", ")),
		Evidence: evidence,
	})
}

func absenceReasons(a ArtifactAbsence) string {
	reasons := make([]string, 0, 2)
	if a.Deleted > 0 {
		reasons = append(reasons, fmt.Sprintf("%d deleted from this workspace", a.Deleted))
	}
	if a.Expired > 0 {
		reasons = append(reasons, fmt.Sprintf("%d past the retention stamped on them", a.Expired))
	}
	return strings.Join(reasons, ", ")
}

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

func traceRef(e trace.EventView, kind string) EvidenceRef {
	excerpt, truncated := cut(string(e.Payload), excerptLimit)
	return EvidenceRef{
		Kind:             KindTraceEvent,
		Match:            MatchExact,
		TraceEventID:     e.EventID,
		OccurredAt:       e.OccurredAt,
		Excerpt:          kind + " " + excerpt,
		ExcerptTruncated: truncated,
		Available:        true,
	}
}

func artifactRef(a ArtifactFacts) EvidenceRef {
	return EvidenceRef{
		Kind:         KindArtifact,
		Match:        MatchNotChecked,
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
