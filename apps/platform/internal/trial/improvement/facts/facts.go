package facts

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

type Run struct {
	ID                 pgtype.UUID
	WorkspaceID        pgtype.UUID
	SkillVersionID     pgtype.UUID
	TestCaseSnapshotID pgtype.UUID
	Status             string
	StatusReason       *string
	RuntimeSnapshot    []byte
	StartedAt          *time.Time
	FinishedAt         *time.Time
	FailureClass       *string
	Terminal           bool
	Succeeded          bool
}

type Skill struct {
	ID               pgtype.UUID
	Name             string
	Summary          *string
	AccessRestricted bool
}

type Version struct {
	ID               pgtype.UUID
	SkillID          pgtype.UUID
	PackageObjectKey string
	SourcePath       string
}

func (v Version) Stored() skillpkg.StoredSkill {
	return skillpkg.StoredSkill{ObjectKey: v.PackageObjectKey, SourcePath: v.SourcePath}
}

type RuntimeCompatibility struct {
	Capability   string
	Runtime      string
	RuntimeImage string
}

type Artifact struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
}

type ArtifactAbsence struct {
	Deleted int
	Expired int
}

func (a ArtifactAbsence) Any() bool { return a.Deleted+a.Expired > 0 }

type EvaluationInput struct {
	Run           Run
	Artifacts     []Artifact
	Absent        ArtifactAbsence
	LatestAttempt int

	EarlierQuestions []trace.Question
}

type Material struct {
	Run      Run
	Version  Version
	Skill    Skill
	Snapshot testlab.Snapshot
	Criteria []testlab.Criterion

	Rubric *testlab.Rubric

	Graders      map[string]graders.Version
	Instructions map[string]string

	Advanced  trace.AdvancedView
	Summary   trace.Summary
	Artifacts []Artifact

	Absent ArtifactAbsence
	Compat *RuntimeCompatibility

	EarlierQuestions []trace.Question
	Report           skillpkg.Report
	ReportOK         bool
	Attempt          int
}

func (m Material) AwaitsAnswer() bool {
	return m.Run.Succeeded && len(m.Summary.Questions) > 0
}

func (m Material) EvidenceComplete() bool { return m.Advanced.Complete && !m.Absent.Any() }

func (m Material) CannotRecordPass(result string) bool {
	return result == verdict.ResultPassed && !m.EvidenceComplete()
}

func (m Material) FinalOutputCut() (reason string, cut bool) {
	if !m.Summary.FinalOutputTruncated {
		return "", false
	}
	return fmt.Sprintf("the final output was cut off at %d characters, so the rest of the answer cannot be checked",
		utf8.RuneCountInString(m.Summary.FinalOutput)), true
}

type ToolCall struct {
	Name       string
	Outcome    string
	DurationMS int64
	Arguments  string
}

type ToolCallLog struct {
	Calls []ToolCall
	Gap   string
}

func (l ToolCallLog) Complete() bool { return l.Gap == "" }

const (
	GapTraceTail      = "the trace kept for evaluation starts partway through the run, so its earlier calls are not visible"
	GapMorePages      = "the trace has pages that were not read"
	GapMissingEvents  = "the trace is missing events the run reported"
	GapUnreadablePart = "a tool call's record could not be read"
)

func traceGap(view trace.AdvancedView) string {
	switch {
	case view.EvaluationTruncated:
		return GapTraceTail
	case view.HasMore:
		return GapMorePages
	case !view.Complete:
		return GapMissingEvents
	}
	return ""
}

func (m Material) ToolCalls() ToolCallLog {
	view := m.Advanced
	log := ToolCallLog{Gap: traceGap(view)}
	for _, e := range view.Events {
		if e.Type != trace.TypeToolCall {
			continue
		}
		var p struct {
			ToolName   string          `json:"tool_name"`
			Outcome    string          `json:"outcome"`
			DurationMS int64           `json:"duration_ms"`
			Arguments  json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			if log.Gap == "" {
				log.Gap = GapUnreadablePart
			}
			continue
		}
		call := ToolCall{Name: p.ToolName, Outcome: p.Outcome, DurationMS: p.DurationMS}
		if len(p.Arguments) > 0 && string(p.Arguments) != "null" {
			call.Arguments = string(p.Arguments)
		}
		log.Calls = append(log.Calls, call)
	}
	return log
}
