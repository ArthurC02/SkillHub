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

func TestNodeAndPoolThresholds(t *testing.T) {
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{
		{"closed beta node, 2 slots at 50%", 2, 1},
		{"4 slots at 50%", 4, 2},
		{"early growth node, 8 slots at 50%", 8, 4},
		{"an undeclared or unreachable node falls back to the floor", 0, 1},

		{"3 slots at 50% rounds up, and the floor of 1 cannot hide it", 3, 2},
	} {
		if got := haltThreshold(tc.slots, 2, 1); got != tc.want {
			t.Errorf("node threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{

		{"closed beta pool, 4 slots at 25% under the floor", 4, 2},
		{"early growth pool, 20 slots at 25%", 20, 5},
		{"6 slots at 25% rounds up to a whole resource", 6, 2},
		{"no slots declared anywhere", 0, 2},
	} {
		if got := haltThreshold(tc.slots, 4, 2); got != tc.want {
			t.Errorf("pool threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
}

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
