package apiserver_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type jobRecorder struct {
	calls []creation.JobArgs
}

func (r *jobRecorder) insert(_ context.Context, _ pgx.Tx, a creation.JobArgs) error {
	r.calls = append(r.calls, a)
	return nil
}

func newCreationWorkspace(t *testing.T, pool *pgxpool.Pool) identity.Workspace {
	t.Helper()
	ctx := context.Background()
	email := "creation-start-" + creation.UUID(creationID(t)) + "@example.test"
	var userID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, display_name) VALUES ($1,$1) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var wsID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (owner_user_id, name) VALUES ($1,$2) RETURNING id`, userID, email).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	return identity.Workspace{ID: wsID}
}

func newCreateService(pool *pgxpool.Pool, rec *jobRecorder) *creation.Service {
	return &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert}
}

func TestCreateRejectsMalformedCommand(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	valid := creationID(t)
	cases := []struct {
		name    string
		id      pgtype.UUID
		message string
		budget  float64
	}{
		{"invalid id", pgtype.UUID{}, "", .5},
		{"NaN budget", valid, "", math.NaN()},
		{"positive infinite budget", valid, "", math.Inf(1)},
		{"message over the 4000 rune limit", valid, strings.Repeat("字", 4001), .5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), ws, tc.id, tc.message, tc.budget)
			if !errors.Is(err, creation.ErrInvalidCommand) {
				t.Fatalf("got %v, want ErrInvalidCommand", err)
			}
		})
	}
}

func TestCreateAcceptsMessageAtTheRuneLimit(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := newCreateService(pool, rec)
	msg := strings.Repeat("字", 4000)
	v, err := svc.Create(context.Background(), ws, creationID(t), msg, .5)
	if err != nil {
		t.Fatalf("Create with a 4000-rune message: %v", err)
	}
	if v.State != "queued" {
		t.Fatalf("state = %q, want queued", v.State)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(rec.calls))
	}
}

func TestCreateBudgetAtTheLowerFloorIsAcceptedJustBelowIsNot(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})

	at := svc.Limits.MaxCallCostUSD
	if _, err := svc.Create(context.Background(), ws, creationID(t), "", at); err != nil {
		t.Fatalf("budget at the floor: %v", err)
	}

	below := math.Nextafter(at, 0)
	_, err := svc.Create(context.Background(), ws, creationID(t), "", below)
	if !errors.Is(err, creation.ErrBudgetOutOfBand) {
		t.Fatalf("budget just below the floor: got %v, want ErrBudgetOutOfBand", err)
	}
}

func TestCreateReplaySameInputReturnsExistingViewWithoutReinserting(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := newCreateService(pool, rec)
	id := creationID(t)
	first, err := svc.Create(context.Background(), ws, id, "hello", .5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("initial create inserted %d jobs, want 1", len(rec.calls))
	}

	second, err := svc.Create(context.Background(), ws, id, "hello", .5)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != first.Revision || second.State != first.State {
		t.Fatalf("replay = %+v, want the same view %+v", second, first)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("replay inserted another job: %d calls", len(rec.calls))
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM creation_sessions WHERE id=$1", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("sessions for id = %d, want 1", rows)
	}
}

func TestCreateReplayWithADifferentMessageIsAMismatch(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "hello", .5); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), ws, id, "different", .5)
	if !errors.Is(err, creation.ErrReplayMismatch) {
		t.Fatalf("got %v, want ErrReplayMismatch", err)
	}
}

func TestCreateOnAnExpiredSessionIsNotFound(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "hello", .5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE creation_sessions SET expires_at = now() - interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), ws, id, "hello", .5)
	if !errors.Is(err, creation.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestCreateWithAnUndecodableSnapshotSurfacesTheDecodeError(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "hello", .5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = '{"snapshot": 5}'::jsonb WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), ws, id, "hello", .5)
	if err == nil || errors.Is(err, creation.ErrReplayMismatch) || errors.Is(err, creation.ErrNotFound) {
		t.Fatalf("got %v, want a decode error that is neither ErrReplayMismatch nor ErrNotFound", err)
	}
}

func TestCreateContinuesQueuedWhenCatalogCheckFails(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := newCreateService(pool, rec)
	svc.CatalogCheck = func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
		return nil, 0, errors.New("catalog down")
	}
	v, err := svc.Create(context.Background(), ws, creationID(t), "請建立摘要", .5)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "queued" || v.Snapshot.CatalogChecked {
		t.Fatalf("catalog-check failure: state=%q catalogChecked=%v, want queued/false", v.State, v.Snapshot.CatalogChecked)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(rec.calls))
	}
}

func TestCreateCatalogCheckReferencesAreCappedAtThreeAndSkipTheQueue(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := newCreateService(pool, rec)
	refs := make([]creation.Reference, 5)
	for i := range refs {
		refs[i] = creation.Reference{SkillID: fmt.Sprintf("skill-%d", i), Confirmed: true}
	}
	svc.CatalogCheck = func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
		return refs, .02, nil
	}
	v, err := svc.Create(context.Background(), ws, creationID(t), "請建立摘要", .5)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "waiting_confirmation" || v.Snapshot.PendingAction != "confirm_references" {
		t.Fatalf("state=%q pendingAction=%q, want waiting_confirmation/confirm_references", v.State, v.Snapshot.PendingAction)
	}
	if len(v.Snapshot.References) != 3 {
		t.Fatalf("references = %d, want 3", len(v.Snapshot.References))
	}
	for i, r := range v.Snapshot.References {
		if r.SkillID != fmt.Sprintf("skill-%d", i) || r.Confirmed {
			t.Fatalf("reference %d = %+v, want skill-%d unconfirmed", i, r, i)
		}
	}
	if v.Snapshot.SpentUSD == nil || *v.Snapshot.SpentUSD != .02 {
		t.Fatalf("spent = %v, want 0.02", v.Snapshot.SpentUSD)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("insert called %d times, want 0", len(rec.calls))
	}
}

func TestCreateCatalogCheckCostIsKeptEvenWhenTheCheckErrors(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := newCreateService(pool, rec)
	svc.CatalogCheck = func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
		return nil, .03, errors.New("catalog degraded but billed")
	}
	v, err := svc.Create(context.Background(), ws, creationID(t), "請建立摘要", .5)
	if err != nil {
		t.Fatal(err)
	}
	if v.Snapshot.SpentUSD == nil || *v.Snapshot.SpentUSD != .03 {
		t.Fatalf("spent = %v, want 0.03 even though the check errored", v.Snapshot.SpentUSD)
	}
	if v.Snapshot.CatalogChecked {
		t.Fatal("catalogChecked true despite the check erroring")
	}
}

func TestCreateWithAWorkspaceThatDoesNotExistIsAConflict(t *testing.T) {
	pool := requireDB(t)
	svc := newCreateService(pool, &jobRecorder{})
	var ghost pgtype.UUID
	if err := pool.QueryRow(context.Background(), "SELECT gen_random_uuid()").Scan(&ghost); err != nil {
		t.Fatal(err)
	}
	id := creationID(t)
	_, err := svc.Create(context.Background(), identity.Workspace{ID: ghost}, id, "", .5)
	if !errors.Is(err, creation.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM creation_sessions WHERE id=$1", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("sessions for id = %d, want 0", rows)
	}
}

func TestCreateWithTheSameIDInAnotherWorkspaceCreatesAnIndependentSession(t *testing.T) {
	pool := requireDB(t)
	ws1 := newCreationWorkspace(t, pool)
	ws2 := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws1, id, "one workspace's message", .1); err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(context.Background(), ws2, id, "a different workspace's message", .1)
	if err != nil {
		t.Fatalf("got %v, want a second independent session", err)
	}
	if second.State != "queued" {
		t.Fatalf("second workspace's session state = %q, want queued", second.State)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM creation_sessions WHERE id=$1", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("sessions sharing id %s = %d, want 2 independent rows", creation.UUID(id), rows)
	}
}

func TestCreateWhenInsertFailsReturnsTheErrorAndLeavesNoSession(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	sentinel := errors.New("insert boom")
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: func(context.Context, pgx.Tx, creation.JobArgs) error {
		return sentinel
	}}
	id := creationID(t)
	_, err := svc.Create(context.Background(), ws, id, "請建立摘要", .5)
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want %v", err, sentinel)
	}
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM creation_sessions WHERE id=$1", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("sessions after insert failure = %d, want 0 (rolled back)", rows)
	}
}
