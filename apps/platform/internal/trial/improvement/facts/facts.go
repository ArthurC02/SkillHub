package facts

import (
	"time"

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
