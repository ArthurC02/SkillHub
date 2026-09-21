package run

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestTransitionCommandUsesItsTargetStatus(t *testing.T) {
	command := TransitionCommand{
		WorkspaceID:  pgtype.UUID{Valid: true},
		RunID:        pgtype.UUID{Valid: true},
		AttemptID:    pgtype.UUID{Valid: true},
		From:         StatusQueued,
		To:           StatusProvisioning,
		Reason:       "provider selected",
		FailureClass: failureProvider,
		Actor:        pgtype.UUID{Valid: true},
	}

	got := transitionCommandParams(command)
	if got.from != gen.RunStatusQueued {
		t.Errorf("from = %q, want %q", got.from, gen.RunStatusQueued)
	}
	if got.to != gen.RunStatusProvisioning {
		t.Errorf("to = %q, want %q", got.to, gen.RunStatusProvisioning)
	}
	if got.reason != statusReason("provider selected") {
		t.Errorf("reason = %q, want provider selected", got.reason)
	}
	if got.failure != failureProvider {
		t.Errorf("failure = %q, want %q", got.failure, failureProvider)
	}
}
