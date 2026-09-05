package ingest

import (
	"context"
	"encoding/json"
	"errors"
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

// GeneratedCandidateProvenance is the durable source fact for an interactive
// candidate. ExistingSkillID is supplied only from the locked session snapshot.
type GeneratedCandidateProvenance struct {
	TaskDescription, Model, PromptVersion string
	GenerationInputs                      []byte
	ExistingSkillID                       *pgtype.UUID
}

// MaterializeGeneratedCandidate is the one creation-specific door into the
// ordinary generated admission path. It owns the object-write fence and calls
// after inside the exact transaction containing the source and version rows.
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

	// Revisions may only extend the same session's generated skill. Do not let a
	// caller turn an arbitrary workspace skill into a generated candidate.
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
	version, duplicate, err := s.persistVersion(ctx, tx, ws, existing, prepared, src, skipEnrichment(prepared))
	if err != nil {
		return Result{}, err
	}
	if duplicate {
		return Result{}, errors.New("ingest: candidate revision duplicates an existing version")
	}
	res := Result{Report: prepared.report, Skill: existing, Version: version}
	if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, map[string]any{"source_type": sourceGenerated}); err != nil {
		return Result{}, err
	}
	if after != nil {
		if err := after(ctx, tx, res); err != nil {
			return Result{}, err
		}
	}
	return res, tx.Commit(ctx)
}

type FixedCreationReference struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
	Name      string
}

// ReadCreationReference checks current availability before reading an immutable
// version. An empty version selects once; subsequent turns supply its exact ID.
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
	return FixedCreationReference{skill.ID, version.ID, skill.Name}, llmclient.GenerateReference{Name: skill.Name, SkillMD: text}, nil
}

// ValidateCreationDraft uses the exact admission validator and package hash.
// It does not execute scripts, publish objects, or invoke a model.
func (s *Service) ValidateCreationDraft(ctx context.Context, draft llmclient.GeneratedSkill) (string, string, bool, error) {
	data, err := buildGeneratedPackage(draft)
	if err != nil {
		return "", "套件結構無法通過驗證。", true, nil
	}
	prepared, err := s.prepare(ctx, data)
	if err != nil {
		return "", "", true, err
	}
	report, err := json.Marshal(prepared.report)
	return prepared.contentHash, string(report), prepared.report.Blocked, err
}
