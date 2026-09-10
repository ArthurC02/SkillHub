package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

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

func TestImportPathsRefuseWithoutProjectionDependencies(t *testing.T) {
	ctx := context.Background()
	if _, _, err := (&Service{}).persistVersion(ctx, nil, identity.Workspace{}, registry.Skill{},
		preparedPackage{}, sourceMeta{Type: "upload"}, enrichment{}); err == nil {
		t.Error("persistVersion succeeded without the search projection write injected")
	}

	if _, _, err := (&Service{LLM: &llmclient.Client{}}).ReindexPending(ctx, 1); err == nil {
		t.Error("ReindexPending succeeded without the search projection write injected")
	}
	if _, _, err := (&Service{
		LLM: &llmclient.Client{},
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error {
			return nil
		},
	}).ReindexPending(ctx, 1); err == nil {
		t.Error("ReindexPending succeeded without the pending enrichment lister injected")
	}
}
