package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type countingStore struct {
	data  []byte
	err   error
	reads int
}

func (s *countingStore) Get(context.Context, string) ([]byte, error) {
	s.reads++
	return s.data, s.err
}

func zippedSkills(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestASecondViewOfTheSameStoredSkillIsAnsweredWithoutReadingThePackageAgain(t *testing.T) {
	store := &countingStore{data: zippedSkills(t, map[string]string{
		"alpha/SKILL.md": "---\nname: alpha\ndescription: first\n---\nbody\n",
		"beta/SKILL.md":  "---\nname: beta\ndescription: second\n---\nbody\n",
	})}
	s := &Service{Store: store}
	alpha := skillpkg.StoredSkill{ObjectKey: "packages/one.zip", SourcePath: "alpha"}

	first, ok := s.scanPackage(context.Background(), alpha)
	if !ok || first.Manifest == nil || first.Manifest.Name != "alpha" {
		t.Fatalf("first scan = %+v (ok %v), want alpha's manifest", first.Manifest, ok)
	}
	again, ok := s.scanPackage(context.Background(), alpha)
	if !ok || again.Manifest == nil || again.Manifest.Name != "alpha" || store.reads != 1 {
		t.Fatalf("second scan read the package %d times in all and named %+v, want 1 read and alpha", store.reads, again.Manifest)
	}

	beta, ok := s.scanPackage(context.Background(), skillpkg.StoredSkill{ObjectKey: "packages/one.zip", SourcePath: "beta"})
	if !ok || beta.Manifest == nil || beta.Manifest.Name != "beta" {
		t.Fatalf("a sibling skill in the same package was answered with %+v, want beta's own manifest", beta.Manifest)
	}
}

func TestAPackageThatCouldNotBeReadIsTriedAgainOnTheNextView(t *testing.T) {
	store := &countingStore{err: errors.New("store unreachable")}
	s := &Service{Store: store}
	stored := skillpkg.StoredSkill{ObjectKey: "packages/two.zip", SourcePath: "gamma"}

	if _, ok := s.scanPackage(context.Background(), stored); ok {
		t.Fatal("an unreadable package was reported as scanned")
	}
	store.err, store.data = nil, zippedSkills(t, map[string]string{
		"gamma/SKILL.md": "---\nname: gamma\ndescription: third\n---\nbody\n",
		"delta/SKILL.md": "---\nname: delta\ndescription: fourth\n---\nbody\n",
	})
	report, ok := s.scanPackage(context.Background(), stored)
	if !ok || report.Manifest == nil || report.Manifest.Name != "gamma" || store.reads != 2 {
		t.Fatalf("after the store recovered: ok %v, manifest %+v, reads %d; want gamma read afresh", ok, report.Manifest, store.reads)
	}
}

func TestTheFilesViewAndTheScanShareOneReportOfAPackage(t *testing.T) {
	store := &countingStore{data: zippedSkills(t, map[string]string{
		"alpha/SKILL.md": "---\nname: alpha\ndescription: first\n---\nbody\n",
		"beta/SKILL.md":  "---\nname: beta\ndescription: second\n---\nbody\n",
	})}
	version := VersionFacts{PackageObjectKey: "packages/one.zip", SourcePath: "alpha"}
	s := &Service{Store: store, ReadLatestVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
		return version, true, nil
	}}

	if _, err := s.SkillFiles(context.Background(), SkillFacts{}); err != nil {
		t.Fatal(err)
	}
	report, ok := s.scanPackage(context.Background(), storedSkill(version))
	if !ok || report.Manifest == nil || report.Manifest.Name != "alpha" || store.reads != 1 {
		t.Fatalf("scan after the files view: ok %v, manifest %+v, reads %d; want alpha from the files view's single read", ok, report.Manifest, store.reads)
	}
}
