package packaging

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const ManifestSchemaVersion = "1.2"

const unavailable = "unavailable"

const maxLineageHops = 32

type Manifest struct {
	SchemaVersion string `json:"schema_version"`

	PackagedAt             string             `json:"packaged_at"`
	SourceVersionCreatedAt string             `json:"source_version_created_at,omitempty"`
	PackagerVersion        string             `json:"packager_version"`
	ProfileID              string             `json:"profile_id"`
	ProfileVersion         string             `json:"profile_version"`
	Source                 ManifestSource     `json:"source"`
	License                ManifestLicense    `json:"license"`
	Validation             ManifestValidation `json:"validation"`
	Compatibility          Compatibility      `json:"compatibility"`
	IncludedTestCases      []IncludedTestCase `json:"included_test_cases"`
	ExcludedTestCases      []ExcludedTestCase `json:"excluded_test_cases"`

	ExcludedFiles []ExcludedFile `json:"excluded_files"`
	ManifestHash  string         `json:"manifest_hash"`
}

type ExcludedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Label  string `json:"label"`
	Note   string `json:"note"`

	ReferencedBySkillMD bool `json:"referenced_by_skill_md,omitempty"`
}

const (
	ReasonExcludedDir    = "excluded_dir"
	ReasonCredentialFile = "credential_file"
	ReasonNotRegularFile = "not_a_regular_file"
	ReasonUnsafePath     = "unsafe_path"
)

var excludedFileWords = map[string][2]string{
	ReasonExcludedDir: {"目錄不隨套件散布",
		"這個路徑在不隨套件走的目錄底下(.git、.github、node_modules、__pycache__、.venv、.tox、.mypy_cache、.aws、.azure、.docker、.kube、.ssh)。" +
			"要讓它進包,把檔案移到這些目錄之外。"},
	ReasonCredentialFile: {"看起來是憑證檔",
		"檔名屬於憑證清單(.env 與 .env.*、.git-credentials、.netrc、.npmrc、.pypirc 等),一律不散布。" +
			"要提供範本,改名為 .example／.sample／.template 結尾。"},
	ReasonNotRegularFile: {"不是一般檔案",
		"symlink、裝置或 fifo。解壓工具對它們的處理各不相同,而那不是一個套件可以替別人的機器決定的事。" +
			"要讓內容進包,用實體檔案取代連結。"},
	ReasonUnsafePath: {"路徑不安全",
		"這個 entry 名稱會逃出套件根目錄,或帶著磁碟機代號或反斜線。重新壓縮成相對路徑即可。"},
}

func (e ExcludedFile) withWords() ExcludedFile {
	if w, ok := excludedFileWords[e.Reason]; ok {
		e.Label, e.Note = w[0], w[1]
		return e
	}
	e.Label, e.Note = e.Reason, "這個平台版本沒有這個排除原因的說明,值照原樣顯示,不猜測它的意思。"
	return e
}

type ManifestSource struct {
	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	VersionNumber  int32  `json:"version_number"`
	ContentHash    string `json:"content_hash"`

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

type ManifestFinding struct {
	Code    string   `json:"code"`
	Path    string   `json:"path,omitempty"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type Compatibility struct {
	Format     string `json:"format"`
	Capability string `json:"capability"`

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

func licenseDisclosures(infos []skillpkg.Finding) []ManifestFinding {
	out := []ManifestFinding{}
	for _, f := range infos {
		if len(f.Code) >= 8 && f.Code[:8] == "license-" {
			out = append(out, ManifestFinding{Code: f.Code, Path: f.Path, Message: f.Message})
		}
	}
	return out
}

func (s *Service) compatibilityOf(ctx context.Context, versionID pgtype.UUID) (Compatibility, error) {
	c := Compatibility{Format: "valid", Capability: unverified, Behaviour: unverified}
	if s.ReadCompatibility == nil {
		return c, errOwnerReadNotConfigured
	}
	row, found, err := s.ReadCompatibility(ctx, versionID)
	if !found && err == nil {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	c.Capability, c.Behaviour = row.Capability, row.Runtime

	if c.Capability != unverified || c.Behaviour != unverified {
		c.RuntimeImage = row.RuntimeImage
		if row.MeasuredAt.Valid {
			c.MeasuredAt = row.MeasuredAt.Time.UTC().Format(time.RFC3339)
		}
	}
	return c, nil
}

func (s *Service) originOf(
	ctx context.Context, ws identity.Workspace, skill SkillFacts, version VersionFacts,
) (any, error) {
	if s.AppliedSuggestions == nil || s.SourceLineage == nil || s.ReadPrevious == nil ||
		s.ReadLineage == nil || s.ReadOldest == nil {
		return nil, errOwnerReadNotConfigured
	}
	sugs, err := s.AppliedSuggestions(ctx, version.ID, ws.ID)
	if err != nil {
		return nil, err
	}
	// Checked first: an improved version still carries the source_id of the
	// import it started from, so checking that before the origin below would
	// read it as a plain import.
	if len(sugs) > 0 {
		return s.improvementOriginOf(ctx, ws, version, sugs)
	}

	if !version.SourceID.Valid && skill.ForkedFromVersionID.Valid {
		chain, root := s.walkLineage(ctx, skill.ForkedFromVersionID)
		return forkOrigin{
			Kind:                   "fork",
			UpstreamSkillID:        pgconv.UUIDString(skill.ForkedFromSkillID),
			UpstreamSkillVersionID: pgconv.UUIDString(skill.ForkedFromVersionID),
			Chain:                  chain,
			RootSource:             root,
		}, nil
	}

	if version.SourceID.Valid {
		src, err := s.SourceLineage(ctx, version.SourceID)
		if err != nil {
			return nil, err
		}
		return importOrigin{
			Kind:        "import",
			SourceType:  src.SourceType,
			SourceURL:   src.SourceURL,
			SourceRef:   src.SourceRef,
			FetchedAt:   rfc3339(src.FetchedAt),
			ContentHash: src.ContentHash,
		}, nil
	}

	return nil, fmt.Errorf("skill version %s has no recorded origin", pgconv.UUIDString(version.ID))
}

func (s *Service) improvementOriginOf(
	ctx context.Context, ws identity.Workspace, version VersionFacts,
	sugs []AppliedSuggestion,
) (any, error) {
	refs := make([]suggestionRef, 0, len(sugs))
	for _, sug := range sugs {
		refs = append(refs, suggestionRef{Category: sug.Category, TargetPath: sug.TargetPath})
	}
	var base any = unavailable
	prev, found, err := s.ReadPrevious(ctx, ws.ID, version.SkillID, version.VersionNumber)
	switch {
	case err == nil && found:
		base = lineageHop{
			SkillID:        pgconv.UUIDString(prev.SkillID),
			SkillVersionID: pgconv.UUIDString(prev.ID),
			VersionNumber:  prev.VersionNumber,
		}
	case err != nil:
		return nil, err
	}
	return improvementOrigin{
		Kind:         "improvement",
		EvaluationID: pgconv.UUIDString(sugs[0].EvaluationID),
		Suggestions:  refs,
		Base:         base,
		RootSource:   s.rootSourceOf(ctx, version.ID),
	}, nil
}

func (s *Service) walkLineage(ctx context.Context, from pgtype.UUID) (chain []any, root any) {
	chain = []any{}
	cur := from
	for i := 0; i < maxLineageHops; i++ {
		row, found, err := s.ReadLineage(ctx, cur)
		if err != nil || !found {

			return append(chain, unavailable), unavailable
		}
		chain = append(chain, lineageHop{
			SkillID:        pgconv.UUIDString(row.SkillID),
			SkillVersionID: pgconv.UUIDString(row.ID),
			VersionNumber:  row.VersionNumber,
		})
		if !row.ForkedFromVersionID.Valid {
			return chain, s.rootSourceOf(ctx, cur)
		}
		cur = row.ForkedFromVersionID
	}
	return append(chain, unavailable), unavailable
}

func (s *Service) rootSourceOf(ctx context.Context, versionID pgtype.UUID) any {
	cur := versionID
	for i := 0; i < maxLineageHops; i++ {
		lineage, found, err := s.ReadLineage(ctx, cur)
		if err != nil || !found {
			return unavailable
		}
		oldest, found, err := s.ReadOldest(ctx, lineage.SkillID)
		if err != nil || !found {
			return unavailable
		}
		if oldest.SourceID.Valid {
			src, err := s.SourceLineage(ctx, oldest.SourceID)
			if err != nil {
				return unavailable
			}
			return rootSource{
				SourceType: src.SourceType, SourceURL: src.SourceURL,
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
