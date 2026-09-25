package apiserver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreationSaveReleasesItsConnectionBeforeDuplicateSearch(t *testing.T) {
	for _, scenario := range []string{"materialize", "hold", "cancel_before_materialize", "cancel_before_hold"} {
		t.Run(scenario, func(t *testing.T) {
			a, worker, _ := creationFixture(t)
			c := a.login(t, "creation-single-"+scenario)
			v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "建立摘要", "budget_credits": 650}, 200)
			v = creationStep(t, worker, v)
			v = creationAct(t, c, v, "confirm_brief")
			v = creationStep(t, worker, v)
			cfg := testPool.Config()
			cfg.MaxConns = 1
			cfg.MinConns = 0
			pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			svc := *a.app.CreationSvc
			svc.Pool = pool
			ws := identity.Workspace{ID: mustUUID(t, c.workspaceID)}
			id := mustUUID(t, v.ID)
			command := creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: v.Snapshot.Draft.ContentHash}
			cancelCommand := creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"}
			checked := false
			svc.DuplicateCheck = func(ctx context.Context, _ identity.Workspace, _ string) ([]creation.Reference, float64, error) {
				if err := pool.Ping(ctx); err != nil {
					t.Errorf("duplicate search could not acquire the only connection: %v", err)
					return nil, 0, err
				}
				checked = true
				if scenario == "cancel_before_materialize" || scenario == "cancel_before_hold" {
					if _, _, err := svc.Act(ctx, ws, id, cancelCommand); err != nil {
						t.Errorf("concurrent cancellation: %v", err)
						return nil, 0, err
					}
				}
				if scenario == "hold" || scenario == "cancel_before_hold" {
					return []creation.Reference{{SkillID: "duplicate", Name: "other-summary"}}, 0, nil
				}
				return nil, 0, nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			got, _, err := svc.Act(ctx, ws, id, command)
			if !checked {
				t.Fatal("duplicate search did not finish")
			}
			if scenario == "cancel_before_materialize" || scenario == "cancel_before_hold" {
				if !errors.Is(err, creation.ErrConflict) {
					t.Fatalf("stale save error = %v, want conflict", err)
				}
				current, err := svc.Get(ctx, ws, id)
				if err != nil || current.State != "cancelled" || current.Snapshot.Candidate != nil {
					t.Fatalf("cancellation overwritten: state=%s error=%v", current.State, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "hold" {
				if got.State != "waiting_confirmation" || got.Snapshot.PendingAction != "confirm_duplicate" || got.Snapshot.Candidate != nil {
					t.Fatalf("duplicate not held: %+v", got)
				}
			} else if got.State != "candidate_ready" || got.Snapshot.Candidate == nil {
				t.Fatalf("candidate not materialized: %+v", got)
			}
		})
	}
}

func TestCreationReadsReleaseTheirConnectionAndRespectCancellation(t *testing.T) {
	for _, kind := range []string{"select_references", "confirm_references", "attach_run"} {
		for _, cancelDuringRead := range []bool{false, true} {
			name := kind
			if cancelDuringRead {
				name += "_cancelled"
			}
			t.Run(name, func(t *testing.T) {
				shared, ws, svc := newActFixture(t)
				v, id := newActSession(t, svc, ws)
				setCreationSnapshotField(t, shared, id, "pending_action", "confirm_references")
				setCreationSnapshotField(t, shared, id, "references", []creation.Reference{{SkillID: "ref", VersionID: "version"}})
				setCreationSnapshotField(t, shared, id, "candidate", creation.Candidate{SkillID: "skill", VersionID: "version"})
				cfg := shared.Config()
				cfg.MaxConns, cfg.MinConns = 1, 0
				pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				svc.Pool = pool
				command := creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: kind, ReferenceSkillIDs: []string{"ref"}, RunID: "run"}
				cancelCommand := creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"}
				read := func(ctx context.Context) error {
					if err := pool.Ping(ctx); err != nil {
						return err
					}
					if cancelDuringRead {
						_, _, err := svc.Act(ctx, ws, id, cancelCommand)
						return err
					}
					return nil
				}
				svc.ResolveReference = func(ctx context.Context, _ identity.Workspace, _, _ string) (creation.Reference, creation.ReferenceSkill, error) {
					return creation.Reference{SkillID: "ref", VersionID: "version", Available: true}, creation.ReferenceSkill{}, read(ctx)
				}
				svc.ReadRun = func(ctx context.Context, _ identity.Workspace, _ string, _ creation.Candidate) (string, error) {
					return `{"execution_status":"succeeded"}`, read(ctx)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				got, _, err := svc.Act(ctx, ws, id, command)
				if cancelDuringRead {
					if !errors.Is(err, creation.ErrConflict) {
						t.Fatalf("stale read error = %v, want conflict", err)
					}
					current, err := svc.Get(ctx, ws, id)
					if err != nil || current.State != "cancelled" {
						t.Fatalf("cancellation overwritten: state=%s error=%v", current.State, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "select_references":
					if got.State != "waiting_confirmation" || len(got.Snapshot.References) != 1 || got.Snapshot.References[0].SkillID != "ref" {
						t.Fatalf("reference selection = %+v", got)
					}
				case "confirm_references":
					if got.State != "queued" || !got.Snapshot.References[0].Confirmed {
						t.Fatalf("reference confirmation = %+v", got)
					}
				case "attach_run":
					if got.State != "queued" || got.Snapshot.Candidate.RunID != "run" {
						t.Fatalf("run attachment = %+v", got)
					}
				}
			})
		}
	}
}
