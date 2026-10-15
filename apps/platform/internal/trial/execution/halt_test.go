package run

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAFleetVerdictIsNotReachedWhenAProvidersLeakCountIsUnreadable(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := &Service{Pool: pool, Providers: &Registry{Providers: []SandboxProvider{answering(t, http.StatusOK)}}}
	svc.EvaluateOrphanThresholds(context.Background())

	if !strings.Contains(logged.String(), "counting persistent orphans") {
		t.Fatalf("log = %q, want the unreadable count reported", logged.String())
	}
	if strings.Contains(logged.String(), "target=pool") {
		t.Errorf("log = %q, want no fleet-wide verdict from a total missing a provider's leaks", logged.String())
	}
}
