package run

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type packageShelf struct {
	data []byte
	err  error
}

func (s packageShelf) Get(context.Context, string) ([]byte, error) { return s.data, s.err }
func (packageShelf) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (packageShelf) PresignPut(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (packageShelf) Remove(context.Context, string) error { return nil }

func cleanSkillArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("---\nname: tidy-rows\ndescription: Removes duplicate rows from a sheet.\nlicense: MIT\n---\n\nRemove duplicate rows.\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAPackageThatWasReadAndScannedCleanIsNotRefused(t *testing.T) {
	s := &Service{Store: packageShelf{data: cleanSkillArchive(t)}}
	if err := requireScanNotBlocking(s.packageReport(context.Background(), skillpkg.StoredSkill{ObjectKey: "skills/tidy.zip"})); err != nil {
		t.Fatalf("a readable, clean package was refused: %v", err)
	}
}

func TestAPackageThatCouldNotBeReadIsRefusedAsUnscanned(t *testing.T) {
	s := &Service{Store: packageShelf{err: errors.New("object missing")}}
	err := requireScanNotBlocking(s.packageReport(context.Background(), skillpkg.StoredSkill{ObjectKey: "skills/tidy.zip"}))
	if !errors.Is(err, ErrScanBlocked) {
		t.Fatalf("err = %v, want the scan sentinel for an unreadable package", err)
	}
}

func TestTheScriptSummaryOfAnUnscannedPackageSaysItIsUnavailable(t *testing.T) {
	got := scriptSummaryOf(packageScan{})
	if got.Status != "unavailable" || got.Findings == nil || len(got.Findings) != 0 {
		t.Errorf("summary = %+v, want unavailable with an empty findings list", got)
	}
	if clean := scriptSummaryOf(packageScan{scanned: true}); clean.Status != "none" {
		t.Errorf("a scanned package with no scripts reads %q, want none", clean.Status)
	}
}
