package apiserver_test

// GEN-005 / GEN-006 ①② — the two newer input modes, twenty of each, against a
// real gateway. report-generate-modes.md ran each mode once and said so: "三次
// 各一筆，不是分布". This is the distribution.
//
// ============================ WHAT THIS IS ==================================
// For each corpus item the real generation path runs (ingest → apps/llm →
// gateway → skillpkg.Validate → version), and the row records what the earlier
// rounds recorded: validated or blocked, attempts, cost, the provenance row. Two
// machine checks are added because the modes have a question the text mode did
// not: did the model READ the input? For a diagram, each node carries a key
// token a faithful document would contain; the row counts how many appear. For
// a reference, each reference carries three marker phrases that appear only in
// its own body; the row counts how many the output copied, and the longest
// run of runes shared between the reference body and the output.
//
// ============================ WHAT THIS IS NOT ==============================
// Not GEN-009 ③④: nothing is run in a sandbox and nobody reads the output.
// A key token present says the node was seen, not that the step was right.
//
// Usage (spends money — about US$0.004 per item):
//
//	GEN_MODES_CORPUS=<corpus.json>  {"diagram":[{id,nodes:[{label,key}],media}],"reference":[{id,description,description_keys,reference:{name,skill_md,markers}}]}
//	GEN_MODES_DIAGRAMS=<dir>        <id>.png / <id>.jpg drawn from the corpus
//	GEN_MODES_OUT=<dir>             results.json and one <id>.SKILL.md per item
//	SKILLHUB_E2E_LLM_URL            a running apps/llm pointed at a real gateway

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

type modesCorpus struct {
	Diagram []struct {
		ID    string `json:"id"`
		Media string `json:"media"`
		Nodes []struct {
			Label string `json:"label"`
			Key   string `json:"key"`
		} `json:"nodes"`
	} `json:"diagram"`
	Reference []struct {
		ID              string   `json:"id"`
		Description     string   `json:"description"`
		DescriptionKeys []string `json:"description_keys"`
		Reference       struct {
			Name    string   `json:"name"`
			SkillMD string   `json:"skill_md"`
			Markers []string `json:"markers"`
		} `json:"reference"`
	} `json:"reference"`
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
	// ProvenanceRecorded: skill_sources.generation_inputs is non-NULL (ADR-066).
	ProvenanceRecorded bool `json:"provenance_recorded"`
	// Diagram: node keys found in the output / total.
	KeysFound int `json:"keys_found"`
	KeysTotal int `json:"keys_total"`
	// Reference: marker phrases copied verbatim, and the longest shared run.
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
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus modesCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Diagram) == 0 && len(corpus.Reference) == 0 {
		t.Fatal("the corpus is empty; a distribution over nothing is a zero, not a pass")
	}
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, base)
	ctx := context.Background()

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

	// One workspace per item: a generated name that repeats across items would
	// be ErrGeneratedNameCollision, and the daily allowance is per workspace.
	generate := func(id, mode string, build func(c *client) ingest.GenerateInput) (modesRow, string) {
		row := modesRow{ID: id, Mode: mode}
		c := a.login(t, "gen-modes-"+strings.ToLower(id))
		ws := workspaceOf(t, pool, c)
		res, err := a.versions.GenerateSkill(ctx, ws, build(c))
		if err != nil {
			row.Error = err.Error()
			t.Logf("%s: %v", id, err)
			return row, ""
		}
		row.Attempts, row.CostUSD = res.Attempts, res.CostUSD
		if res.Report.Blocked {
			row.Blocked = true
			for _, f := range res.Report.Findings {
				if f.Severity == skillpkg.SeverityError {
					row.Findings = append(row.Findings, f.Code)
				}
			}
			return row, ""
		}
		row.Generated = true
		row.SkillName = res.Report.Manifest.Name
		data, err := a.packages.Get(ctx, res.Version.PackageObjectKey)
		if err != nil {
			t.Fatalf("%s: stored package unreadable: %v", id, err)
		}
		fsys, err := skillpkg.PackageFS(data)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		md, err := fs.ReadFile(fsys, "SKILL.md")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, id+".SKILL.md"), md, 0o600); err != nil {
			t.Fatal(err)
		}
		var inputs []byte
		if err := pool.QueryRow(ctx, `
			SELECT s.generation_inputs FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
			WHERE v.id = $1`, res.Version.ID).Scan(&inputs); err != nil {
			t.Fatal(err)
		}
		row.ProvenanceRecorded = len(inputs) > 0
		row.OutputRunes = len([]rune(string(md)))
		return row, string(md)
	}

	for _, d := range corpus.Diagram {
		ext, mediaType := d.Media, "image/png"
		if ext == "jpg" {
			mediaType = "image/jpeg"
		}
		img, err := os.ReadFile(filepath.Join(diagramDir, d.ID+"."+ext))
		if err != nil {
			t.Fatal(err)
		}
		row, out := generate(d.ID, "diagram", func(*client) ingest.GenerateInput {
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
		rows = append(rows, row)
		flush()
		t.Logf("%s: generated=%v attempts=%d keys=%d/%d", d.ID, row.Generated, row.Attempts, row.KeysFound, row.KeysTotal)
	}

	for _, r := range corpus.Reference {
		row, out := generate(r.ID, "reference", func(c *client) ingest.GenerateInput {
			refID, _ := importFiles(t, a, pool, c, map[string]string{"SKILL.md": r.Reference.SkillMD})
			return ingest.GenerateInput{
				TaskDescription:   r.Description,
				ReferenceSkillIDs: []pgtype.UUID{mustUUID(t, refID)},
			}
		})
		row.KeysTotal, row.MarkersTotal = len(r.DescriptionKeys), len(r.Reference.Markers)
		if out != "" {
			for _, k := range r.DescriptionKeys {
				if strings.Contains(out, k) {
					row.KeysFound++
				}
			}
			for _, m := range r.Reference.Markers {
				if strings.Contains(out, m) {
					row.MarkersCopied++
				}
			}
			_, body, _ := strings.Cut(strings.TrimPrefix(r.Reference.SkillMD, "---\n"), "\n---\n")
			row.LongestSharedRunes = longestCommonRun([]rune(body), []rune(out))
		}
		rows = append(rows, row)
		flush()
		t.Logf("%s: generated=%v attempts=%d keys=%d/%d markers=%d/%d shared=%d", r.ID, row.Generated, row.Attempts,
			row.KeysFound, row.KeysTotal, row.MarkersCopied, row.MarkersTotal, row.LongestSharedRunes)
	}

	if len(rows) == 0 {
		t.Fatal(errors.New("no rows"))
	}
}

// longestCommonRun is the length of the longest substring (in runes) a and b
// share. Two rows of DP; the inputs are a few thousand runes each.
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
