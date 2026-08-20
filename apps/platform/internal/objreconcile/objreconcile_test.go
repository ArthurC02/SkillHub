package objreconcile

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// Nil Pool and nil Store are deliberate throughout: both checks under test must
// be reached before anything is queried, and a nil dereference would be a louder
// failure than a passing assertion.

func noMark(context.Context, *gen.Queries, pgtype.UUID) error { return nil }

// The corrections belong to packaging and testlab and arrive by injection
// (ADR-033 clearance path 4), which makes "nobody wired them" a silent failure
// mode by construction: they are plain function values, and a sweep that skipped
// its writes would log exactly what a clean sweep logs. So it refuses instead.
func TestSweepRefusesToRunWithoutTheOwnerWrites(t *testing.T) {
	for name, svc := range map[string]*Service{
		"neither injected":        {},
		"only the artifact write": {RecordArtifactPurged: noMark},
		"only the dataset write":  {RecordDatasetLost: noMark},
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.Sweep(context.Background()); err == nil {
				t.Error("Sweep ran with an owner write missing")
			}
		})
	}
}

// A deployment with no object store is a working one: there is nothing to
// reconcile against, so the sweep does nothing and says so by succeeding. This
// pins the order of the two checks — putting the fail-closed one after it would
// let a mis-wired service hide behind an absent store.
func TestSweepWithoutAStoreIsANoOp(t *testing.T) {
	svc := &Service{RecordArtifactPurged: noMark, RecordDatasetLost: noMark}
	if err := svc.Sweep(context.Background()); err != nil {
		t.Errorf("Sweep with no store: %v", err)
	}
}
