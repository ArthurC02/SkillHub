package catalog

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type ExposedSkill struct {
	SkillID          pgtype.UUID
	VersionID        pgtype.UUID
	SnapshotDigest   string
	OwnerWorkspaceID pgtype.UUID
}

type SearchSnapshot struct {
	VersionID       pgtype.UUID
	Name            string
	Summary         string
	EnrichedSummary string
	TaskExamples    string
	Tags            json.RawMessage
	Limitations     string
	Enriched        bool
	Listable        bool
	Digest          string
}

type publicScope struct {
	catalogs    []pgtype.UUID
	exposedKeys []string
	exposed     map[pgtype.UUID]ExposedSkill
}

func exposureKey(skillID, versionID pgtype.UUID, digest string) string {
	return pgconv.UUIDString(skillID) + ":" + pgconv.UUIDString(versionID) + ":" + digest
}

func (s *Service) publicScope(ctx context.Context) (publicScope, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return publicScope{}, err
	}
	scope := publicScope{catalogs: catalogs, exposedKeys: []string{}, exposed: map[pgtype.UUID]ExposedSkill{}}
	if s.ExposedSkills == nil {
		return scope, nil
	}
	exposed, err := s.ExposedSkills(ctx)
	if err != nil {
		return publicScope{}, err
	}
	for _, e := range exposed {
		scope.exposedKeys = append(scope.exposedKeys, exposureKey(e.SkillID, e.VersionID, e.SnapshotDigest))
		scope.exposed[e.SkillID] = e
	}
	return scope, nil
}

func (s *Service) SearchSnapshotOf(ctx context.Context, skillID pgtype.UUID) (SearchSnapshot, bool, error) {
	row, err := gen.New(s.Pool).GetSearchSnapshot(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SearchSnapshot{}, false, nil
	}
	if err != nil {
		return SearchSnapshot{}, false, err
	}
	return SearchSnapshot{
		VersionID: row.LatestVersionID, Name: row.Name, Summary: row.Summary, EnrichedSummary: row.EnrichedSummary,
		TaskExamples: row.TaskExamples, Tags: json.RawMessage(row.Tags), Limitations: row.Limitations,
		Enriched: EnrichmentStatus(row.EnrichmentStatus) == EnrichmentEnriched, Listable: row.Listable,
		Digest: derefString(row.ExposureDigest),
	}, true, nil
}

func (s *Service) exposedSkill(ctx context.Context, id pgtype.UUID) (SkillFacts, bool, error) {
	scope, err := s.publicScope(ctx)
	if err != nil {
		return SkillFacts{}, false, err
	}
	exposed, ok := scope.exposed[id]
	if !ok {
		return SkillFacts{}, false, nil
	}
	snapshot, found, err := s.SearchSnapshotOf(ctx, id)
	if err != nil || !found || snapshot.VersionID != exposed.VersionID || snapshot.Digest != exposed.SnapshotDigest {
		return SkillFacts{}, false, err
	}
	return s.WorkspaceSkill(ctx, exposed.OwnerWorkspaceID, id)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
