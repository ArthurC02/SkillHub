package packaging

import (
	"archive/zip"
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	policy "github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

type sourceEntry struct {
	content string
	mode    fs.FileMode
}

func sourceZip(t *testing.T, entries map[string]sourceEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, e := range entries {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func exportedForkFor(t *testing.T, target string, entries map[string]sourceEntry) map[string]*zip.File {
	t.Helper()
	if purgePool == nil {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; the export reads its test cases from the database")
	}
	profiles, err := LoadProfiles(realProfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	skillID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	s := &Service{
		Profiles:  profiles,
		Retention: policy.DownloadRetention(24 * time.Hour),
		Store:     memberStore{"packages/fork.zip": sourceZip(t, entries)},
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Name: "tidy-csv-fork-2", Redistribution: string(RedistributionAllowed)}, true, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{
				ID: versionID, SkillID: skillID, VersionNumber: 1, PackageObjectKey: "packages/fork.zip",
				SourceID:  pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
				CreatedAt: pgtype.Timestamptz{Time: time.Unix(1_700_000_000, 0), Valid: true},
			}, true, nil
		},
		TestLab:            &testlab.Service{Pool: purgePool},
		AppliedSuggestions: func(context.Context, pgtype.UUID, pgtype.UUID) ([]AppliedSuggestion, error) { return nil, nil },
		SourceLineage:      func(context.Context, pgtype.UUID) (LineageSource, error) { return LineageSource{}, nil },
		ReadCompatibility: func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error) {
			return RuntimeCompatibility{}, false, nil
		},
		ReadPrevious: func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error) {
			return PreviousVersion{}, false, nil
		},
		ReadLineage: func(context.Context, pgtype.UUID) (LineageStep, bool, error) { return LineageStep{}, false, nil },
		ReadOldest:  func(context.Context, pgtype.UUID) (OldestVersion, bool, error) { return OldestVersion{}, false, nil },
		CuratedSource: func(context.Context, pgtype.UUID, pgtype.UUID) (CuratedSource, bool, error) {
			return CuratedSource{}, false, nil
		},
		ReadVersionSummaries: func(context.Context, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error) {
			return map[pgtype.UUID]VersionSummary{versionID: {LatestVersionNumber: 1}}, nil
		},
	}
	p, err := s.Plan(context.Background(), identity.Workspace{}, PackageRequest{SkillID: skillID, VersionID: versionID, Target: target})
	if err != nil || p.BlockedReason != "" {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(p.Zip), int64(len(p.Zip)))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*zip.File{}
	for _, f := range zr.File {
		byName[f.Name] = f
	}
	return byName
}

const tidyCSVSkill = "---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\n---\n\nRun scripts/tidy.sh.\n"

func TestAnExportedForkIsFiledUnderTheNameItsFrontmatterDeclares(t *testing.T) {
	files := exportedForkFor(t, "claude-code", map[string]sourceEntry{
		"SKILL.md": {content: tidyCSVSkill}, "scripts/tidy.sh": {content: "#!/bin/sh\n", mode: 0o755},
	})
	for _, want := range []string{"tidy-csv/SKILL.md", "tidy-csv/scripts/tidy.sh", "tidy-csv/" + InstallFile} {
		if files[want] == nil {
			t.Errorf("the export has no %s; names: %v", want, keysOf(files))
		}
	}
	for name := range files {
		if strings.HasPrefix(name, "tidy-csv-fork-2/") {
			t.Errorf("%s is filed under the record name, not the frontmatter name", name)
		}
	}
}

func TestAnExportedScriptKeepsItsExecutableBitAndOtherFilesGainNone(t *testing.T) {
	files := exportedForkFor(t, "standard", map[string]sourceEntry{
		"SKILL.md":           {content: tidyCSVSkill},
		"scripts/tidy.sh":    {content: "#!/bin/sh\n", mode: 0o755},
		"scripts/helper.py":  {content: "x = 1\n", mode: 0o644},
		"references/rows.md": {content: "rows\n"},
	})
	if f := files["scripts/tidy.sh"]; f == nil || f.Mode().Perm() != 0o755 {
		t.Fatalf("scripts/tidy.sh = %v, want mode 0755", modeOf(f))
	}
	for _, name := range []string{"scripts/helper.py", "references/rows.md", "SKILL.md"} {
		if f := files[name]; f == nil || f.Mode().Perm()&0o111 != 0 {
			t.Errorf("%s = %v, want a file without an executable bit", name, modeOf(f))
		}
	}
}

func keysOf(m map[string]*zip.File) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func modeOf(f *zip.File) any {
	if f == nil {
		return "missing"
	}
	return f.Mode()
}
