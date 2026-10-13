package checks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

func Findings(m facts.Material) []verdict.Finding {
	out := []verdict.Finding{}
	out = append(out, specFindings(m)...)
	out = append(out, activationFindings(m)...)
	out = append(out, executionFindings(m)...)
	out = append(out, artifactFindings(m)...)
	out = append(out, compatibilityFindings(m)...)
	out = append(out, costFindings(m)...)
	return out
}

func specFindings(m facts.Material) []verdict.Finding {
	if !m.ReportOK {

		return []verdict.Finding{verdict.NewFinding(verdict.CategorySpec, verdict.SeverityWarning,
			"the skill package could not be read back, so its specification "+
				"could not be re-validated for this evaluation; this is not the same as a clean result")}
	}
	out := []verdict.Finding{}
	for _, f := range m.Report.Findings {
		if f.Severity == skillpkg.SeverityInfo {

			continue
		}
		message := f.Code + ": " + f.Message
		if f.Path != "" {
			message = f.Code + " (" + f.Path + "): " + f.Message
		}
		out = append(out, verdict.NewFinding(verdict.CategorySpec, string(f.Severity), message))
	}
	return out
}

func activationFindings(m facts.Material) []verdict.Finding {
	activated := map[string]bool{}
	skipped := map[string]bool{}
	var evidence []verdict.EvidenceRef
	for _, e := range m.Advanced.Events {
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
			evidence = append(evidence, TraceRef(e, e.Type))
		case "skipped":
			skipped[p.SkillVersionID] = true
			evidence = append(evidence, TraceRef(e, e.Type))
		}
	}

	mounted := pgconv.UUIDString(m.Run.SkillVersionID)
	switch {
	case activated[mounted]:
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryActivation, verdict.SeverityInfo,
			"the skill version mounted for this run was activated", evidence...)}
	case skipped[mounted]:
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryActivation, verdict.SeverityWarning,
			"the run trace records the skill version as skipped", evidence...)}
	default:
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryActivation, verdict.SeverityWarning,
			"no activation event appeared for the skill version mounted for this run. "+
				"That is all the trace can say: whether the agent saw the skill and used "+
				"something else instead is not observable in the message stream (TRACE-002)", evidence...)}
	}
}

func executionFindings(m facts.Material) []verdict.Finding {
	out := []verdict.Finding{}
	severity := verdict.SeverityInfo
	message := "the workload ran to its own end and reported success. That is an " +
		"execution outcome and not a task verdict"
	if !m.Run.Succeeded {
		severity = verdict.SeverityError
		message = fmt.Sprintf("the run ended as %s", m.Run.Status)
		if m.Run.FailureClass != nil && *m.Run.FailureClass != "" {
			message += ", classified " + *m.Run.FailureClass
		}
		if m.Run.StatusReason != nil && *m.Run.StatusReason != "" {
			message += ": " + *m.Run.StatusReason
		}
	}
	out = append(out, verdict.NewFinding(verdict.CategoryExecution, severity, message))

	for _, e := range m.Advanced.Events {
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
		out = append(out, verdict.NewFinding(verdict.CategoryExecution, verdict.SeverityError,
			fmt.Sprintf("error event (%s/%s): %s", p.Category, p.Code, p.Message), TraceRef(e, e.Type)))
	}

	if !m.Advanced.Complete {
		out = append(out, verdict.NewFinding(verdict.CategoryExecution, verdict.SeverityWarning,
			"the run trace has gaps, so the evidence behind every judgement below "+
				"may be incomplete; no criterion may be recorded as passed on it"))
	}
	return out
}

func artifactFindings(m facts.Material) []verdict.Finding {
	var out []verdict.Finding
	if m.Absent.Any() {
		out = append(out, verdict.NewFinding(verdict.CategoryExecution, verdict.SeverityWarning,
			fmt.Sprintf("this run's output manifest has %d file(s) this evaluation cannot read (%s). "+
				"The run recorded them, so this is not a run that produced nothing, and nothing below "+
				"may be judged from their absence (02:EVAL-001, NFR-002a)",
				m.Absent.Deleted+m.Absent.Expired, absenceReasons(m.Absent))))
	}
	if len(m.Artifacts) == 0 {
		if m.Absent.Any() || !m.Run.Succeeded {

			return out
		}
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryExecution, verdict.SeverityWarning,
			"the run reported success and no output files were recorded for it. "+
				"Whether the task needed a file is for the criteria below to say")}
	}
	names := make([]string, 0, len(m.Artifacts))
	evidence := make([]verdict.EvidenceRef, 0, len(m.Artifacts))
	for _, a := range m.Artifacts {
		names = append(names, a.FileName)
		evidence = append(evidence, ArtifactRef(a))
	}
	return append(out, verdict.NewFinding(verdict.CategoryExecution, verdict.SeverityInfo,
		fmt.Sprintf("the run recorded %d readable output file(s): %s", len(names), strings.Join(names, ", ")), evidence...))
}

func absenceReasons(a facts.ArtifactAbsence) string {
	reasons := make([]string, 0, 2)
	if a.Deleted > 0 {
		reasons = append(reasons, fmt.Sprintf("%d deleted from this workspace", a.Deleted))
	}
	if a.Expired > 0 {
		reasons = append(reasons, fmt.Sprintf("%d past the retention stamped on them", a.Expired))
	}
	return strings.Join(reasons, ", ")
}

type capability string

const capabilityActivated capability = "activated"

func compatibilityFindings(m facts.Material) []verdict.Finding {
	if m.Compat == nil {
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryCompatibility, verdict.SeverityInfo,
			"no compatibility measurement exists for this skill version, "+
				"so nothing is claimed about it either way")}
	}
	severity := verdict.SeverityInfo
	if capability(m.Compat.Capability) != capabilityActivated {
		severity = verdict.SeverityWarning
	}
	message := fmt.Sprintf(
		"compatibility measured as %q on runtime image %s (%s); this run records no runtime image, "+
			"so whether the measurement covers it is not claimed",
		m.Compat.Capability, m.Compat.RuntimeImage, m.Compat.Runtime)
	return []verdict.Finding{verdict.NewFinding(verdict.CategoryCompatibility, severity, message)}
}

func costFindings(m facts.Material) []verdict.Finding {
	if m.Summary.Usage == nil {
		return []verdict.Finding{verdict.NewFinding(verdict.CategoryCost, verdict.SeverityWarning,
			"this run reported no usage at all, so its cost is unknown rather than zero. "+
				"The authoritative figure is the gateway's per-key spend")}
	}
	u := m.Summary.Usage
	message := fmt.Sprintf("the run used %d input and %d output tokens on %s",
		u.InputTokens, u.OutputTokens, orUnknown(u.Model))
	if u.CostUSD == nil {
		message += "; no cost was reported, which is unreported and not $0"
	} else {
		message += fmt.Sprintf("; reported cost $%.4f (%s)", *u.CostUSD, orUnknown(u.CostSource))
	}
	message += ". This total is a lower bound: a response still in flight when the " +
		"stream ended is not in it, and the authoritative figure is the gateway's " +
		"per-key spend"
	return []verdict.Finding{verdict.NewFinding(verdict.CategoryCost, verdict.SeverityInfo, message)}
}

func TraceRef(e trace.EventView, kind string) verdict.EvidenceRef {
	excerpt, truncated := verdict.Excerpt(string(e.Payload))
	return verdict.EvidenceRef{
		Kind:             verdict.KindTraceEvent,
		Match:            verdict.MatchExact,
		TraceEventID:     e.EventID,
		OccurredAt:       e.OccurredAt,
		Excerpt:          kind + " " + excerpt,
		ExcerptTruncated: truncated,
		Available:        true,
	}
}

func ArtifactRef(a facts.Artifact) verdict.EvidenceRef {
	return verdict.EvidenceRef{
		Kind:         verdict.KindArtifact,
		Match:        verdict.MatchNotChecked,
		ArtifactPath: a.FileName,
		Excerpt: fmt.Sprintf("%s (%d bytes, %s, %s)",
			a.FileName, a.SizeBytes, orUnknown(a.ContentType), a.ContentHash),
		Available: true,
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
