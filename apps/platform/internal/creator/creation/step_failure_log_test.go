package creation

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAStepThatFailsAfterAGoodModelCallIsLogged(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	_, _ = (&Service{}).concludeAttempt(context.Background(), JobArgs{}, gen.CreationSession{}, &envelope{}, stepCall{})

	if !strings.Contains(logged.String(), "creation: step failed") || !strings.Contains(logged.String(), ErrUnavailable.Error()) {
		t.Errorf("log = %q, want the failure and its cause even though the model call itself returned no error", logged.String())
	}
}
