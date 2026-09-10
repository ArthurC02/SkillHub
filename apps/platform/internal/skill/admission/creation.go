package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type GeneratedCandidateProvenance struct {
	TaskDescription, Model, PromptVersion string
	GenerationInputs                      []byte
	ExistingSkillID                       *pgtype.UUID
}

func (s *Service) MaterializeGeneratedCandidate(ctx context.Context, ws identity.Workspace, skill llmclient.GeneratedSkill, p GeneratedCandidateProvenance, after func(context.Context, pgx.Tx, Result) error) (Result, error) {
	data, err := buildGeneratedPackage(skill)
	if err != nil {
		return Result{}, err
	}
	desc, model, prompt := p.TaskDescription, p.Model, p.PromptVersion
	src := sourceMeta{Type: sourceGenerated, TaskDescription: &desc, GeneratorModel: &model, GeneratorPromptVersion: &prompt, GenerationInputs: p.GenerationInputs}
	if p.ExistingSkillID == nil {
		return s.importZipWithCommit(ctx, ws, data, src, after)
	}

	prepared, err := s.prepare(ctx, data)
	if err != nil || prepared.report.Blocked {
		return Result{Report: prepared.report}, err
	}
	tx, release, err := s.beginPackageWrite(ctx, ws, prepared, data)
	if err != nil {
		return Result{}, err
	}
	defer release()
	existing, found, err := registry.SkillByID(ctx, tx, ws.ID, *p.ExistingSkillID)
	if err != nil {
		return Result{}, err
	}
	if !found || existing.Redistribution != registry.RedistributionGenerated {
		return Result{}, ErrGeneratedNameCollision
	}
	version, duplicate, err := s.persistVersion(ctx, tx, ws, existing, prepared, src, s.enrichPackage(ctx, prepared, ws.ID))
	if err != nil {
		return Result{}, err
	}

	res := Result{Report: prepared.report, Skill: existing, Version: version, Duplicate: duplicate}
	if !duplicate {
		if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, map[string]any{"source_type": sourceGenerated}); err != nil {
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

type FixedCreationReference struct {
	SkillID       pgtype.UUID
	VersionID     pgtype.UUID
	Name          string
	Description   string
	Compatibility string
	AllowedTools  string
}

func (s *Service) ReadCreationReference(ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID) (FixedCreationReference, llmclient.GenerateReference, error) {
	if s.References == nil || s.Store == nil {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
	}
	skill, found, err := s.References.WorkspaceSkill(ctx, ws.ID, skillID)
	if err != nil {
		return FixedCreationReference{}, llmclient.GenerateReference{}, err
	}
	if !found {
		skill, found, err = s.References.CatalogSkill(ctx, skillID)
	}
	if err != nil || !found || skill.TakedownAt.Valid || skill.AccessRestriction != nil || skill.Redistribution == "blocked" {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
	}
	var version registry.Version
	if versionID.Valid {
		reader, ok := s.References.(interface {
			WorkspaceVersion(context.Context, pgtype.UUID, pgtype.UUID) (registry.Version, bool, error)
		})
		if !ok {
			return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
		}
		version, found, err = reader.WorkspaceVersion(ctx, skill.WorkspaceID, versionID)
	} else {
		version, found, err = s.References.LatestVersion(ctx, skill.WorkspaceID, skill.ID)
	}
	if err != nil || !found || version.SkillID != skill.ID {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
	}
	data, err := s.Store.Get(ctx, version.PackageObjectKey)
	if err != nil {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
	}
	tree, err := skillpkg.PackageFS(data)
	if err != nil {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
	}
	md, err := fs.ReadFile(tree, "SKILL.md")
	if err != nil {
		return FixedCreationReference{}, llmclient.GenerateReference{}, ErrReferenceUnavailable
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
	return fixed, llmclient.GenerateReference{Name: skill.Name, SkillMD: text}, nil
}

func (s *Service) ValidateCreationDraft(ctx context.Context, draft llmclient.GeneratedSkill) (string, string, bool, error) {
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
