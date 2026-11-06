package eval

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func uuidN(n byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{n}, Valid: true} }

func TestAnImprovedVersionEventDecodesItsParentWithAndWithoutAReflection(t *testing.T) {
	const evaluation = "01000000-0000-0000-0000-000000000000"
	const reflection = "02000000-0000-0000-0000-000000000000"
	for _, tc := range []struct {
		name, payload          string
		evaluation, reflection pgtype.UUID
	}{
		{"an evaluation's, written before reflections existed",
			`{"version_id":"` + evaluation + `","improved_by":{"evaluation_id":"` + evaluation + `","suggestion_ids":[]}}`,
			uuidN(1), pgtype.UUID{}},
		{"a reflection's",
			`{"version_id":"` + evaluation + `","improved_by":{"evaluation_id":null,"reflection_id":"` + reflection + `","suggestion_ids":[]}}`,
			pgtype.UUID{}, uuidN(2)},
	} {
		args, improved, err := suggestionsApplied(outbox.Event{Payload: []byte(tc.payload)})
		if err != nil || !improved {
			t.Fatalf("%s: improved %v, err %v", tc.name, improved, err)
		}
		if args.EvaluationID != tc.evaluation || args.ReflectionID != tc.reflection {
			t.Errorf("%s: evaluation %v reflection %v, want %v and %v", tc.name, args.EvaluationID, args.ReflectionID, tc.evaluation, tc.reflection)
		}
	}
}

func TestARunIsWorthReadingOnlyWhenItsCompletedEvaluationLeftSomethingToFix(t *testing.T) {
	info := `[{"category":"execution","severity":"info","message":"ran to its end","evidence":[]}]`
	warning := `[{"category":"activation","severity":"warning","message":"no activation","evidence":[]}]`
	for _, tc := range []struct {
		name, status, overall, findings string
		worth                           bool
	}{
		{"met with only info findings", "completed", "met", info, false},
		{"met with no findings", "completed", "met", "", false},
		{"met with a warning", "completed", "met", warning, true},
		{"not met", "completed", "not_met", info, true},
		{"undetermined", "completed", "undetermined", "", true},
		{"not completed", "failed", "not_met", warning, false},
	} {
		got, worth, err := failureOf(uuidN(7), gen.Evaluation{
			Status: tc.status, Overall: tc.overall, DeterministicFindings: []byte(tc.findings),
		})
		if err != nil || worth != tc.worth {
			t.Errorf("%s: worth %v err %v, want %v", tc.name, worth, err, tc.worth)
			continue
		}
		if worth && got.RunID != uuidN(7) {
			t.Errorf("%s: run %v, want the evaluated run", tc.name, got.RunID)
		}
	}
	if _, worth, err := failureOf(uuidN(7), gen.Evaluation{
		Status: "completed", Overall: "met", DeterministicFindings: []byte("{"),
	}); err == nil || worth {
		t.Errorf("unreadable findings: worth %v err %v, want an error and nothing to read", worth, err)
	}
}
