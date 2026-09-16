package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
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
	if _, _, err := (&Service{}).persistVersion(ctx, nil, identity.Workspace{}, &registry.SkillRoot{},
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

func TestAVersionIsNotPersistedFromASourceWithoutItsProvenance(t *testing.T) {
	s := &Service{IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error { return nil }}

	_, _, err := s.persistVersion(context.Background(), nil, identity.Workspace{}, &registry.SkillRoot{},
		preparedPackage{}, sourceMeta{Type: SourceGenerated}, enrichment{})

	if !errors.Is(err, ErrIncompleteProvenance) {
		t.Fatalf("err = %v, want ErrIncompleteProvenance", err)
	}
}

func TestASourceIsWrittenOnlyWithTheProvenanceItsTypeRequires(t *testing.T) {
	text := func(s string) *string { return &s }
	generated := func(task, model, prompt *string) sourceMeta {
		return sourceMeta{Type: SourceGenerated, TaskDescription: task, GeneratorModel: model, GeneratorPromptVersion: prompt}
	}
	for _, tc := range []struct {
		name     string
		src      sourceMeta
		complete bool
	}{
		{"a git source with its URL", sourceMeta{Type: SourceGit, URL: text("https://example.test/repo")}, true},
		{"a git source without a URL", sourceMeta{Type: SourceGit}, false},
		{"a git source with a blank URL", sourceMeta{Type: SourceGit, URL: text("  ")}, false},
		{"a git source claiming a generator", sourceMeta{Type: SourceGit, URL: text("https://example.test/repo"), GeneratorModel: text("m")}, false},
		{"a plain upload", sourceMeta{Type: SourceUpload}, true},
		{"an upload claiming a URL", sourceMeta{Type: SourceUpload, URL: text("https://example.test/repo")}, false},
		{"an upload claiming a task", sourceMeta{Type: SourceUpload, TaskDescription: text("t")}, false},
		{"a generated source with task, model and prompt", generated(text("t"), text("m"), text("p")), true},
		{"a generated source without its task", generated(nil, text("m"), text("p")), false},
		{"a generated source from a diagram alone records an empty task", generated(text(""), text("m"), text("p")), true},
		{"a generated source with a blank model", generated(text("t"), text(" "), text("p")), false},
		{"a generated source without its prompt version", generated(text("t"), text("m"), nil), false},
		{"a generated source claiming a URL", func() sourceMeta {
			src := generated(text("t"), text("m"), text("p"))
			src.URL = text("https://example.test/repo")
			return src
		}(), false},
		{"an unknown source type", sourceMeta{Type: "mirror"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.src.provenanceComplete()
			if tc.complete && err != nil {
				t.Fatalf("refused a complete source: %v", err)
			}
			if !tc.complete && !errors.Is(err, ErrIncompleteProvenance) {
				t.Fatalf("err = %v, want ErrIncompleteProvenance", err)
			}
		})
	}
}
