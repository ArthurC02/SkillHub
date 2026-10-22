package catalog

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type EnrichmentBackfill struct {
	Pool *pgxpool.Pool
}

type PendingEnrichment struct {
	SkillID          pgtype.UUID
	VersionID        pgtype.UUID
	WorkspaceID      pgtype.UUID
	Name             string
	PackageObjectKey string
	SourcePath       string
}

type MissingEmbedding struct {
	SkillID         pgtype.UUID
	WorkspaceID     pgtype.UUID
	Name            string
	Summary         string
	EnrichedSummary string
	TaskExamples    string
	Tags            []byte
}

func (b *EnrichmentBackfill) OldestPending(ctx context.Context) (pgtype.Timestamptz, error) {
	return gen.New(b.Pool).OldestPendingEnrichment(ctx, string(EnrichmentPending))
}

func (b *EnrichmentBackfill) Pending(ctx context.Context, limit int32) ([]PendingEnrichment, error) {
	rows, err := gen.New(b.Pool).ListPendingEnrichment(ctx, gen.ListPendingEnrichmentParams{ClaimLease: pgconv.Interval(queue.SweepClaimLease), BatchSize: limit})
	if err != nil {
		return nil, err
	}
	result := make([]PendingEnrichment, 0, len(rows))
	for _, row := range rows {
		if row.PackageObjectKey == nil {
			continue
		}
		result = append(result, PendingEnrichment{
			SkillID:          row.SkillID,
			VersionID:        row.VersionID,
			WorkspaceID:      row.WorkspaceID,
			Name:             row.Name,
			PackageObjectKey: *row.PackageObjectKey,
			SourcePath:       row.SourcePath,
		})
	}
	return result, nil
}

func (b *EnrichmentBackfill) EmbedMissing(ctx context.Context, limit int32, embed func(context.Context, MissingEmbedding) *pgvector.Vector) (done, failed int, err error) {
	q := gen.New(b.Pool)
	rows, err := q.ClaimEnrichedWithoutEmbedding(ctx, gen.ClaimEnrichedWithoutEmbeddingParams{ClaimLease: pgconv.Interval(queue.SweepClaimLease), BatchSize: limit})
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		vector := embed(ctx, MissingEmbedding{
			SkillID: row.SkillID, WorkspaceID: row.WorkspaceID, Name: row.Name, Summary: row.Summary,
			EnrichedSummary: row.EnrichedSummary, TaskExamples: row.TaskExamples, Tags: row.Tags,
		})
		if vector == nil {
			failed++
			continue
		}
		stored, err := q.StoreMissingEmbedding(ctx, gen.StoreMissingEmbeddingParams{Embedding: vector, SkillID: row.SkillID, WrittenAt: row.UpdatedAt})
		if err != nil {
			return done, failed, err
		}
		done += int(stored)
	}
	return done, failed, nil
}
