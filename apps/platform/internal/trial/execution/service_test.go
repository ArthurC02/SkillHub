package run

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

type registryReaderFuncs struct {
	skill          func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	version        func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	versionSummary func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error)
	contentSource  func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error)
}

func (f registryReaderFuncs) Skill(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error) {
	if f.skill == nil {
		return SkillFacts{}, false, nil
	}
	return f.skill(ctx, workspaceID, skillID)
}

func (f registryReaderFuncs) Version(ctx context.Context, workspaceID, versionID pgtype.UUID) (VersionFacts, bool, error) {
	if f.version == nil {
		return VersionFacts{}, false, nil
	}
	return f.version(ctx, workspaceID, versionID)
}

func (f registryReaderFuncs) VersionSummaries(ctx context.Context, workspaceID pgtype.UUID, versionIDs []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error) {
	if f.versionSummary == nil {
		return nil, nil
	}
	return f.versionSummary(ctx, workspaceID, versionIDs)
}

func (f registryReaderFuncs) ContentSource(ctx context.Context, workspaceID, versionID pgtype.UUID) (ContentSource, bool, error) {
	if f.contentSource == nil {
		return ContentSource{}, false, nil
	}
	return f.contentSource(ctx, workspaceID, versionID)
}

func TestARunsDeadlineIsJudgedAgainstTheClockTheServiceReads(t *testing.T) {
	dispatched := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	policy, err := json.Marshal(policySnapshot{ResourceLimits: ResourceLimits{WallClockHardSeconds: 120}})
	if err != nil {
		t.Fatal(err)
	}
	clock := clockFor(
		gen.Run{CreatedAt: pgtype.Timestamptz{Time: dispatched, Valid: true}, PolicySnapshot: policy},
		[]gen.RunAttempt{dispatchedAttempt(dispatched)},
	)
	deadline := dispatched.Add(2 * time.Minute)

	for _, tc := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{"at the deadline", deadline, false},
		{"a second past it", deadline.Add(time.Second), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &driver{svc: &Service{Now: func() time.Time { return tc.now }}, clock: clock}
			if got := d.expired(); got != tc.want {
				t.Errorf("expired at %s = %v, want %v (deadline %s)", tc.now, got, tc.want, deadline)
			}
		})
	}
}

func TestAServiceWithoutAnInjectedClockReadsTheRealOneInUTC(t *testing.T) {
	before := time.Now().UTC()
	now := (&Service{}).now()

	if now.Before(before) || now.Sub(before) > time.Minute {
		t.Errorf("now = %s, want the real time around %s", now, before)
	}
	if now.Location() != time.UTC {
		t.Errorf("now is in %s, want UTC: deadlines are compared against stored UTC timestamps", now.Location())
	}
}

func TestAProvidersSilenceIsMeasuredAgainstTheClockTheServiceReads(t *testing.T) {
	since := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	d := &driver{svc: &Service{Now: func() time.Time { return since.Add(ProviderLostAfter) }}}

	if got := d.providerSilentFor(since); got != ProviderLostAfter {
		t.Errorf("provider silence = %s, want %s", got, ProviderLostAfter)
	}
}

func TestRequireTestLabDoesNotInspectOwnerInternals(t *testing.T) {
	if err := (&Service{TestLab: &testlab.Service{}}).requireTestLab(); err != nil {
		t.Fatalf("requireTestLab rejected an injected owner service: %v", err)
	}
}

func TestDeleteArtifactRefusesWithoutOwnerCounter(t *testing.T) {
	ws := identity.Workspace{}
	if err := (&Service{}).DeleteArtifact(context.Background(), ws, ws.ID, ws.ID); err == nil {
		t.Error("DeleteArtifact succeeded without packaging's artifact reference counter")
	}
}

func TestMaskingActivityRefusesWithoutTraceService(t *testing.T) {
	if _, err := (&Service{}).maskingActivity(t.Context(), time.Now(), time.Now()); err == nil {
		t.Error("masking activity succeeded without Trace owner service")
	}
}

func TestPermissionSummaryRefusesWithoutRegistryRead(t *testing.T) {
	if _, err := (&Service{}).PermissionSummaryFor(t.Context(), gen.Workspace{}.ID, gen.Workspace{}.ID, gen.Workspace{}.ID, gen.Workspace{}.ID); err == nil {
		t.Error("PermissionSummaryFor succeeded without Registry's version reader")
	}
}

func TestRunHistoryAndLinkageRefuseWithoutTheirOwnerReaders(t *testing.T) {
	id := gen.Workspace{}.ID
	summaries := func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error) {
		return nil, nil
	}
	for _, tc := range []struct {
		name string
		svc  *Service
	}{
		{"without Registry's version summaries", &Service{TestLab: &testlab.Service{}}},
		{"without Test Lab", &Service{Registry: registryReaderFuncs{versionSummary: summaries}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.svc.List(t.Context(), id, id, 10, 0); err == nil {
				t.Error("List succeeded")
			}
			if _, err := tc.svc.Linkage(t.Context(), id, id); err == nil {
				t.Error("Linkage succeeded")
			}
		})
	}
}

func TestRunSummaryKeepsTheRunListConceptSeparateFromTheDatabaseRow(t *testing.T) {
	created := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	started := created.Add(time.Minute)
	finished := started.Add(time.Minute)
	reason := "provider recovered"
	failure := ""
	versionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	testCaseID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	for _, tc := range []struct {
		name     string
		row      gen.ListWorkspaceRunsRow
		started  bool
		finished bool
	}{
		{
			name: "a completed run",
			row: gen.ListWorkspaceRunsRow{
				Status: gen.RunStatusSucceeded, StatusReason: &reason, FailureClass: &failure,
				CleanupStatus: gen.RunCleanupStatusCleaned, SkillVersionID: versionID,
				CreatedAt:  pgtype.Timestamptz{Time: created, Valid: true},
				StartedAt:  pgtype.Timestamptz{Time: started, Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: finished, Valid: true},
			},
			started: true, finished: true,
		},
		{
			name: "a queued run",
			row: gen.ListWorkspaceRunsRow{
				Status: gen.RunStatusQueued, CleanupStatus: gen.RunCleanupStatusPending,
				SkillVersionID: versionID, CreatedAt: pgtype.Timestamptz{Time: created, Valid: true},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runSummary(tc.row, VersionSummary{SkillID: versionID, SkillName: "Skill"}, testCaseID)
			if got.SkillVersionID != versionID || got.TestCaseID != testCaseID {
				t.Fatalf("summary links = (%v, %v), want (%v, %v)", got.SkillVersionID, got.TestCaseID, versionID, testCaseID)
			}
			if got.CreatedAt == nil || !got.CreatedAt.Equal(created) {
				t.Fatalf("created at = %v, want %v", got.CreatedAt, created)
			}
			if (got.StartedAt != nil) != tc.started || (got.FinishedAt != nil) != tc.finished {
				t.Fatalf("lifecycle timestamps = (%v, %v), want (%v, %v)", got.StartedAt, got.FinishedAt, tc.started, tc.finished)
			}
		})
	}
}

func TestArtifactKeepsStorageMaintenanceDetailsOutOfTheReadModel(t *testing.T) {
	created := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	row := gen.Artifact{
		ID:          pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		FileName:    "result.json",
		ContentType: "application/json",
		SizeBytes:   42,
		ContentHash: "hash",
		ObjectKey:   "internal-only",
		CreatedAt:   pgtype.Timestamptz{Time: created, Valid: true},
		PurgedAt:    pgtype.Timestamptz{Valid: true},
	}

	got := artifact(row)
	if got.FileName != row.FileName || got.ContentHash != row.ContentHash || got.CreatedAt == nil || !got.CreatedAt.Equal(created) {
		t.Fatalf("artifact = %#v, want its downloadable details", got)
	}
	if !got.Purged || got.ExpiresAt != nil {
		t.Fatalf("artifact lifetime = (purged %v, expires %v), want (true, nil)", got.Purged, got.ExpiresAt)
	}
}

func TestRunViewKeepsTheUseCaseResponseSeparateFromTheAggregateRow(t *testing.T) {
	created := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	cancelled := created.Add(time.Minute)
	reason := "user requested cancellation"
	row := gen.Run{
		ID:                 pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		Status:             gen.RunStatusRunning,
		StatusReason:       &reason,
		SkillVersionID:     pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		TestCaseSnapshotID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
		Provider:           "sandbox",
		CleanupStatus:      gen.RunCleanupStatusPending,
		CreatedAt:          pgtype.Timestamptz{Time: created, Valid: true},
		CancelRequestedAt:  pgtype.Timestamptz{Time: cancelled, Valid: true},
		ArtifactsTruncated: true,
	}

	got := runView(row)
	if got.ID != row.ID || got.Status != string(row.Status) || got.SkillVersionID != row.SkillVersionID || got.TestCaseSnapshotID != row.TestCaseSnapshotID {
		t.Fatalf("view identity and state = %#v, want run fields", got)
	}
	if got.CancelRequestedAt == nil || !got.CancelRequestedAt.Equal(cancelled) || !got.ArtifactsTruncated {
		t.Fatalf("view lifecycle = %#v, want cancellation and truncation", got)
	}
}

func TestStatusTransitionUsesAnEmptyFromStatusForTheFirstRecordedState(t *testing.T) {
	occurred := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	if got := derefStatus(nil); got != "" {
		t.Fatalf("first transition from status = %q, want empty", got)
	}
	queued := gen.RunStatusQueued
	if got := derefStatus(&queued); got != string(queued) {
		t.Fatalf("transition from status = %q, want %q", got, queued)
	}
	if got := timePointer(pgtype.Timestamptz{Time: occurred, Valid: true}); got == nil || !got.Equal(occurred) {
		t.Fatalf("transition occurred at = %v, want %v", got, occurred)
	}
}
