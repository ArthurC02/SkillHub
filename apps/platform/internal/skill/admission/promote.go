package ingest

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type Promotion struct {
	VersionID     pgtype.UUID
	BaseVersionID pgtype.UUID
	By            pgtype.UUID
}

func (s *Service) PromoteVersion(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, p Promotion) error {
	if err := requireProjection(s.IndexSkill); err != nil {
		return err
	}
	inLine, err := registry.VersionInLine(ctx, s.Pool, ws.ID, p.VersionID)
	if err != nil || inLine {
		return err
	}
	held, err := s.heldVersion(ctx, ws.ID, skillID, p.VersionID)
	if err != nil {
		return err
	}
	if !held.HeldForEvaluation {
		return registry.ErrVersionNotHeld
	}
	if held.newest.ID != p.BaseVersionID {
		return ErrBaseVersionMoved
	}
	data, err := s.Store.Get(ctx, held.PackageObjectKey)
	if err != nil {
		return err
	}
	pkg, err := readPackage(data, held.SourcePath)
	if err != nil {
		return err
	}
	enriched := s.enrichPackage(ctx, pkg, ws.ID)
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		root, err := registry.LoadSkill(ctx, tx, ws.ID, skillID)
		if errors.Is(err, registry.ErrNotFound) {
			return ErrSkillNotFound
		}
		if err != nil {
			return err
		}
		root.Promote(held.Version, p.BaseVersionID, p.By)
		if err := registry.SaveSkill(ctx, tx, root); err != nil {
			return promotionError(err)
		}
		skill := root.Skill()
		return s.IndexSkill(ctx, tx, enriched.projection(ws.ID, skill.ID, skill.Name))
	})
}

type heldCandidate struct {
	registry.Version
	newest registry.Version
}

func (s *Service) heldVersion(ctx context.Context, workspaceID, skillID, versionID pgtype.UUID) (heldCandidate, error) {
	var held heldCandidate
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		version, found, err := registry.VersionIn(ctx, tx, workspaceID, versionID)
		if err == nil && (!found || version.SkillID != skillID) {
			return ErrSkillNotFound
		}
		if err != nil {
			return err
		}
		held.Version = version
		held.newest, _, err = registry.LatestVersionIn(ctx, tx, workspaceID, skillID)
		return err
	})
	return held, err
}

func promotionError(err error) error {
	if errors.Is(err, registry.ErrLineMovedOn) {
		return ErrBaseVersionMoved
	}
	return err
}
