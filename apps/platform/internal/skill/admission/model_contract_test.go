package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

type fakeModel struct {
	embeddings *Embeddings
	enrichment *SkillEnrichment
	draft      *GeneratedDraft
	asked      GenerateRequest
}

func (f *fakeModel) Embed(context.Context, []string) (*Embeddings, error) { return f.embeddings, nil }

func (f *fakeModel) EnrichSkill(context.Context, EnrichRequest) (*SkillEnrichment, error) {
	return f.enrichment, nil
}

func (f *fakeModel) GenerateSkill(_ context.Context, req GenerateRequest) (*GeneratedDraft, error) {
	f.asked = req
	return f.draft, nil
}

func modelOverAWire(t *testing.T, path string, capture any, status int, reply any) Model {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("adapter called %s, want %s", r.URL.Path, path)
		}
		if capture != nil {
			if err := json.NewDecoder(r.Body).Decode(capture); err != nil {
				t.Errorf("decoding the request the adapter sent: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func aGeneratedDraft() *GeneratedDraft {
	return &GeneratedDraft{
		Skill: GeneratedSkill{
			Name: "invoice-reader", Description: "reads invoices", Body: "# steps\n",
			Files: []GeneratedFile{{Path: "scripts/run.py", Content: "print(1)"}},
		},
		Model: "a-model", PromptVersion: "generate@1",
	}
}

func TestEveryModelGeneratesInTheDomainsOwnWords(t *testing.T) {
	want := aGeneratedDraft()
	overTheWire := modelOverAWire(t, "/v1/generate-skill", nil, http.StatusOK,
		llmclient.GenerateSkillResponse{
			Skill: llmclient.GeneratedSkill{
				Name: "invoice-reader", Description: "reads invoices", Body: "# steps\n",
				Files: []llmclient.GeneratedFile{{Path: "scripts/run.py", Content: "print(1)"}},
			},
			Model: "a-model", PromptVersion: "generate@1",
		})

	for name, model := range map[string]Model{"fake": &fakeModel{draft: want}, "http adapter": overTheWire} {
		t.Run(name, func(t *testing.T) {
			got, err := model.GenerateSkill(context.Background(), GenerateRequest{TaskDescription: "read invoices"})
			if err != nil {
				t.Fatalf("generating the skill: %v", err)
			}
			if got.Model != want.Model || got.PromptVersion != want.PromptVersion {
				t.Errorf("draft = %+v, want %+v", got, want)
			}
			if got.Skill.Name != want.Skill.Name || got.Skill.Description != want.Skill.Description ||
				got.Skill.Body != want.Skill.Body {
				t.Errorf("skill = %+v, want %+v", got.Skill, want.Skill)
			}
			if len(got.Skill.Files) != 1 || got.Skill.Files[0] != want.Skill.Files[0] {
				t.Errorf("files = %+v, want %+v", got.Skill.Files, want.Skill.Files)
			}
		})
	}
}

func TestTheAdapterEncodesTheDiagramTheWireExpects(t *testing.T) {
	var sent llmclient.GenerateSkillRequest
	model := modelOverAWire(t, "/v1/generate-skill", &sent, http.StatusOK, llmclient.GenerateSkillResponse{})

	_, err := model.GenerateSkill(context.Background(), GenerateRequest{
		TaskDescription: "read invoices",
		Diagram:         &GenerateDiagram{MediaType: "image/png", Data: []byte{0x01, 0x02}},
		References:      []ReferenceSkill{{Name: "prior", SkillMD: "# prior"}},
	})
	if err != nil {
		t.Fatalf("generating the skill: %v", err)
	}

	if sent.TaskDescription != "read invoices" {
		t.Errorf("task on the wire = %q, want the domain's description", sent.TaskDescription)
	}
	if sent.Diagram == nil || sent.Diagram.MediaType != "image/png" || sent.Diagram.Data != "AQI=" {
		t.Errorf("diagram on the wire = %+v, want the bytes base64-encoded; the domain hands over raw bytes",
			sent.Diagram)
	}
	if len(sent.References) != 1 || sent.References[0].SkillMD != "# prior" {
		t.Errorf("references on the wire = %+v, want the one reference skill", sent.References)
	}
}

func TestAGenerationStoppedAtTheCeilingArrivesAsTheDomainsOwnError(t *testing.T) {
	model := modelOverAWire(t, "/v1/generate-skill", nil, http.StatusBadGateway, map[string]string{
		"detail": "generate model output was truncated at the token ceiling",
	})

	_, err := model.GenerateSkill(context.Background(), GenerateRequest{TaskDescription: "read invoices"})
	if !errors.Is(err, ErrGenerationTruncated) {
		t.Errorf("error = %v, want the domain's truncation error; the caller decides what to retry "+
			"and must not read the gateway's wording to find out", err)
	}
}

func TestEveryModelEnrichesInTheDomainsOwnWords(t *testing.T) {
	want := &SkillEnrichment{
		Summary:      "reads invoices",
		TaskExamples: []TaskExample{{ZhHant: "讀發票", En: "read an invoice"}},
		Tags:         SkillTags{Inputs: []string{"pdf"}, Outputs: []string{"csv"}},
		Limitations:  []string{"scanned pages only"},
		Model:        "a-model", PromptVersion: "enrich@1",
		Checks: []EnrichCheck{{Rule: "no-brand", Field: "summary", Severity: "warning"}},
	}
	var sent llmclient.EnrichSkillRequest
	overTheWire := modelOverAWire(t, "/v1/enrich-skill", &sent, http.StatusOK,
		llmclient.EnrichSkillResponse{
			Summary:      "reads invoices",
			TaskExamples: []llmclient.TaskExample{{ZhHant: "讀發票", En: "read an invoice"}},
			Tags:         llmclient.SkillTags{Inputs: []string{"pdf"}, Outputs: []string{"csv"}},
			Limitations:  []string{"scanned pages only"},
			Model:        "a-model", PromptVersion: "enrich@1",
			Checks: []llmclient.EnrichCheck{{Rule: "no-brand", Field: "summary", Severity: "warning"}},
		})

	for name, model := range map[string]Model{"fake": &fakeModel{enrichment: want}, "http adapter": overTheWire} {
		t.Run(name, func(t *testing.T) {
			got, err := model.EnrichSkill(context.Background(), EnrichRequest{
				SkillName: "invoice-reader", SkillMD: "# skill", FileTree: []string{"SKILL.md"},
			})
			if err != nil {
				t.Fatalf("enriching the skill: %v", err)
			}
			if got.Summary != want.Summary || got.Model != want.Model || got.PromptVersion != want.PromptVersion {
				t.Errorf("enrichment = %+v, want %+v", got, want)
			}
			if len(got.TaskExamples) != 1 || got.TaskExamples[0] != want.TaskExamples[0] {
				t.Errorf("task examples = %+v, want %+v", got.TaskExamples, want.TaskExamples)
			}
			if len(got.Tags.Inputs) != 1 || got.Tags.Inputs[0] != "pdf" {
				t.Errorf("tags = %+v, want the inputs the model named", got.Tags)
			}
			if len(got.Checks) != 1 || got.Checks[0] != want.Checks[0] {
				t.Errorf("checks = %+v, want %+v", got.Checks, want.Checks)
			}
		})
	}

	if sent.SkillName != "invoice-reader" || sent.SkillMD != "# skill" || len(sent.FileTree) != 1 {
		t.Errorf("request on the wire = %+v, want the name, the markdown and the file tree", sent)
	}
}

func TestEveryModelEmbedsInTheDomainsOwnWords(t *testing.T) {
	want := &Embeddings{Vectors: [][]float32{{0.5, 0.25}}, Model: "a-model", Dimensions: 2}
	overTheWire := modelOverAWire(t, "/embed", nil, http.StatusOK, llmclient.EmbedResponse{
		Embeddings: [][]float32{{0.5, 0.25}}, Model: "a-model", Dimensions: 2,
	})

	for name, model := range map[string]Model{"fake": &fakeModel{embeddings: want}, "http adapter": overTheWire} {
		t.Run(name, func(t *testing.T) {
			got, err := model.Embed(context.Background(), []string{"read invoices"})
			if err != nil {
				t.Fatalf("embedding: %v", err)
			}
			if got.Model != want.Model || got.Dimensions != want.Dimensions {
				t.Errorf("embeddings = %+v, want %+v", got, want)
			}
			if len(got.Vectors) != 1 || len(got.Vectors[0]) != 2 || got.Vectors[0][0] != 0.5 {
				t.Errorf("vectors = %+v, want the one vector the model returned", got.Vectors)
			}
		})
	}
}

func TestOnlyAGatewayPricedCallCarriesACostIntoIngest(t *testing.T) {
	cost := 0.0042
	for _, tc := range []struct {
		name         string
		source       llmclient.CostSource
		wantReported bool
	}{
		{"priced by the gateway", llmclient.CostSourceGateway, true},
		{"priced by something else", "estimated", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := modelOverAWire(t, "/v1/generate-skill", nil, http.StatusOK,
				llmclient.GenerateSkillResponse{
					Usage: &llmclient.GatewayUsage{PromptTokens: 10, CostUSD: &cost, CostSource: tc.source},
				})
			got, err := model.GenerateSkill(context.Background(), GenerateRequest{TaskDescription: "x"})
			if err != nil {
				t.Fatalf("generating the skill: %v", err)
			}
			if got.Usage == nil || got.Usage.PromptTokens != 10 {
				t.Fatalf("usage = %+v, want the tokens the gateway counted", got.Usage)
			}
			if (got.Usage.ReportedCostUSD() != nil) != tc.wantReported {
				t.Errorf("reported cost = %v, want reported=%v", got.Usage.ReportedCostUSD(), tc.wantReported)
			}
		})
	}
}
