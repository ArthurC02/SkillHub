package packaging

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestAnOriginWhoseLineageCannotBeReadIsAnErrorNotAnUnavailableUpstream(t *testing.T) {
	outage := errors.New("lineage read failed")
	forked := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	for _, tc := range []struct {
		name        string
		suggestions []AppliedSuggestion
		lineageErr  error
		wantErr     error
	}{
		{name: "fork, lineage unreadable", lineageErr: outage, wantErr: outage},
		{name: "fork, upstream gone"},
		{name: "improvement, lineage unreadable", suggestions: []AppliedSuggestion{{}}, lineageErr: outage, wantErr: outage},
		{name: "improvement, upstream gone", suggestions: []AppliedSuggestion{{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				AppliedSuggestions: func(context.Context, pgtype.UUID, pgtype.UUID) ([]AppliedSuggestion, error) {
					return tc.suggestions, nil
				},
				SourceLineage: func(context.Context, pgtype.UUID) (LineageSource, error) { return LineageSource{}, nil },
				ReadPrevious: func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error) {
					return PreviousVersion{}, false, nil
				},
				ReadLineage: func(context.Context, pgtype.UUID) (LineageStep, bool, error) {
					return LineageStep{}, false, tc.lineageErr
				},
				ReadOldest: func(context.Context, pgtype.UUID) (OldestVersion, bool, error) { return OldestVersion{}, false, nil },
			}
			origin, err := svc.originOf(context.Background(), identity.Workspace{}, SkillFacts{ForkedFromVersionID: forked}, VersionFacts{})
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if origin != nil {
					t.Errorf("origin = %+v, want none when the lineage could not be read", origin)
				}
				return
			}
			assertUnavailableUpstream(t, origin)
		})
	}
}

func assertUnavailableUpstream(t *testing.T, origin any) {
	t.Helper()
	switch o := origin.(type) {
	case forkOrigin:
		if o.RootSource != unavailable || len(o.Chain) != 1 || o.Chain[0] != unavailable {
			t.Errorf("fork origin = %+v, want an unavailable upstream", o)
		}
	case improvementOrigin:
		if o.RootSource != unavailable {
			t.Errorf("improvement origin = %+v, want an unavailable root source", o)
		}
	default:
		t.Errorf("origin = %T, want a fork or improvement origin", origin)
	}
}
