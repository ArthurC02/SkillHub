package packaging

// skillhub-manifest.json: the platform's account of how one package was produced
// (contracts/packaging/download-manifest.schema.json, ADR-027 decision 5).
//
// The consumer list has one member outside this repository — the user unzipping
// the package — so the JSON Schema is the contract and these structs are one
// implementation of it. Every level is closed there (`additionalProperties:
// false`), which is why nothing here has a catch-all field: adding one would be a
// contract change rather than an oversight, and the branch that matters is
// `improvement`, where the suggestion prose quotes a Run's private inputs.
//
// This manifest is not a signature and not an endorsement (ADR-027 decision 3).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// ManifestSchemaVersion is the contract version these structs write. Minor bumps
// are additive and an older manifest stays a valid instance of a newer minor.
const ManifestSchemaVersion = "1.0"

// unavailable is what a deleted lineage hop records. Omitting it is not an
// option: a gap reads as "there was no upstream", which is a different and false
// statement (packaging-design §4.2).
const unavailable = "unavailable"

// maxLineageHops bounds the provenance walk. A fork chain is a linked list of
// rows nothing stops from being long, and a cycle would only need one bad row.
// ponytail: flat cap; the last hop records `unavailable` if it is ever reached,
// which is the honest answer for "we stopped looking".
const maxLineageHops = 32

type Manifest struct {
	SchemaVersion     string             `json:"schema_version"`
	PackagedAt        string             `json:"packaged_at"`
	PackagerVersion   string             `json:"packager_version"`
	ProfileID         string             `json:"profile_id"`
	ProfileVersion    string             `json:"profile_version"`
	Source            ManifestSource     `json:"source"`
	License           ManifestLicense    `json:"license"`
	Validation        ManifestValidation `json:"validation"`
	Compatibility     Compatibility      `json:"compatibility"`
	IncludedTestCases []IncludedTestCase `json:"included_test_cases"`
	ExcludedTestCases []ExcludedTestCase `json:"excluded_test_cases"`
	ManifestHash      string             `json:"manifest_hash"`
}

type ManifestSource struct {
	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	VersionNumber  int32  `json:"version_number"`
	ContentHash    string `json:"content_hash"`
	// Origin is one of importOrigin / forkOrigin / improvementOrigin — three
	// mutually exclusive shapes, because in the data model PACK-003's "original
	// source and derivation" is not one field but three mutually exclusive paths.
	Origin any `json:"origin"`
}

type importOrigin struct {
	Kind        string  `json:"kind"`
	SourceType  string  `json:"source_type"`
	SourceURL   *string `json:"source_url"`
	SourceRef   *string `json:"source_ref"`
	FetchedAt   string  `json:"fetched_at"`
	ContentHash string  `json:"content_hash"`
}

type forkOrigin struct {
	Kind                   string `json:"kind"`
	UpstreamSkillID        string `json:"upstream_skill_id"`
	UpstreamSkillVersionID string `json:"upstream_skill_version_id"`
	// Chain runs from the immediate upstream to the oldest hop still known.
	// DISC-003 clause 5 asks for the ORIGINAL source, so one hop does not satisfy
	// it. Elements are lineageHop or the string "unavailable".
	Chain      []any `json:"chain"`
	RootSource any   `json:"root_source"`
}

type improvementOrigin struct {
	Kind         string          `json:"kind"`
	EvaluationID string          `json:"evaluation_id"`
	Suggestions  []suggestionRef `json:"suggestions"`
	Base         any             `json:"base"`
	RootSource   any             `json:"root_source"`
}

// suggestionRef is a suggestion's class and the file it targeted, and nothing
// else. `problem`, `proposed_content`, `expected_impact` and the evidence
// excerpt are model-written text quoting the private inputs of a Run; they are
// intentionally absent here and intentionally unrepresentable (iron rule 11).
type suggestionRef struct {
	Category   string `json:"category"`
	TargetPath string `json:"target_path"`
}

type lineageHop struct {
	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	VersionNumber  int32  `json:"version_number"`
}

type rootSource struct {
	SourceType string  `json:"source_type"`
	SourceURL  *string `json:"source_url"`
	SourceRef  *string `json:"source_ref"`
	FetchedAt  string  `json:"fetched_at"`
}

// ManifestLicense is the outward form of ADR-021's tiers. Expression and
// SourceTier are a PAIR and are never flattened: "MIT" declared by the author
// and "MIT" read off a repository root LICENSE are not the same claim. Both nil
// or both set — the same all-or-nothing the 0012 CHECK enforces in the database.
//
// This block reports licence evidence. It does not decide what may be packaged:
// that is `skills.redistribution`, and `license_status = Confirmed` is explicitly
// not a release condition (02:CONTENT-002).
type ManifestLicense struct {
	Expression  *string           `json:"expression"`
	SourceTier  *string           `json:"source_tier"`
	Disclosures []ManifestFinding `json:"disclosures"`
}

type ManifestValidation struct {
	Blocked  bool              `json:"blocked"`
	Errors   []ManifestFinding `json:"errors"`
	Warnings []ManifestFinding `json:"warnings"`
	Infos    []ManifestFinding `json:"infos"`
}

// ManifestFinding mirrors skillpkg.Finding minus its severity, which is implied
// by the list it sits in.
type ManifestFinding struct {
	Code    string   `json:"code"`
	Path    string   `json:"path,omitempty"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// Compatibility keeps ADR-012's three layers apart. Passing `format` is never a
// claim that the package installs or runs — that is what PACK-008 exists to
// prevent.
type Compatibility struct {
	Format     string `json:"format"`
	Capability string `json:"capability"`
	// Behaviour is a record of ONE measurement, never a promise. Its value may
	// come only from the measured row for this Skill Version on this Runtime
	// Image; with no such row it is `unverified` and it is never extrapolated
	// from another version or another image (04 乙-4).
	Behaviour    string `json:"behaviour"`
	RuntimeImage string `json:"runtime_image,omitempty"`
	MeasuredAt   string `json:"measured_at,omitempty"`
}

const unverified = "unverified"

func toManifestFindings(in []skillpkg.Finding) []ManifestFinding {
	out := make([]ManifestFinding, 0, len(in))
	for _, f := range in {
		out = append(out, ManifestFinding{Code: f.Code, Path: f.Path, Message: f.Message, Details: f.Details})
	}
	return out
}

// licenseDisclosures are the info findings the licence resolution raised.
// ADR-021 decision 5: every carry and every fallback is disclosed, never silent.
func licenseDisclosures(infos []skillpkg.Finding) []ManifestFinding {
	out := []ManifestFinding{}
	for _, f := range infos {
		if len(f.Code) >= 8 && f.Code[:8] == "license-" {
			out = append(out, ManifestFinding{Code: f.Code, Path: f.Path, Message: f.Message})
		}
	}
	return out
}

// compatibilityOf reads the measured row for this version, if there is one.
func compatibilityOf(ctx context.Context, q *gen.Queries, versionID pgtype.UUID) (Compatibility, error) {
	c := Compatibility{Format: "valid", Capability: unverified, Behaviour: unverified}
	row, err := q.GetSkillRuntimeCompatibility(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	c.Capability, c.Behaviour = row.Capability, row.Runtime
	// The image is required whenever either measured axis is not `unverified`: a
	// verdict without the image it was measured on IS the extrapolation the
	// contract forbids. Both axes unverified means there is nothing to attribute.
	if c.Capability != unverified || c.Behaviour != unverified {
		c.RuntimeImage = row.RuntimeImage
		if row.MeasuredAt.Valid {
			c.MeasuredAt = row.MeasuredAt.Time.UTC().Format(time.RFC3339)
		}
	}
	return c, nil
}

// originOf resolves PACK-003's three mutually exclusive provenance paths.
//
// Order matters and is not the order they are listed in the design. A version
// built from improvement suggestions goes through the ordinary version writer,
// so it HAS a skill_sources row of type `upload` — checking `source_id` first
// would report every improved version as a fresh import and lose the derivation
// DISC-003 asks for. So: the reverse lookup first, then the fork shape (whose
// marker is source_id being NULL), then the import.
func (s *Service) originOf(
	ctx context.Context, q *gen.Queries, ws gen.Workspace, skill gen.Skill, version gen.SkillVersion,
) (any, error) {
	sugs, err := q.ListSuggestionsAppliedToVersion(ctx, gen.ListSuggestionsAppliedToVersionParams{
		AppliedSkillVersionID: version.ID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return nil, err
	}
	if len(sugs) > 0 {
		return s.improvementOriginOf(ctx, q, ws, version, sugs)
	}

	if !version.SourceID.Valid && skill.ForkedFromVersionID.Valid {
		chain, root := walkLineage(ctx, q, skill.ForkedFromVersionID)
		return forkOrigin{
			Kind:                   "fork",
			UpstreamSkillID:        uuidString(skill.ForkedFromSkillID),
			UpstreamSkillVersionID: uuidString(skill.ForkedFromVersionID),
			Chain:                  chain,
			RootSource:             root,
		}, nil
	}

	if version.SourceID.Valid {
		src, err := q.GetLineageSource(ctx, version.SourceID)
		if err != nil {
			return nil, err
		}
		return importOrigin{
			Kind:        "import",
			SourceType:  src.SourceType,
			SourceURL:   src.SourceUrl,
			SourceRef:   src.SourceRef,
			FetchedAt:   rfc3339(src.FetchedAt),
			ContentHash: src.ContentHash,
		}, nil
	}
	// Neither a fork nor an import and nothing applied it: there is no honest
	// origin to write, and a manifest that omits one would claim this version
	// came from nowhere. Refusing is the correct answer to a row that should not
	// exist.
	return nil, fmt.Errorf("skill version %s has no recorded origin", uuidString(version.ID))
}

func (s *Service) improvementOriginOf(
	ctx context.Context, q *gen.Queries, ws gen.Workspace, version gen.SkillVersion,
	sugs []gen.ListSuggestionsAppliedToVersionRow,
) (any, error) {
	refs := make([]suggestionRef, 0, len(sugs))
	for _, sug := range sugs {
		refs = append(refs, suggestionRef{Category: sug.Category, TargetPath: sug.TargetPath})
	}
	var base any = unavailable
	prev, err := q.GetPreviousSkillVersion(ctx, gen.GetPreviousSkillVersionParams{
		SkillID: version.SkillID, WorkspaceID: ws.ID, VersionNumber: version.VersionNumber,
	})
	switch {
	case err == nil:
		base = lineageHop{
			SkillID:        uuidString(prev.SkillID),
			SkillVersionID: uuidString(prev.ID),
			VersionNumber:  prev.VersionNumber,
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	return improvementOrigin{
		Kind:         "improvement",
		EvaluationID: uuidString(sugs[0].EvaluationID),
		Suggestions:  refs,
		Base:         base,
		RootSource:   rootSourceOf(ctx, q, version.ID),
	}, nil
}

// walkLineage follows a fork chain from one version up to the oldest hop still
// readable, and resolves the import at the end of it.
//
// The reads it uses are not workspace scoped, deliberately: an upstream lives in
// another workspace by definition, and DISC-003 clause 5 asks for the original
// source rather than for the nearest one the caller happens to own (see
// db/queries/packaging.sql for what those two statements are bounded to).
func walkLineage(ctx context.Context, q *gen.Queries, from pgtype.UUID) (chain []any, root any) {
	chain = []any{}
	cur := from
	for i := 0; i < maxLineageHops; i++ {
		row, err := q.GetVersionLineage(ctx, cur)
		if err != nil {
			// Deleted or unreadable: recorded, not skipped.
			return append(chain, unavailable), unavailable
		}
		chain = append(chain, lineageHop{
			SkillID:        uuidString(row.SkillID),
			SkillVersionID: uuidString(row.ID),
			VersionNumber:  row.VersionNumber,
		})
		if !row.ForkedFromVersionID.Valid {
			return chain, rootSourceOf(ctx, q, cur)
		}
		cur = row.ForkedFromVersionID
	}
	return append(chain, unavailable), unavailable
}

// rootSourceOf finds the import a lineage started from: the oldest version of
// the skill, then up through each fork until one of them has a source row.
func rootSourceOf(ctx context.Context, q *gen.Queries, versionID pgtype.UUID) any {
	cur := versionID
	for i := 0; i < maxLineageHops; i++ {
		lineage, err := q.GetVersionLineage(ctx, cur)
		if err != nil {
			return unavailable
		}
		oldest, err := q.GetOldestSkillVersion(ctx, lineage.SkillID)
		if err != nil {
			return unavailable
		}
		if oldest.SourceID.Valid {
			src, err := q.GetLineageSource(ctx, oldest.SourceID)
			if err != nil {
				return unavailable
			}
			return rootSource{
				SourceType: src.SourceType, SourceURL: src.SourceUrl,
				SourceRef: src.SourceRef, FetchedAt: rfc3339(src.FetchedAt),
			}
		}
		if !lineage.ForkedFromVersionID.Valid {
			return unavailable
		}
		cur = lineage.ForkedFromVersionID
	}
	return unavailable
}

func rfc3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return time.Unix(0, 0).UTC().Format(time.RFC3339)
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
