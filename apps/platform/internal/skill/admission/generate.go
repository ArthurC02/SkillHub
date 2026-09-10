package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"gopkg.in/yaml.v3"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

const generateMaxAttempts = 2 // one-number: generateMaxAttempts

var ErrGenerateBlank = errors.New("ingest: task description is empty or too short to act on")

var ErrGenerateTooLong = errors.New("ingest: task description is too long")

var ErrGenerateInFlight = errors.New("ingest: a generation is already running for this workspace")

const (
	minTaskDescriptionRunes = 8
	maxTaskDescriptionRunes = 4000 // one-number: generateMaxTaskRunes
)

var ErrGenerateNotForCatalogue = errors.New("ingest: the catalogue does not generate skills")

var ErrGenerateNoInput = errors.New("ingest: nothing to generate from — write a description or attach a diagram")

var ErrDiagramInvalid = errors.New("ingest: diagram is not a usable image")

var ErrTooManyReferences = errors.New("ingest: too many reference skills")

var ErrReferenceUnavailable = errors.New("ingest: a reference skill could not be used")

const generateMaxDiagramBytes = 4_000_000 // one-number: generateMaxDiagramBytes

const generateMaxReferences = 3 // one-number: generateMaxReferences

const generateMaxReferenceChars = 20000 // one-number: generateMaxReferenceChars

const referenceTruncationMarker = "…[truncated]"

func classifyTaskDescription(task string, hasDiagram bool) error {
	n := len([]rune(task))
	switch {
	case n == 0 && !hasDiagram:
		return ErrGenerateNoInput
	case n > maxTaskDescriptionRunes:
		return ErrGenerateTooLong
	case n > 0 && n < minTaskDescriptionRunes && !hasDiagram:
		return ErrGenerateBlank
	}
	return nil
}

type GenerateDiagram struct {
	MediaType string
	Data      []byte
}

type GenerateInput struct {
	TaskDescription   string
	Diagram           *GenerateDiagram
	ReferenceSkillIDs []pgtype.UUID
}

type ReferenceReader interface {
	WorkspaceSkill(ctx context.Context, workspaceID, skillID pgtype.UUID) (registry.Skill, bool, error)
	CatalogSkill(ctx context.Context, skillID pgtype.UUID) (registry.Skill, bool, error)
	LatestVersion(ctx context.Context, workspaceID, skillID pgtype.UUID) (registry.Version, bool, error)
}

var ErrGeneratedPackageInvalid = errors.New("ingest: generated skill cannot be packaged")

var ErrGeneratedNameCollision = errors.New(
	"ingest: this workspace already has a skill with that name, and at least one of the two is generated")

const (
	FailureQuota         = "quota"
	FailureUnavailable   = "unavailable"
	FailureGateway       = "gateway"
	FailureUnpackageable = "unpackageable"
	FailureRejected      = "rejected"
	FailureBlocked       = "blocked"
)

var FailureVocabulary = []string{
	FailureQuota, FailureUnavailable, FailureGateway, FailureUnpackageable, FailureRejected, FailureBlocked,
}

type GenerateResult struct {
	Result

	Attempts int

	Model         string
	PromptVersion string

	CostUSD *float64

	PromptTokens     int64
	CompletionTokens int64
}

func (r *GenerateResult) addUsage(u *llmclient.GatewayUsage) {
	if u == nil {
		return
	}
	r.PromptTokens += u.PromptTokens
	r.CompletionTokens += u.CompletionTokens
	if u.CostUSD == nil || u.CostSource != "gateway" {
		return
	}
	total := *u.CostUSD
	if r.CostUSD != nil {
		total += *r.CostUSD
	}
	r.CostUSD = &total
}

func (s *Service) GenerateSkill(ctx context.Context, ws identity.Workspace, in GenerateInput) (GenerateResult, error) {
	if s.LLM == nil {
		return GenerateResult{}, errors.New("ingest: generation needs an LLM service")
	}

	if ws.IsCatalog {
		return GenerateResult{}, ErrGenerateNotForCatalogue
	}
	task := strings.TrimSpace(in.TaskDescription)

	if err := classifyTaskDescription(task, in.Diagram != nil); err != nil {
		return GenerateResult{}, err
	}

	if in.Diagram != nil {
		if !validDiagramMediaType(in.Diagram.MediaType) ||
			len(in.Diagram.Data) == 0 || len(in.Diagram.Data) > generateMaxDiagramBytes {
			return GenerateResult{}, ErrDiagramInvalid
		}
	}

	if len(in.ReferenceSkillIDs) > generateMaxReferences {
		return GenerateResult{}, ErrTooManyReferences
	}

	var references []llmclient.GenerateReference
	var refProvenance []referenceProvenance
	if len(in.ReferenceSkillIDs) > 0 {
		if s.References == nil {
			return GenerateResult{}, ErrReferenceUnavailable
		}
		for _, id := range in.ReferenceSkillIDs {
			ref, prov, err := s.resolveReference(ctx, ws, id)
			if err != nil {
				return GenerateResult{}, err
			}
			references = append(references, ref)
			refProvenance = append(refProvenance, prov)
		}
	}
	generationInputsJSON, err := marshalGenerationInputs(in.Diagram, refProvenance)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("ingest: generation_inputs: %w", err)
	}

	if !s.holdGenerateSlot(ws.ID) {
		return GenerateResult{}, ErrGenerateInFlight
	}
	defer s.releaseGenerateSlot(ws.ID)

	if reason, err := s.requireGenerateAllowance(ctx, ws.ID); err != nil {

		failure := FailureQuota
		if errors.Is(err, policy.ErrAllowanceUnavailable) {
			failure = FailureUnavailable
		}

		s.auditGenerateFailure(ctx, ws, task, in, GenerateResult{}, map[string]any{
			"failure": failure,
			"reason":  reason,
		})
		return GenerateResult{}, err
	}

	var out GenerateResult

	defer func() {
		if out.Attempts == 0 {
			return
		}
		attrs := []any{
			"attempts", out.Attempts, "model", out.Model,
			"prompt_tokens", out.PromptTokens, "completion_tokens", out.CompletionTokens,
		}

		if out.CostUSD != nil {
			attrs = append(attrs, "cost_usd", *out.CostUSD)
		}
		slog.Info("generate: model usage", attrs...)
	}()
	for attempt := 1; attempt <= generateMaxAttempts; attempt++ {
		out.Attempts = attempt

		gen, err := s.generateOnce(ctx, ws.ID, task, in.Diagram, references)
		if err != nil {

			slog.Warn("generate: gateway call failed", "attempt", attempt, "error", err)

			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure":   FailureGateway,
				"truncated": errors.Is(err, llmclient.ErrGenerateTruncated),
			})
			return out, err
		}
		out.Model, out.PromptVersion = gen.Model, gen.PromptVersion

		out.addUsage(gen.Usage)

		data, err := buildGeneratedPackage(gen.Skill)
		if err != nil {
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{"failure": FailureUnpackageable})
			return out, err
		}

		desc, model, promptVersion := task, gen.Model, gen.PromptVersion
		res, err := s.importZip(ctx, ws, data, sourceMeta{
			Type:                   sourceGenerated,
			TaskDescription:        &desc,
			GeneratorModel:         &model,
			GeneratorPromptVersion: &promptVersion,

			CostUSD:          out.CostUSD,
			PromptTokens:     out.PromptTokens,
			CompletionTokens: out.CompletionTokens,

			GenerationInputs: generationInputsJSON,
		})
		if err != nil {

			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure":   FailureRejected,
				"collision": errors.Is(err, ErrGeneratedNameCollision),
			})
			return out, err
		}
		out.Result = res
		if !res.Report.Blocked {
			return out, nil
		}

		if !shouldRetry(attempt, res.Report) {
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure": FailureBlocked,

				"codes": blockingCodes(res.Report),
			})
			return out, nil
		}
	}
	return out, nil
}

func shouldRetry(attempt int, r skillpkg.Report) bool {
	return attempt < generateMaxAttempts &&
		!slices.Contains(blockingCodes(r), skillpkg.CodePossibleSecret)
}

// budget-over: generate.LLM_TIMEOUT_SECONDS
const generateTimeout = 130 * time.Second

func (s *Service) generateOnce(
	ctx context.Context, workspaceID pgtype.UUID, task string,
	diagram *GenerateDiagram, references []llmclient.GenerateReference,
) (*llmclient.GenerateSkillResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	req := llmclient.GenerateSkillRequest{TaskDescription: task, References: references}
	if diagram != nil {

		req.Diagram = &llmclient.GenerateDiagram{
			MediaType: diagram.MediaType,
			Data:      base64.StdEncoding.EncodeToString(diagram.Data),
		}
	}
	resp, err := s.LLM.GenerateSkill(callCtx, req)
	if err != nil {
		return nil, err
	}

	s.recordCost(ctx, credit.KindGenerate, workspaceID, resp.Model, resp.PromptVersion, resp.Usage)
	return resp, nil
}

func validDiagramMediaType(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

type referenceProvenance struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
	Name      string
}

func (s *Service) resolveReference(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID,
) (llmclient.GenerateReference, referenceProvenance, error) {
	skill, found, err := s.References.WorkspaceSkill(ctx, ws.ID, id)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, err
	}
	if !found {
		skill, found, err = s.References.CatalogSkill(ctx, id)
		if err != nil {
			return llmclient.GenerateReference{}, referenceProvenance{}, err
		}
	}
	if !found || skill.TakedownAt.Valid || skill.AccessRestriction != nil || skill.Redistribution == "blocked" {
		return llmclient.GenerateReference{}, referenceProvenance{}, ErrReferenceUnavailable
	}

	version, found, err := s.References.LatestVersion(ctx, skill.WorkspaceID, skill.ID)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, err
	}
	if !found {
		return llmclient.GenerateReference{}, referenceProvenance{}, ErrReferenceUnavailable
	}

	data, err := s.Store.Get(ctx, version.PackageObjectKey)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}
	md, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}

	content, truncated := cutRunes(strings.ToValidUTF8(string(md), ""),
		generateMaxReferenceChars-utf8.RuneCountInString(referenceTruncationMarker))
	if truncated {
		content += referenceTruncationMarker
	}
	return llmclient.GenerateReference{Name: skill.Name, SkillMD: content},
		referenceProvenance{SkillID: skill.ID, VersionID: version.ID, Name: skill.Name}, nil
}

func cutRunes(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

type generationInputsDiagram struct {
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
}

type generationInputsReference struct {
	SkillID   string `json:"skill_id"`
	VersionID string `json:"version_id"`
	Name      string `json:"name"`
}

type generationInputsRecord struct {
	Diagram    *generationInputsDiagram    `json:"diagram,omitempty"`
	References []generationInputsReference `json:"references,omitempty"`
}

func marshalGenerationInputs(diagram *GenerateDiagram, refs []referenceProvenance) ([]byte, error) {
	if diagram == nil && len(refs) == 0 {
		return nil, nil
	}
	rec := generationInputsRecord{}
	if diagram != nil {
		sum := sha256.Sum256(diagram.Data)
		rec.Diagram = &generationInputsDiagram{
			MediaType: diagram.MediaType,
			SHA256:    hex.EncodeToString(sum[:]),
			Bytes:     len(diagram.Data),
		}
	}
	for _, r := range refs {
		rec.References = append(rec.References, generationInputsReference{
			SkillID: pgconv.UUIDString(r.SkillID), VersionID: pgconv.UUIDString(r.VersionID), Name: r.Name,
		})
	}
	return json.Marshal(rec)
}

func blockingCodes(r skillpkg.Report) []string {
	var codes []string
	for _, f := range r.Findings {
		if f.Severity == skillpkg.SeverityError && !slices.Contains(codes, f.Code) {
			codes = append(codes, f.Code)
		}
	}
	return codes
}

type generatedFrontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Compatibility string `yaml:"compatibility,omitempty"`
	AllowedTools  string `yaml:"allowed-tools,omitempty"`
}

func buildGeneratedPackage(g llmclient.GeneratedSkill) ([]byte, error) {
	fm, err := yaml.Marshal(generatedFrontmatter{
		Name:          g.Name,
		Description:   g.Description,
		Compatibility: g.Compatibility,
		AllowedTools:  g.AllowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: frontmatter: %v", ErrGeneratedPackageInvalid, err)
	}

	var md bytes.Buffer
	md.WriteString("---\n")
	md.Write(fm)
	md.WriteString("---\n\n")
	md.WriteString(g.Body)
	if !strings.HasSuffix(g.Body, "\n") {
		md.WriteString("\n")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("%w: entry %q: %v", ErrGeneratedPackageInvalid, name, err)
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("SKILL.md", md.String()); err != nil {
		return nil, err
	}
	for _, f := range g.Files {

		clean := path.Clean(strings.ReplaceAll(f.Path, `\`, "/"))
		if strings.EqualFold(clean, "SKILL.md") {
			return nil, fmt.Errorf("%w: a second SKILL.md at %q", ErrGeneratedPackageInvalid, f.Path)
		}

		if clean == "." || clean == "" || clean == "/" {
			return nil, fmt.Errorf("%w: entry %q names no file", ErrGeneratedPackageInvalid, f.Path)
		}

		if err := write(f.Path, f.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneratedPackageInvalid, err)
	}
	return buf.Bytes(), nil
}

func (s *Service) auditGenerateFailure(
	ctx context.Context, ws identity.Workspace, task string, in GenerateInput, out GenerateResult, meta map[string]any,
) {
	meta["attempts"] = out.Attempts
	meta["task_description_chars"] = len([]rune(task))

	meta["diagram"] = in.Diagram != nil
	meta["references"] = len(in.ReferenceSkillIDs)
	usageMeta(meta, out.CostUSD, out.PromptTokens, out.CompletionTokens)

	ctx = context.WithoutCancel(ctx)
	if err := audit.Log(ctx, s.Pool, audit.Event{
		Actor:     ws.OwnerUserID,
		Workspace: ws.ID,
		Action:    audit.ActionSkillGenerateFailed,

		ResourceType: audit.ResourceSkill,
		Metadata:     meta,
	}); err != nil {
		slog.Error("generate: failure record not written", "error", err)
	}
}

func usageMeta(meta map[string]any, cost *float64, prompt, completion int64) {
	if cost != nil {
		meta["cost_usd"] = *cost
	}
	if prompt > 0 || completion > 0 {
		meta["prompt_tokens"] = prompt
		meta["completion_tokens"] = completion
	}
}

func (s *Service) requireGenerateAllowance(ctx context.Context, workspaceID pgtype.UUID) (string, error) {
	if !s.GenerateQuota.Enforced() {
		return "", nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		slog.Error("generate: the allowance could not be counted", "error", err)
		return "generate_quota_unavailable", fmt.Errorf("%w: %w", policy.ErrAllowanceUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	return policy.EnforceGenerateQuota(ctx, generateUsageReader(tx), s.GenerateQuota, workspaceID)
}

func generateUsageReader(tx pgx.Tx) policy.UsageReader {
	q := gen.New(tx)
	return policy.UsageReader{
		WorkspaceCreatedAt: func(ctx context.Context, id pgtype.UUID) (time.Time, error) {
			return identity.ReadWorkspaceCreatedAt(ctx, tx, id)
		},
		CountRuns: func(ctx context.Context, id pgtype.UUID, since time.Time) (policy.RunUsage, error) {
			row, err := q.CountGeneratedSkills(ctx, gen.CountGeneratedSkillsParams{
				WorkspaceID: id, Since: pgtype.Timestamptz{Time: since, Valid: true},
			})
			if err != nil {
				return policy.RunUsage{}, err
			}
			u := policy.RunUsage{Used: row.Used}
			if row.Oldest.Valid {
				oldest := row.Oldest.Time
				u.Oldest = &oldest
			}
			return u, nil
		},
	}
}

func (s *Service) holdGenerateSlot(workspaceID pgtype.UUID) bool {
	_, busy := s.generating.LoadOrStore(workspaceID, struct{}{})
	return !busy
}

func (s *Service) releaseGenerateSlot(workspaceID pgtype.UUID) {
	s.generating.Delete(workspaceID)
}
