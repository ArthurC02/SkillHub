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
		{"without Test Lab", &Service{ReadVersionSummaries: summaries}},
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
