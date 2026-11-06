package eval

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
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
