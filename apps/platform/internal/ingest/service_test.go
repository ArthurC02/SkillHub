package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// The PackageFS/PackageRoot tests that used to live here moved to
// internal/skillpkg/archive_test.go with the functions themselves (DDD-006,
// 2026-08-20). What is left is the zip fixture the enrichment tests build on.

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const skillMD = "---\nname: pdf-tools\ndescription: Work with PDFs.\nlicense: MIT\n---\n# PDF\n"

// A service assembled without catalog's projection write must refuse before it
// writes anything, not import a version nobody can search for (INGEST-009,
// ADR-034). Neither call has a pool or a transaction, so anything that got past
// the check would panic rather than return — which is what makes this a test of
// the ordering and not only of the message.
func TestImportPathsRefuseWithoutTheProjectionWrite(t *testing.T) {
	ctx := context.Background()
	if _, _, err := (&Service{}).persistVersion(ctx, nil, gen.Workspace{}, gen.Skill{},
		preparedPackage{}, sourceMeta{Type: "upload"}, enrichment{}); err == nil {
		t.Error("persistVersion succeeded without the search projection write injected")
	}
	// LLM set so the backfill's own precondition passes and the projection check
	// is the one being exercised.
	if _, _, err := (&Service{LLM: &llmclient.Client{}}).ReindexPending(ctx, 1); err == nil {
		t.Error("ReindexPending succeeded without the search projection write injected")
	}
}
