package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

type modesDiagram struct {
	ID    string `json:"id"`
	Media string `json:"media"`
	Nodes []struct {
		Label string `json:"label"`
		Key   string `json:"key"`
	} `json:"nodes"`
}

type modesReference struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	DescriptionKeys []string `json:"description_keys"`
	Reference       struct {
		Name    string   `json:"name"`
		SkillMD string   `json:"skill_md"`
		Markers []string `json:"markers"`
	} `json:"reference"`
	Holdout []holdoutCase `json:"holdout"`
}

type modesCorpus struct {
	Diagram   []modesDiagram   `json:"diagram"`
	Reference []modesReference `json:"reference"`
}

type modesRow struct {
	ID        string   `json:"id"`
	Mode      string   `json:"mode"`
	Generated bool     `json:"generated"`
	Blocked   bool     `json:"blocked"`
	Findings  []string `json:"findings,omitempty"`
	Error     string   `json:"error,omitempty"`
	Attempts  int      `json:"attempts,omitempty"`
	CostUSD   *float64 `json:"cost_usd,omitempty"`
	SkillName string   `json:"skill_name,omitempty"`

	ProvenanceRecorded bool `json:"provenance_recorded"`

	KeysFound int `json:"keys_found"`
	KeysTotal int `json:"keys_total"`

	MarkersCopied      int `json:"markers_copied,omitempty"`
	MarkersTotal       int `json:"markers_total,omitempty"`
	LongestSharedRunes int `json:"longest_shared_runes,omitempty"`
	OutputRunes        int `json:"output_runes,omitempty"`
}

func TestTheTwoNewerModesTwentyTimesEach(t *testing.T) {
	corpusPath, diagramDir, outDir := os.Getenv("GEN_MODES_CORPUS"), os.Getenv("GEN_MODES_DIAGRAMS"), os.Getenv("GEN_MODES_OUT")
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	if corpusPath == "" || diagramDir == "" || outDir == "" || base == "" {
		t.Skip("set GEN_MODES_CORPUS, GEN_MODES_DIAGRAMS, GEN_MODES_OUT and SKILLHUB_E2E_LLM_URL; this test spends money")
	}
	corpus := readModesCorpus(t, corpusPath)
	if len(corpus.Diagram) == 0 && len(corpus.Reference) == 0 {
		t.Fatal("the corpus is empty; a distribution over nothing is a zero, not a pass")
	}
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, base)
	g := modesGeneration{t: t, a: a, pool: pool, ctx: context.Background(), diagramDir: diagramDir, outDir: outDir}

	var rows []modesRow
	flush := func() {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "results.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, d := range corpus.Diagram {
		row := g.measureDiagram(d)
		rows = append(rows, row)
		flush()
		t.Logf("%s: generated=%v attempts=%d keys=%d/%d", d.ID, row.Generated, row.Attempts, row.KeysFound, row.KeysTotal)
	}

	for _, r := range corpus.Reference {
		row := g.measureReference(r)
		rows = append(rows, row)
		flush()
		t.Logf("%s: generated=%v attempts=%d keys=%d/%d markers=%d/%d shared=%d", r.ID, row.Generated, row.Attempts,
			row.KeysFound, row.KeysTotal, row.MarkersCopied, row.MarkersTotal, row.LongestSharedRunes)
	}

	if len(rows) == 0 {
		t.Fatal(errors.New("no rows"))
	}
}

func readModesCorpus(t *testing.T, path string) modesCorpus {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var corpus modesCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

type modesGeneration struct {
	t          *testing.T
	a          *api
	pool       *pgxpool.Pool
	ctx        context.Context
	diagramDir string
	outDir     string
}

func (g modesGeneration) generate(id, mode string, build func(c *client) ingest.GenerateInput) (modesRow, string) {
	t := g.t
	row := modesRow{ID: id, Mode: mode}
	c := g.a.login(t, "gen-modes-"+strings.ToLower(id))
	ws := workspaceOf(t, g.pool, c)
	res, err := g.a.versions.GenerateSkill(g.ctx, ws, build(c))
	if err != nil {
		row.Error = err.Error()
		t.Logf("%s: %v", id, err)
		return row, ""
	}
	row.Attempts, row.CostUSD = res.Attempts, res.CostUSD
	if res.Report.Blocked {
		row.Blocked = true
		row.Findings = errorFindingCodes(res.Report.Findings)
		return row, ""
	}
	row.Generated = true
	row.SkillName = res.Report.Manifest.Name
	md := readStoredSkillMD(t, g.a, g.ctx, res.Version.PackageObjectKey, id)
	if err := os.WriteFile(filepath.Join(g.outDir, id+".SKILL.md"), md, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := generationInputsOf(t, g.ctx, g.pool, res.Version.ID)
	row.ProvenanceRecorded = len(inputs) > 0
	row.OutputRunes = len([]rune(string(md)))
	return row, string(md)
}

func (g modesGeneration) measureDiagram(d modesDiagram) modesRow {
	ext, mediaType := d.Media, diagramMediaType(d.Media)
	img, err := os.ReadFile(filepath.Join(g.diagramDir, d.ID+"."+ext))
	if err != nil {
		g.t.Fatal(err)
	}
	row, out := g.generate(d.ID, "diagram", func(*client) ingest.GenerateInput {
		return ingest.GenerateInput{Diagram: &ingest.GenerateDiagram{MediaType: mediaType, Data: img}}
	})
	row.KeysTotal = len(d.Nodes)
	if out != "" {
		for _, n := range d.Nodes {
			if strings.Contains(out, n.Key) {
				row.KeysFound++
			}
		}
	}
	return row
}

func (g modesGeneration) measureReference(r modesReference) modesRow {
	row, out := g.generate(r.ID, "reference", func(c *client) ingest.GenerateInput {
		refID, _ := importFiles(g.t, g.a, g.pool, c, map[string]string{"SKILL.md": r.Reference.SkillMD})
		return ingest.GenerateInput{
			TaskDescription:   r.Description,
			ReferenceSkillIDs: []pgtype.UUID{mustUUID(g.t, refID)},
		}
	})
	row.KeysTotal, row.MarkersTotal = len(r.DescriptionKeys), len(r.Reference.Markers)
	if out != "" {
		row.KeysFound = countContained(out, r.DescriptionKeys)
		row.MarkersCopied = countContained(out, r.Reference.Markers)
		row.LongestSharedRunes = longestCommonRun([]rune(referenceSkillBody(r.Reference.SkillMD)), []rune(out))
	}
	return row
}

func diagramMediaType(ext string) string {
	mediaType := "image/png"
	if ext == "jpg" {
		mediaType = "image/jpeg"
	}
	return mediaType
}

func errorFindingCodes(findings []skillpkg.Finding) []string {
	var codes []string
	for _, f := range findings {
		if f.Severity == skillpkg.SeverityError {
			codes = append(codes, f.Code)
		}
	}
	return codes
}

func countContained(text string, needles []string) int {
	found := 0
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			found++
		}
	}
	return found
}

func referenceSkillBody(skillMD string) string {
	_, body, _ := strings.Cut(strings.TrimPrefix(skillMD, "---\n"), "\n---\n")
	return body
}

func readStoredSkillMD(t *testing.T, a *api, ctx context.Context, key, label string) []byte {
	t.Helper()
	data, err := a.packages.Get(ctx, key)
	if err != nil {
		t.Fatalf("%s: stored package unreadable: %v", label, err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	md, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	return md
}

func generationInputsOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, versionID pgtype.UUID) []byte {
	t.Helper()
	var inputs []byte
	if err := pool.QueryRow(ctx, `
			SELECT s.generation_inputs FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
			WHERE v.id = $1`, versionID).Scan(&inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func longestCommonRun(a, b []rune) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	best := 0
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
				best = max(best, cur[j])
			} else {
				cur[j] = 0
			}
		}
		prev, cur = cur, prev
	}
	return best
}
