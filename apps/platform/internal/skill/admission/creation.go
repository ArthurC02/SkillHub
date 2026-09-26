package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type GeneratedCandidateProvenance struct {
	TaskDescription, Model, PromptVersion string
	GenerationInputs                      []byte
	ExistingSkillID                       *pgtype.UUID
}

func (s *Service) MaterializeGeneratedCandidate(ctx context.Context, ws identity.Workspace, skill GeneratedSkill, p GeneratedCandidateProvenance, after func(context.Context, pgx.Tx, Result) error) (Result, error) {
	data, err := buildGeneratedPackage(skill)
	if err != nil {
		return Result{}, err
	}
	desc, model, prompt := p.TaskDescription, p.Model, p.PromptVersion
	src := sourceMeta{Type: SourceGenerated, TaskDescription: &desc, GeneratorModel: &model, GeneratorPromptVersion: &prompt, GenerationInputs: p.GenerationInputs, Interactive: true}
	if p.ExistingSkillID == nil {
		return s.importZipWithCommit(ctx, ws, data, src, after)
	}

	prepared, err := s.prepare(ctx, data)
	if err != nil || prepared.report.Blocked {
		return Result{Report: prepared.report}, err
	}
	if err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		_, err := loadGeneratedSkill(ctx, tx, ws.ID, *p.ExistingSkillID)
		return err
	}); err != nil {
		return Result{}, err
	}
	enriched := s.enrichPackage(ctx, prepared, ws.ID)
	tx, release, err := s.beginPackageWrite(ctx, ws, prepared.objectKey, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	existing, err := loadGeneratedSkill(ctx, tx, ws.ID, *p.ExistingSkillID)
	if err != nil {
		return Result{}, err
	}
	version, duplicate, err := s.persistVersion(ctx, tx, ws, existing, prepared, src, enriched)
	if err != nil {
		return Result{}, err
	}

	res := Result{Report: prepared.report, Skill: existing.Skill(), Version: version, Duplicate: duplicate}
	if !duplicate {
		if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, map[string]any{"source_type": string(SourceGenerated)}); err != nil {
			return Result{}, err
		}
	}
	if after != nil {
		if err := after(ctx, tx, res); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

func loadGeneratedSkill(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (*registry.SkillRoot, error) {
	skill, err := registry.LoadSkill(ctx, tx, workspaceID, skillID)
	if errors.Is(err, registry.ErrNotFound) {
		return nil, ErrGeneratedNameCollision
	}
	if err != nil {
		return nil, err
	}
	if !skill.Generated() {
		return nil, ErrGeneratedNameCollision
	}
	return skill, nil
}

type FixedCreationReference struct {
	SkillID       pgtype.UUID
	VersionID     pgtype.UUID
	Name          string
	Description   string
	Compatibility string
	AllowedTools  string
}

func (s *Service) ReadCreationReference(ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID) (FixedCreationReference, ReferenceSkill, error) {
	if s.References == nil || s.Store == nil {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	skill, found, err := s.References.WorkspaceSkill(ctx, ws.ID, skillID)
	if err != nil {
		return FixedCreationReference{}, ReferenceSkill{}, err
	}
	if !found {
		skill, found, err = s.References.CatalogSkill(ctx, skillID)
	}
	if err != nil || !found || !referenceable(skill) {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	var version registry.Version
	if versionID.Valid {
		version, found, err = s.References.WorkspaceVersion(ctx, skill.WorkspaceID, versionID)
	} else {
		version, found, err = s.References.LatestVersion(ctx, skill.WorkspaceID, skill.ID)
	}
	if err != nil || !found || version.SkillID != skill.ID {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	data, err := s.Store.Get(ctx, version.PackageObjectKey)
	if err != nil {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	tree, err := skillpkg.SkillFS(data, version.SourcePath)
	if err != nil {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	md, err := fs.ReadFile(tree, "SKILL.md")
	if err != nil {
		return FixedCreationReference{}, ReferenceSkill{}, ErrReferenceUnavailable
	}
	text, truncated := cutRunes(strings.ToValidUTF8(string(md), ""), generateMaxReferenceChars-utf8.RuneCountInString(referenceTruncationMarker))
	if truncated {
		text += referenceTruncationMarker
	}
	fixed := FixedCreationReference{SkillID: skill.ID, VersionID: version.ID, Name: skill.Name}
	report := skillpkg.Validate(tree)
	if report.Manifest != nil {
		fixed.Description = report.Manifest.Description
		fixed.Compatibility = report.Manifest.Compatibility
		fixed.AllowedTools = strings.Join(report.Manifest.AllowedTools, " ")
	}
	return fixed, ReferenceSkill{Name: skill.Name, SkillMD: text}, nil
}

func (s *Service) ValidateCreationDraft(ctx context.Context, draft GeneratedSkill) (string, string, bool, error) {
	data, err := buildGeneratedPackage(draft)
	if err != nil {

		return "", fmt.Sprintf("套件結構無法通過驗證：%v。frontmatter 與 SKILL.md 由 Go 從 name、description、compatibility、allowed_tools 與 body 產生；files 不得包含 SKILL.md，也沒有 license 欄位可填。", err), true, nil
	}
	prepared, err := s.prepare(ctx, data)
	if err != nil {
		return "", "", true, err
	}
	report, err := json.Marshal(prepared.report)
	return prepared.contentHash, string(report), prepared.report.Blocked, err
}
