package packaging

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

const tidySkillMD = "---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\n---\n\n"

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readSourceOf(t *testing.T, files map[string]string) sourceFiles {
	t.Helper()
	s := &Service{Store: memberStore{"packages/src.zip": zipOf(t, files)}}
	out, err := s.readSource(context.Background(), VersionFacts{PackageObjectKey: "packages/src.zip"})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestASkillMDPointingAtAFileThePackagerDropsBlocksThePackage(t *testing.T) {
	out := readSourceOf(t, map[string]string{
		"SKILL.md":               tidySkillMD + "Run [the helper](node_modules/helper.js).\n",
		"node_modules/helper.js": "module.exports = 1\n",
	})
	if out.blockedReason != BlockedFileRemoved {
		t.Fatalf("blocked reason = %q, want %q for a referenced file the packager drops", out.blockedReason, BlockedFileRemoved)
	}
}

func TestAnUnreferencedDroppedFileDoesNotBlockThePackage(t *testing.T) {
	out := readSourceOf(t, map[string]string{
		"SKILL.md":               tidySkillMD + "Tidy it.\n",
		"node_modules/helper.js": "module.exports = 1\n",
	})
	if out.blockedReason != "" {
		t.Fatalf("blocked reason = %q, want none when SKILL.md never points at the dropped file", out.blockedReason)
	}
}

func TestAStoredSourceThatNoLongerValidatesIsNotPackaged(t *testing.T) {
	out := readSourceOf(t, map[string]string{"SKILL.md": "no front matter at all\n"})
	if out.blockedReason != BlockedValidation || !out.validation.Blocked {
		t.Fatalf("blocked reason = %q, validation blocked = %v; want %q and true", out.blockedReason, out.validation.Blocked, BlockedValidation)
	}
}

func TestAPluginMemberWhoseVersionBelongsToAnotherSkillIsNotFound(t *testing.T) {
	skillID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherSkill := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	s := &Service{
		Store: memberStore{},
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Name: "tidy-csv", Redistribution: string(RedistributionAllowed)}, true, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{ID: versionID, SkillID: otherSkill, VersionNumber: 1, PackageObjectKey: "packages/other.zip"}, true, nil
		},
	}

	_, err := s.exportPluginMember(context.Background(), identity.Workspace{}, PluginMember{SkillID: skillID, VersionID: versionID}, &PluginPlan{})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("a member naming another skill's version: err = %v, want ErrNotFound", err)
	}
}

func TestAPackageForARecipientNeverSelectsTestCases(t *testing.T) {
	files := []exportFile{{path: "SKILL.md", data: []byte(tidySkillMD)}}
	p := &Plan{forRecipient: true, IncludeTestCases: true}

	got, err := (&Service{}).withTestCases(context.Background(), identity.Workspace{}, p, files)

	if err != nil || len(got) != 1 || len(p.Included) != 0 || len(p.Excluded) != 0 {
		t.Fatalf("recipient package: %d files, %d included, %d excluded, err %v; want the files unchanged and no test cases",
			len(got), len(p.Included), len(p.Excluded), err)
	}
}
