package ingest

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

var errFirstAcquireRefused = errors.New("the first connection of this pool is refused")

func poolRefusingItsFirstConnection(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv(creationDBURLEnv))
	if err != nil {
		t.Fatal(err)
	}
	var acquired atomic.Int32
	cfg.PrepareConn = func(context.Context, *pgx.Conn) (bool, error) {
		if acquired.Add(1) == 1 {
			return true, errFirstAcquireRefused
		}
		return true, nil
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestAGenerationWhoseAllowanceCannotBeCountedIsRefusedAndRecordedAsUnavailable(t *testing.T) {
	requireCreationDB(t)
	ws := seedCreationWorkspace(t, creationPool, "generate-allowance-unreadable")
	svc := &Service{
		Pool:          poolRefusingItsFirstConnection(t),
		LLM:           ModelOrNone(&llmclient.Client{BaseURL: "http://127.0.0.1:1"}),
		GenerateQuota: policy.DefaultGenerateQuotaLimits(),
	}

	out, err := svc.GenerateSkill(context.Background(), ws, GenerateInput{TaskDescription: "把掃描的單據整理成表格。"})
	if !errors.Is(err, policy.ErrAllowanceUnavailable) || !errors.Is(err, errFirstAcquireRefused) || out.Attempts != 0 {
		t.Fatalf("attempts=%d err=%v, want no attempt and ErrAllowanceUnavailable wrapping the refused connection", out.Attempts, err)
	}

	var failure, reason string
	var attempts int
	if err := creationPool.QueryRow(context.Background(),
		`SELECT metadata->>'failure', metadata->>'reason', (metadata->>'attempts')::int FROM audit_events
		 WHERE workspace_id = $1 AND action = $2`, ws.ID, audit.ActionSkillGenerateFailed,
	).Scan(&failure, &reason, &attempts); err != nil {
		t.Fatalf("the refusal left no single failure record: %v", err)
	}
	if failure != FailureUnavailable || reason != "generate_quota_unavailable" || attempts != 0 {
		t.Errorf("failure=%q reason=%q attempts=%d, want %q/generate_quota_unavailable/0", failure, reason, attempts, FailureUnavailable)
	}
}
