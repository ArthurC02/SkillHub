package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func goodGeneratedSkill() llmclient.GeneratedSkill {
	return llmclient.GeneratedSkill{
		Name:          "scanned-invoice-table",
		Description:   "從掃描的單據影像抽出表格內容並合併成一份檔案。當使用者手上是掃描件時使用。",
		Compatibility: "需要能讀取影像或 PDF 的工具。",
		AllowedTools:  "Read Write",
		Body:          "# 掃描單據轉表格\n\n1. 確認每份檔案是影像還是 PDF。\n2. 逐份抽出表格。\n",
	}
}

func validateGenerated(t *testing.T, g llmclient.GeneratedSkill) skillpkg.Report {
	t.Helper()
	data, err := buildGeneratedPackage(g)
	if err != nil {
		t.Fatalf("buildGeneratedPackage: %v", err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Fatalf("PackageFS: %v", err)
	}
	return skillpkg.Validate(fsys)
}

func TestAGeneratedAnswerPassesTheImportValidator(t *testing.T) {
	r := validateGenerated(t, goodGeneratedSkill())
	if r.Blocked {
		t.Fatalf("blocked: %+v", r.Findings)
	}
	if r.Manifest.Name != "scanned-invoice-table" {
		t.Errorf("name = %q", r.Manifest.Name)
	}
	if r.Manifest.Compatibility == "" {
		t.Error("compatibility did not survive serialisation")
	}
}

func TestGeneratedFrontmatterHasNoLicence(t *testing.T) {
	data, err := buildGeneratedPackage(goodGeneratedSkill())
	if err != nil {
		t.Fatal(err)
	}
	if bytes := string(data); strings.Contains(bytes, "license") {
		t.Error("the archive mentions a license key")
	}
	r := validateGenerated(t, goodGeneratedSkill())
	if r.Manifest.License != "" || r.LicenseExpression != "" {
		t.Errorf("licence leaked: manifest=%q expression=%q", r.Manifest.License, r.LicenseExpression)
	}
}

func TestTheSameAnswerAlwaysProducesTheSameBytes(t *testing.T) {
	first, err := buildGeneratedPackage(goodGeneratedSkill())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := buildGeneratedPackage(goodGeneratedSkill())
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("run %d produced different bytes for the same answer", i)
		}
	}
}

func TestAnEscapingFilePathIsBlockedNotFiltered(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []llmclient.GeneratedFile{{Path: "../../evil.sh", Content: "echo hi\n"}}
	r := validateGenerated(t, g)
	if !r.Blocked {
		t.Fatal("an escaping entry was accepted")
	}
	if !containsCode(r, "entry-path-escape") {
		t.Errorf("blocked for the wrong reason: %v", blockingCodes(r))
	}
}

func TestASecondSkillMDIsRefused(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []llmclient.GeneratedFile{{Path: "SKILL.md", Content: "---\nname: other\n---\n"}}
	if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
		t.Fatalf("err = %v, want ErrGeneratedPackageInvalid", err)
	}
}

func TestPossibleSecretIsNotRetried(t *testing.T) {
	secret := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityError, Code: skillpkg.CodePossibleSecret, Path: "setup.sh"},
	}}
	slip := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityError, Code: "frontmatter-invalid-yaml", Path: "SKILL.md"},
	}}
	both := skillpkg.Report{Findings: append(append([]skillpkg.Finding{}, slip.Findings...), secret.Findings...)}

	if shouldRetry(1, secret) {
		t.Error("a credential-shaped line bought a second attempt")
	}
	if !shouldRetry(1, slip) {
		t.Error("a formatting slip did not get its one retry")
	}
	if shouldRetry(1, both) {
		t.Error("mixed findings retried; 02:GEN-003 says not retrying wins")
	}
	if shouldRetry(generateMaxAttempts, slip) {
		t.Error("retried past the ceiling")
	}
}

func TestBlankTaskDescriptionNeverReachesTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	for _, in := range []string{"", "   ", "\n\t \n"} {
		if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, GenerateInput{TaskDescription: in}); !errors.Is(err, ErrGenerateNoInput) {
			t.Errorf("GenerateSkill(%q) err = %v, want ErrGenerateNoInput", in, err)
		}
	}
}

func TestGeneratedTakesItsOwnRedistributionValue(t *testing.T) {
	ws := identity.Workspace{}
	if got := redistributionFor(ws, sourceMeta{Type: sourceGenerated}); got != "generated" {
		t.Errorf("generated -> %q", got)
	}
	if got := redistributionFor(ws, sourceMeta{Type: "upload"}); got == "generated" {
		t.Error("an upload took the generated value")
	}
	if got := redistributionFor(identity.Workspace{IsCatalog: true}, sourceMeta{Type: sourceGenerated}); got != "" {
		t.Errorf("the catalogue took %q", got)
	}
}

func containsCode(r skillpkg.Report, code string) bool {
	for _, c := range blockingCodes(r) {
		if c == code {
			return true
		}
	}
	return false
}

func TestTheSecondSkillMDIsCaughtUnderItsRealName(t *testing.T) {
	for _, p := range []string{"SKILL.md/", ".//SKILL.md", "././SKILL.md", "skill.MD", `.\SKILL.md`} {
		g := goodGeneratedSkill()
		g.Files = []llmclient.GeneratedFile{{Path: p, Content: "---\nname: other\n---\n"}}
		if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
			t.Errorf("path %q: err = %v, want ErrGeneratedPackageInvalid", p, err)
		}
	}
}

func TestAnEntryThatNamesNoFileIsRefused(t *testing.T) {
	for _, p := range []string{".", "./", "/", "././"} {
		g := goodGeneratedSkill()
		g.Files = []llmclient.GeneratedFile{{Path: p, Content: "AKIA0123456789ABCDEF\n"}}
		if _, err := buildGeneratedPackage(g); !errors.Is(err, ErrGeneratedPackageInvalid) {
			t.Errorf("path %q: err = %v, want ErrGeneratedPackageInvalid", p, err)
		}
	}
}

func TestATooShortOrTooLongDescriptionNeverReachesTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	ctx, ws := context.Background(), identity.Workspace{}

	for _, in := range []string{"abc", "  短  ", strings.Repeat("a", minTaskDescriptionRunes-1)} {
		if _, err := svc.GenerateSkill(ctx, ws, GenerateInput{TaskDescription: in}); !errors.Is(err, ErrGenerateBlank) {
			t.Errorf("GenerateSkill(%q) err = %v, want ErrGenerateBlank", in, err)
		}
	}

	if _, err := svc.GenerateSkill(ctx, ws, GenerateInput{TaskDescription: "整理發票單據"}); !errors.Is(err, ErrGenerateBlank) {
		t.Errorf("a six-character description was measured in bytes: %v", err)
	}
	long := strings.Repeat("台", maxTaskDescriptionRunes+1)
	if _, err := svc.GenerateSkill(ctx, ws, GenerateInput{TaskDescription: long}); !errors.Is(err, ErrGenerateTooLong) {
		t.Errorf("an over-long description err = %v, want ErrGenerateTooLong", err)
	}
}

func TestOneGenerationPerWorkspaceAtATime(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	ws := identity.Workspace{ID: mustUUIDForTest(t, "11111111-1111-1111-1111-111111111111")}
	other := identity.Workspace{ID: mustUUIDForTest(t, "22222222-2222-2222-2222-222222222222")}

	if !svc.holdGenerateSlot(ws.ID) {
		t.Fatal("the first hold on a fresh workspace failed")
	}
	if _, err := svc.GenerateSkill(context.Background(), ws, GenerateInput{TaskDescription: "把掃描的單據整理成表格。"}); !errors.Is(err, ErrGenerateInFlight) {
		t.Fatalf("err = %v, want ErrGenerateInFlight", err)
	}

	if svc.holdGenerateSlot(other.ID) != true {
		t.Error("a second workspace was blocked by the first workspace's slot")
	}
	svc.releaseGenerateSlot(other.ID)

	svc.releaseGenerateSlot(ws.ID)
	if !svc.holdGenerateSlot(ws.ID) {
		t.Error("the slot was not released")
	}
}

func mustUUIDForTest(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAnUploadCollidingWithAGeneratedSkillIsRefusedNotBroken(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Handler{}).respond(rec, Result{}, fmt.Errorf("upload: %w", ErrGeneratedNameCollision))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "同名") || strings.Contains(body, "import failed") {
		t.Errorf("the refusal does not tell the user what happened: %s", body)
	}
}

func gatewayReturning(t *testing.T, body string) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Service{LLM: &llmclient.Client{BaseURL: srv.URL}}
}

func TestAGenerationRecordsTheCostTheGatewayReported(t *testing.T) {
	const skill = `"skill":{"name":"a","description":"b","body":"c"},` +
		`"model":"m","prompt_version":"v"`
	cents := func(f float64) *float64 { return &f }

	for _, tc := range []struct {
		name       string
		usage      string
		wantCost   *float64
		wantPrompt int64
	}{{
		name:       "priced by the gateway",
		usage:      `,"usage":{"prompt_tokens":1200,"completion_tokens":800,"cost_usd":0.0123,"cost_source":"gateway"}`,
		wantCost:   cents(0.0123),
		wantPrompt: 1200,
	}, {

		name: "no usage reported",
	}, {

		name:       "tokens without a price",
		usage:      `,"usage":{"prompt_tokens":1200,"completion_tokens":800,"cost_usd":null,"cost_source":""}`,
		wantPrompt: 1200,
	}, {

		name:       "priced by something other than the gateway",
		usage:      `,"usage":{"prompt_tokens":1200,"completion_tokens":800,"cost_usd":0.0123,"cost_source":"estimated"}`,
		wantPrompt: 1200,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := gatewayReturning(t, "{"+skill+tc.usage+"}")
			resp, err := svc.generateOnce(context.Background(), pgtype.UUID{}, "把掃描的單據整理成表格。", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var out GenerateResult
			out.Attempts = 1
			out.addUsage(resp.Usage)

			switch {
			case tc.wantCost == nil && out.CostUSD != nil:
				t.Errorf("an unreported cost became %v; absence is not zero", *out.CostUSD)
			case tc.wantCost != nil && out.CostUSD == nil:
				t.Errorf("the gateway reported %v and nothing was recorded", *tc.wantCost)
			case tc.wantCost != nil && *out.CostUSD != *tc.wantCost:
				t.Errorf("cost = %v, want %v", *out.CostUSD, *tc.wantCost)
			}
			if out.PromptTokens != tc.wantPrompt {
				t.Errorf("prompt_tokens = %d, want %d", out.PromptTokens, tc.wantPrompt)
			}

			meta := map[string]any{}
			usageMeta(meta, out.CostUSD, out.PromptTokens, out.CompletionTokens)
			got, present := meta["cost_usd"]
			if (tc.wantCost != nil) != present {
				t.Fatalf("cost_usd present = %v in %v, want %v", present, meta, tc.wantCost != nil)
			}
			if present && got != *tc.wantCost {
				t.Errorf("audit cost_usd = %v, want %v", got, *tc.wantCost)
			}
		})
	}
}

func TestARetriedGenerationCostsWhatBothAttemptsCost(t *testing.T) {
	first := 0.01
	second := 0.02
	var out GenerateResult
	out.addUsage(&llmclient.GatewayUsage{PromptTokens: 100, CompletionTokens: 50,
		CostUSD: &first, CostSource: "gateway"})
	out.addUsage(&llmclient.GatewayUsage{PromptTokens: 100, CompletionTokens: 50,
		CostUSD: &second, CostSource: "gateway"})

	if out.CostUSD == nil || *out.CostUSD != first+second {
		t.Errorf("cost = %v, want %v", out.CostUSD, first+second)
	}
	if out.PromptTokens != 200 || out.CompletionTokens != 100 {
		t.Errorf("tokens = %d/%d, want 200/100", out.PromptTokens, out.CompletionTokens)
	}

	out.addUsage(&llmclient.GatewayUsage{PromptTokens: 100, CompletionTokens: 50})
	if out.CostUSD == nil || *out.CostUSD != first+second {
		t.Errorf("an unpriced attempt changed the total: %v", out.CostUSD)
	}
	if out.PromptTokens != 300 {
		t.Errorf("prompt_tokens = %d, want 300", out.PromptTokens)
	}
}

func requestCapturingStub(t *testing.T, body string) (*Service, *[]byte) {
	t.Helper()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		captured = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Service{LLM: &llmclient.Client{BaseURL: srv.URL}}, &captured
}

func TestDiagramOnlyReachesTheGatewayWithAnEmptyTaskDescription(t *testing.T) {
	const skillResp = `{"skill":{"name":"a","description":"b","body":"c"},"model":"m","prompt_version":"v"}`
	svc, captured := requestCapturingStub(t, skillResp)

	diagram := &GenerateDiagram{MediaType: "image/png", Data: []byte("not really a png, just bytes")}
	if _, err := svc.generateOnce(context.Background(), pgtype.UUID{}, "", diagram, nil); err != nil {
		t.Fatalf("generateOnce: %v", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(*captured, &sent); err != nil {
		t.Fatalf("decode what was sent: %v", err)
	}
	if _, present := sent["task_description"]; present {
		t.Errorf("task_description was sent as %v; omitempty should have dropped an empty one", sent["task_description"])
	}
	diag, ok := sent["diagram"].(map[string]any)
	if !ok {
		t.Fatalf("no diagram in the request: %v", sent)
	}
	if diag["media_type"] != "image/png" {
		t.Errorf("media_type = %v, want image/png", diag["media_type"])
	}
	if diag["data"] != base64.StdEncoding.EncodeToString(diagram.Data) {
		t.Errorf("data was not standard base64 of the decoded bytes")
	}
}

func TestGenerationInputsRecordsTheDiagramDigestNotTheBytes(t *testing.T) {
	diagram := &GenerateDiagram{MediaType: "image/webp", Data: []byte("some diagram bytes")}
	raw, err := marshalGenerationInputs(diagram, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "some diagram bytes") {
		t.Fatal("the raw image bytes leaked into generation_inputs")
	}
	var got struct {
		Diagram struct {
			MediaType string `json:"media_type"`
			SHA256    string `json:"sha256"`
			Bytes     int    `json:"bytes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(diagram.Data)
	if got.Diagram.MediaType != "image/webp" {
		t.Errorf("media_type = %q, want image/webp", got.Diagram.MediaType)
	}
	if got.Diagram.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q, want the digest of the decoded bytes", got.Diagram.SHA256)
	}
	if got.Diagram.Bytes != len(diagram.Data) {
		t.Errorf("bytes = %d, want %d", got.Diagram.Bytes, len(diagram.Data))
	}
}

func TestAShortCaptionWithADiagramIsNotBlank(t *testing.T) {
	if err := classifyTaskDescription("abc", true); err != nil {
		t.Errorf("classifyTaskDescription(short, withDiagram) = %v, want nil", err)
	}
	if err := classifyTaskDescription("abc", false); !errors.Is(err, ErrGenerateBlank) {
		t.Errorf("classifyTaskDescription(short, noDiagram) = %v, want ErrGenerateBlank", err)
	}
	if err := classifyTaskDescription("", true); err != nil {
		t.Errorf("classifyTaskDescription(empty, withDiagram) = %v, want nil", err)
	}
}

func TestNoTextAndNoDiagramNeverReachesTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, GenerateInput{}); !errors.Is(err, ErrGenerateNoInput) {
		t.Errorf("err = %v, want ErrGenerateNoInput", err)
	}
}

func TestAnOversizedDiagramIsRefusedBeforeTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	in := GenerateInput{Diagram: &GenerateDiagram{
		MediaType: "image/png",
		Data:      make([]byte, generateMaxDiagramBytes+1),
	}}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrDiagramInvalid) {
		t.Errorf("err = %v, want ErrDiagramInvalid", err)
	}
}

func TestFourReferencesIsRefusedBeforeTheGateway(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	var ids []pgtype.UUID
	for i := 1; i <= generateMaxReferences+1; i++ {
		ids = append(ids, mustUUIDForTest(t, fmt.Sprintf("00000000-0000-0000-0000-%012d", i)))
	}
	in := GenerateInput{TaskDescription: "把掃描的單據整理成一份表格。", ReferenceSkillIDs: ids}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrTooManyReferences) {
		t.Errorf("err = %v, want ErrTooManyReferences", err)
	}
}

type fakeReferenceReader struct {
	workspace map[string]registry.Skill
	catalog   map[string]registry.Skill
	versions  map[string]registry.Version
}

func (f fakeReferenceReader) WorkspaceSkill(_ context.Context, _, skillID pgtype.UUID) (registry.Skill, bool, error) {
	s, ok := f.workspace[pgconv.UUIDString(skillID)]
	return s, ok, nil
}

func (f fakeReferenceReader) CatalogSkill(_ context.Context, skillID pgtype.UUID) (registry.Skill, bool, error) {
	s, ok := f.catalog[pgconv.UUIDString(skillID)]
	return s, ok, nil
}

func (f fakeReferenceReader) LatestVersion(_ context.Context, _, skillID pgtype.UUID) (registry.Version, bool, error) {
	v, ok := f.versions[pgconv.UUIDString(skillID)]
	return v, ok, nil
}

type fakeObjectStore map[string][]byte

func (f fakeObjectStore) Put(_ context.Context, key string, data []byte) error {
	f[key] = data
	return nil
}

func (f fakeObjectStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := f[key]
	if !ok {
		return nil, fmt.Errorf("fakeObjectStore: no object %q", key)
	}
	return data, nil
}

func TestAnUnresolvableReferenceIsRefusedBeforeTheGateway(t *testing.T) {
	svc := &Service{
		LLM:        &llmclient.Client{},
		References: fakeReferenceReader{},
	}
	missing := mustUUIDForTest(t, "99999999-9999-9999-9999-999999999999")
	in := GenerateInput{TaskDescription: "把掃描的單據整理成一份表格。", ReferenceSkillIDs: []pgtype.UUID{missing}}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrReferenceUnavailable) {
		t.Errorf("err = %v, want ErrReferenceUnavailable", err)
	}
}

func TestAReadableReferencesSkillMDReachesTheGateway(t *testing.T) {
	const skillMDContent = "---\nname: reference-skill\ndescription: A worked example.\n---\n\nDo the thing.\n"
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000001")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000002")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-000000000003")
	const objectKey = "packages/reference.zip"

	store := fakeObjectStore{objectKey: zipBytes(t, map[string]string{"SKILL.md": skillMDContent})}
	svc := &Service{
		Store: store,
		References: fakeReferenceReader{
			workspace: map[string]registry.Skill{
				pgconv.UUIDString(skillID): {ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: "self_supplied"},
			},
			versions: map[string]registry.Version{
				pgconv.UUIDString(skillID): {ID: versionID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: objectKey},
			},
		},
	}

	ref, prov, err := svc.resolveReference(context.Background(), ws, skillID)
	if err != nil {
		t.Fatalf("resolveReference: %v", err)
	}
	if ref.Name != "reference-skill" || ref.SkillMD != skillMDContent {
		t.Errorf("resolved reference = %+v, want the stored SKILL.md verbatim", ref)
	}
	if prov.SkillID != skillID || prov.VersionID != versionID || prov.Name != "reference-skill" {
		t.Errorf("provenance = %+v, want skill/version ids and the name", prov)
	}

	const skillResp = `{"skill":{"name":"a","description":"b","body":"c"},"model":"m","prompt_version":"v"}`
	fakeSvc, captured := requestCapturingStub(t, skillResp)
	if _, err := fakeSvc.generateOnce(context.Background(), pgtype.UUID{}, "抽出重點。", nil,
		[]llmclient.GenerateReference{ref}); err != nil {
		t.Fatalf("generateOnce: %v", err)
	}
	var sent struct {
		References []struct {
			Name    string `json:"name"`
			SkillMD string `json:"skill_md"`
		} `json:"references"`
	}
	if err := json.Unmarshal(*captured, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.References) != 1 || sent.References[0].SkillMD != skillMDContent {
		t.Errorf("references[0].skill_md = %+v, want the reference's SKILL.md verbatim", sent.References)
	}

	raw, err := marshalGenerationInputs(nil, []referenceProvenance{prov})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), skillMDContent) {
		t.Fatal("the reference's SKILL.md content leaked into generation_inputs")
	}
	var got struct {
		References []struct {
			SkillID   string `json:"skill_id"`
			VersionID string `json:"version_id"`
			Name      string `json:"name"`
		} `json:"references"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.References) != 1 ||
		got.References[0].SkillID != pgconv.UUIDString(skillID) ||
		got.References[0].VersionID != pgconv.UUIDString(versionID) ||
		got.References[0].Name != "reference-skill" {
		t.Errorf("generation_inputs.references = %+v, want one entry naming the resolved skill/version", got.References)
	}
}

func TestALongReferenceIsCutToLeaveRoomForTheMarker(t *testing.T) {
	ws := identity.Workspace{ID: mustUUIDForTest(t, "10000000-0000-0000-0000-000000000011")}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-000000000012")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-000000000013")
	const objectKey = "packages/long-reference.zip"

	longBody := strings.Repeat("台", 30000)
	skillMD := "---\nname: reference-skill\ndescription: A worked example.\n---\n\n" + longBody + "\n"
	store := fakeObjectStore{objectKey: zipBytes(t, map[string]string{"SKILL.md": skillMD})}
	svc := &Service{
		Store: store,
		References: fakeReferenceReader{
			workspace: map[string]registry.Skill{
				pgconv.UUIDString(skillID): {ID: skillID, WorkspaceID: ws.ID, Name: "reference-skill", Redistribution: "self_supplied"},
			},
			versions: map[string]registry.Version{
				pgconv.UUIDString(skillID): {ID: versionID, SkillID: skillID, WorkspaceID: ws.ID, PackageObjectKey: objectKey},
			},
		},
	}

	ref, _, err := svc.resolveReference(context.Background(), ws, skillID)
	if err != nil {
		t.Fatalf("resolveReference: %v", err)
	}
	if n := len([]rune(ref.SkillMD)); n > generateMaxReferenceChars {
		t.Errorf("resolved reference is %d runes, want at most %d (the cap apps/llm enforces)", n, generateMaxReferenceChars)
	}
	if !strings.HasSuffix(ref.SkillMD, referenceTruncationMarker) {
		t.Errorf("a cut reference does not end with the truncation marker: %q", ref.SkillMD[max(0, len(ref.SkillMD)-40):])
	}
}

func TestADisallowedDiagramMediaTypeIsRefused(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	in := GenerateInput{Diagram: &GenerateDiagram{MediaType: "image/svg+xml", Data: []byte("<svg/>")}}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrDiagramInvalid) {
		t.Errorf("err = %v, want ErrDiagramInvalid", err)
	}
}

func TestReferencesAloneWithNoDescriptionOrDiagramIsRefused(t *testing.T) {
	svc := &Service{LLM: &llmclient.Client{}}
	someID := mustUUIDForTest(t, "40000000-0000-0000-0000-000000000001")
	in := GenerateInput{ReferenceSkillIDs: []pgtype.UUID{someID}}
	if _, err := svc.GenerateSkill(context.Background(), identity.Workspace{}, in); !errors.Is(err, ErrGenerateNoInput) {
		t.Errorf("err = %v, want ErrGenerateNoInput", err)
	}
}

func TestValidateCreationDraftReportsWhyThePackageCouldNotBeBuilt(t *testing.T) {
	g := goodGeneratedSkill()
	g.Files = []llmclient.GeneratedFile{{Path: "SKILL.md", Content: "---\nlicense: MIT\n---\n"}}
	hash, report, blocked, err := (&Service{}).ValidateCreationDraft(context.Background(), g)
	if err != nil || !blocked || hash != "" {
		t.Fatalf("a second SKILL.md must block without error: hash=%q blocked=%v err=%v", hash, blocked, err)
	}
	if !strings.Contains(report, "a second SKILL.md") || !strings.Contains(report, "license") {
		t.Fatalf("the report does not say what to change: %s", report)
	}
}
