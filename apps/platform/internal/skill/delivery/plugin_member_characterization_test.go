package packaging

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

type memberStore map[string][]byte

func (m memberStore) Get(_ context.Context, key string) ([]byte, error) {
	if b, ok := m[key]; ok {
		return b, nil
	}
	return nil, errors.New("missing " + key)
}
func (m memberStore) GetIfPresent(_ context.Context, key string) ([]byte, bool, error) {
	b, ok := m[key]
	return b, ok, nil
}
func (m memberStore) Put(context.Context, string, []byte) error    { return nil }
func (m memberStore) Remove(context.Context, string) error         { return nil }
func (m memberStore) Exists(context.Context, string) (bool, error) { return false, nil }

func TestAPluginMemberIsFiledUnderTheNameItsOwnManifestDeclares(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\n---\n\nTidy it.\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	skillID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	s := &Service{
		Store: memberStore{"packages/member.zip": buf.Bytes()},
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Name: "tidy-csv-fork-2", Redistribution: string(RedistributionAllowed)}, true, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{ID: versionID, SkillID: skillID, VersionNumber: 1, PackageObjectKey: "packages/member.zip"}, true, nil
		},
	}
	s.TestLab = &testlab.Service{}
	s.AppliedSuggestions = func(context.Context, pgtype.UUID, pgtype.UUID) ([]AppliedSuggestion, error) { return nil, nil }
	s.SourceLineage = func(context.Context, pgtype.UUID) (LineageSource, error) { return LineageSource{}, nil }
	s.ReadCompatibility = func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error) {
		return RuntimeCompatibility{}, false, nil
	}
	s.ReadPrevious = func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error) {
		return PreviousVersion{}, false, nil
	}
	s.ReadLineage = func(context.Context, pgtype.UUID) (LineageStep, bool, error) { return LineageStep{}, false, nil }
	s.ReadOldest = func(context.Context, pgtype.UUID) (OldestVersion, bool, error) { return OldestVersion{}, false, nil }
	s.CuratedSource = func(context.Context, pgtype.UUID) (CuratedSource, bool, error) { return CuratedSource{}, false, nil }
	s.ReadVersionSummaries = func(context.Context, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error) { return nil, nil }

	p, err := s.PlanPlugin(context.Background(), identity.Workspace{}, PluginSpec{
		Name: "csv-kit", Version: "1.0.0", Members: []PluginMember{{SkillID: skillID, VersionID: versionID}},
	})

	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(p.Zip), int64(len(p.Zip)))
	if err != nil {
		t.Fatal(err)
	}
	var skillFiles int
	for _, f := range zr.File {
		if f.Name == pluginManifestFile {
			continue
		}
		skillFiles++
		if !strings.HasPrefix(f.Name, pluginSkillsDir+"tidy-csv/") {
			t.Errorf("file %q is not filed under skills/tidy-csv/", f.Name)
		}
	}
	if skillFiles == 0 || len(p.Members) != 1 || p.Members[0].Name != "tidy-csv" {
		t.Errorf("skill files=%d members=%+v, want the member named tidy-csv", skillFiles, p.Members)
	}
}
