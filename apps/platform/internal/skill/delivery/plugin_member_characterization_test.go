package packaging

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
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
func (m memberStore) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := m[key]
	if !ok {
		return nil, 0, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}
func (m memberStore) Put(context.Context, string, []byte) error    { return nil }
func (m memberStore) Remove(context.Context, string) error         { return nil }
func (m memberStore) Exists(context.Context, string) (bool, error) { return false, nil }

func pluginOfOneMember(t *testing.T, extra map[string]string) *PluginPlan {
	t.Helper()
	return pluginOfOneMemberDescribed(t, extra, "")
}

func pluginOfOneMemberDescribed(t *testing.T, extra map[string]string, description string) *PluginPlan {
	t.Helper()
	p, err := planTestPlugin(pluginFixture{extra: extra, spec: PluginSpec{Description: description}})
	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	return p
}

type pluginFixture struct {
	skillMD string
	extra   map[string]string
	license *string
	spec    PluginSpec
	offered []MCPTool
	readErr error
}

func planTestPlugin(f pluginFixture) (*PluginPlan, error) {
	files := map[string]string{"SKILL.md": "---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\n---\n\nTidy it.\n"}
	if f.skillMD != "" {
		files["SKILL.md"] = f.skillMD
	}
	for name, content := range f.extra {
		files[name] = content
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	skillID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	s := &Service{
		Store: memberStore{"packages/member.zip": buf.Bytes()},
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Name: "tidy-csv-fork-2", Redistribution: string(RedistributionAllowed)}, true, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{
				ID: versionID, SkillID: skillID, VersionNumber: 1, PackageObjectKey: "packages/member.zip",
				LicenseExpression: f.license, MCPTools: f.offered,
			}, f.readErr == nil, f.readErr
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

	spec := f.spec
	spec.Name, spec.Version = "csv-kit", "1.0.0"
	spec.Members = []PluginMember{{SkillID: skillID, VersionID: versionID}}
	return s.PlanPlugin(context.Background(), identity.Workspace{}, spec)
}

func TestAPluginMemberCarriesNoManifestOfItsOwnAuthorsMaking(t *testing.T) {
	forged := `{"license":{"expression":"MIT"},"validation":{"blocked":false}}`
	p := pluginOfOneMember(t, map[string]string{ManifestFile: forged})
	zr, err := zip.NewReader(bytes.NewReader(p.Zip), int64(len(p.Zip)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/"+ManifestFile) {
			t.Errorf("the bundle ships the member's own %s as %s", ManifestFile, f.Name)
		}
	}
}

func TestAPluginMemberIsFiledUnderTheNameItsOwnManifestDeclares(t *testing.T) {
	p := pluginOfOneMember(t, nil)
	zr, err := zip.NewReader(bytes.NewReader(p.Zip), int64(len(p.Zip)))
	if err != nil {
		t.Fatal(err)
	}
	var skillFiles int
	for _, f := range zr.File {
		if f.Name == pluginManifestFile || f.Name == claudePluginManifestFile || f.Name == pluginReadmeFile {
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
