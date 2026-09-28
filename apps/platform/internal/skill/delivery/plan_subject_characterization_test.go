package packaging

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func planUUID(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }

func TestAPackagePlanReadsOnlyAVersionOfTheRequestedSkill(t *testing.T) {
	errRead := errors.New("owner read failed")
	skill, other := planUUID(1), planUUID(2)
	version := VersionFacts{ID: planUUID(3), SkillID: skill}
	for _, tc := range []struct {
		name        string
		skillFound  bool
		skillErr    error
		version     VersionFacts
		versionOK   bool
		versionErr  error
		wantErr     error
		wantVersion bool
	}{
		{name: "skill not found", wantErr: ErrNotFound},
		{name: "skill read fails", skillErr: errRead, wantErr: errRead},
		{name: "version not found", skillFound: true, wantErr: ErrNotFound},
		{name: "version read fails", skillFound: true, versionErr: errRead, wantErr: errRead},
		{name: "version of another skill", skillFound: true, version: VersionFacts{ID: planUUID(3), SkillID: other}, versionOK: true, wantErr: ErrNotFound},
		{name: "version of this skill", skillFound: true, version: version, versionOK: true, wantVersion: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
					return SkillFacts{ID: skill, Name: "ledger"}, tc.skillFound, tc.skillErr
				},
				ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
					return tc.version, tc.versionOK, tc.versionErr
				},
			}
			gotSkill, gotVersion, err := svc.readRequestedVersion(context.Background(), identity.Workspace{},
				PackageRequest{SkillID: skill, VersionID: version.ID})
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil) != (err == nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantVersion && (gotSkill.ID != skill || gotVersion.ID != version.ID) {
				t.Fatalf("got skill %v version %v, want the requested pair", gotSkill.ID, gotVersion.ID)
			}
			if !tc.wantVersion && (gotSkill.ID.Valid || gotVersion.ID.Valid) {
				t.Fatalf("a refused read still returned skill %v version %v", gotSkill.ID, gotVersion.ID)
			}
		})
	}
}

func TestTestCaseSlugLengthBoundaries(t *testing.T) {
	const id = "12345678-9abc-4def-8123-456789abcdef"
	for _, tc := range []struct {
		name, caseName, id, want string
	}{
		{"name at the forty-character ceiling is kept", strings.Repeat("a", 40), id, strings.Repeat("a", 40) + "-12345678"},
		{"name one past the ceiling is cut to forty", strings.Repeat("a", 41), id, strings.Repeat("a", 40) + "-12345678"},
		{"id shorter than eight characters is used whole", "ledger", "1234567", "ledger-1234567"},
		{"id of exactly eight characters is used whole", "ledger", "12345678", "ledger-12345678"},
		{"a name with nothing sluggable falls back", "——", id, "test-case-12345678"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testCaseSlug(tc.caseName, tc.id); got != tc.want {
				t.Fatalf("testCaseSlug(%q, %q) = %q, want %q", tc.caseName, tc.id, got, tc.want)
			}
		})
	}
}

func TestRetentionDaysCountsWholeDays(t *testing.T) {
	for _, tc := range []struct {
		period time.Duration
		want   int
	}{
		{48 * time.Hour, 2},
		{48*time.Hour - time.Nanosecond, 1},
		{24 * time.Hour, 1},
	} {
		if got := retentionDays(tc.period); got != tc.want {
			t.Errorf("retentionDays(%v) = %d, want %d", tc.period, got, tc.want)
		}
	}
}
