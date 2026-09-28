package ingest

import (
	"context"
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestARefusedGenerationRecordsTheShapeOfWhatWasAskedAgainstTheCaller(t *testing.T) {
	pool := requireCreationDB(t)
	ws := seedCreationWorkspace(t, pool, "generate-failure-audit-shape")
	removeAdmissionWorkspace(t, pool, ws)
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-00000000a0d1")
	const objectKey = "packages/audit-shape-reference.zip"
	svc := &Service{
		Pool:  pool,
		LLM:   ModelOrNone(&llmclient.Client{BaseURL: "http://127.0.0.1:1"}),
		Store: fakeObjectStore{objectKey: zipBytes(t, map[string]string{"SKILL.md": skillMD})},
		References: fakeReferenceReader{
			workspace: map[string]registry.Skill{
				pgconv.UUIDString(skillID): {ID: skillID, WorkspaceID: ws.ID, Name: "pdf-tools", Redistribution: "self_supplied"},
			},
			versions: map[string]registry.Version{
				pgconv.UUIDString(skillID): {ID: skillID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: objectKey},
			},
		},
		CreditCanStart: func(context.Context, pgtype.UUID) (bool, error) { return false, nil },
	}
	const task = "  把掃描的單據整理成表格。  "

	_, err := svc.GenerateSkill(context.Background(), ws, GenerateInput{
		TaskDescription:   task,
		Diagram:           &GenerateDiagram{MediaType: "image/png", Data: []byte{0x89, 'P', 'N', 'G'}},
		ReferenceSkillIDs: []pgtype.UUID{skillID},
	})
	if !errors.Is(err, ErrCreditThreshold) {
		t.Fatalf("err = %v, want ErrCreditThreshold", err)
	}

	var actor pgtype.UUID
	var failure string
	var chars, references, attempts int
	var diagram bool
	if err := pool.QueryRow(context.Background(),
		`SELECT actor_user_id, metadata->>'failure', (metadata->>'task_description_chars')::int,
		        (metadata->>'diagram')::bool, (metadata->>'references')::int, (metadata->>'attempts')::int
		   FROM audit_events WHERE workspace_id = $1 AND action = $2`, ws.ID, audit.ActionSkillGenerateFailed,
	).Scan(&actor, &failure, &chars, &diagram, &references, &attempts); err != nil {
		t.Fatalf("the refusal left no single failure record: %v", err)
	}
	if actor != ws.OwnerUserID || failure != FailureCredit {
		t.Errorf("actor=%v failure=%q, want the workspace owner and %q", actor, failure, FailureCredit)
	}
	if want := utf8.RuneCountInString("把掃描的單據整理成表格。"); chars != want || !diagram || references != 1 || attempts != 0 {
		t.Errorf("task_description_chars=%d diagram=%v references=%d attempts=%d, want %d/true/1/0",
			chars, diagram, references, attempts, want)
	}
}
