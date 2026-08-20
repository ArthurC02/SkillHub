package packaging

// PACK-005: the redistributable Test Cases and example data, in the shape of
// contracts/packaging/portable-test-case.schema.json.
//
// The same schema is read in both directions (04 丙-12): the packager writes it
// here, and the curation seeding script under tools/content/ will read it into a
// fresh deployment. Two schemas would become two formats, and the import
// direction is the one that does not exist yet.
//
// ONLY CURATED CONTENT TRAVELS. A user's uploaded Dataset bytes are never
// packaged and there is no checkbox to make them: a checkbox looks like
// respecting the user's choice, but it hands a licensing judgement to the person
// least equipped to make it, about files that may belong to their employer
// (packaging-design §5.1). `origin` in the contract is an enum with one value for
// exactly that reason — widening it is a contract change, not a flag.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
)

// PortableTestCaseSchemaVersion is the contract version the exporter writes.
const PortableTestCaseSchemaVersion = "1.0"

// Reasons a Test Case is left out (download-manifest.schema.json
// excluded_test_cases[].reason).
const (
	ExcludedUserUploadedDataset = "user_uploaded_dataset"
	ExcludedNotCurated          = "not_curated"
	ExcludedUserOptedOut        = "user_opted_out"
)

// IncludedTestCase and ExcludedTestCase are what the manifest and the preview
// both report. Listing the exclusions is what keeps an empty inclusion list from
// reading as "this Skill has no tests".
type IncludedTestCase struct {
	TestCaseID string `json:"-"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
}

type ExcludedTestCase struct {
	TestCaseID string `json:"-"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
}

// portableTestCase is one test-cases/<slug>/case.json.
type portableTestCase struct {
	SchemaVersion string              `json:"schema_version"`
	Slug          string              `json:"slug"`
	Name          string              `json:"name"`
	Origin        string              `json:"origin"`
	UserPrompt    string              `json:"user_prompt"`
	Criteria      []portableCriterion `json:"criteria"`
	Rubric        *testlab.Rubric     `json:"rubric,omitempty"`
	Datasets      []portableDataset   `json:"datasets"`
}

// portableCriterion carries id and text only. `source` and `confirmed_at` say
// who proposed a condition and when one particular user agreed to its wording —
// facts about one workspace, which do not survive the trip and are re-established
// on import.
type portableCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type portableDataset struct {
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

// selectTestCases decides which of a Skill's Test Cases travel, and builds the
// files for the ones that do.
//
// Curation is the criterion, and the platform's curated content lives in a
// catalog workspace (0010) — so "was this produced by platform curation" is a
// property of the workspace the packaging request is scoped to, not a flag
// somebody sets per row. A user packaging their own Skill therefore gets no test
// cases and is told why, rather than being offered an opt-in the platform cannot
// honour.
func (s *Service) selectTestCases(
	ctx context.Context, q *gen.Queries, ws gen.Workspace, skillID pgtype.UUID, include bool,
) (included []IncludedTestCase, excluded []ExcludedTestCase, files []exportFile, err error) {
	included, excluded, files = []IncludedTestCase{}, []ExcludedTestCase{}, nil

	rows, err := q.ListTestCasesForSkill(ctx, gen.ListTestCasesForSkillParams{
		SkillID: skillID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	for _, tc := range rows {
		datasets, err := q.ListDatasets(ctx, gen.ListDatasetsParams{
			TestCaseID: tc.ID, WorkspaceID: ws.ID,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		switch {
		case !include:
			excluded = append(excluded, ExcludedTestCase{
				TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedUserOptedOut,
			})
			continue
		case !ws.IsCatalog && len(datasets) > 0:
			// The more specific of the two refusals, and the one worth naming: it
			// says the obstacle is a licensing judgement about their files, not a
			// defect in their test case.
			excluded = append(excluded, ExcludedTestCase{
				TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedUserUploadedDataset,
			})
			continue
		case !ws.IsCatalog:
			excluded = append(excluded, ExcludedTestCase{
				TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedNotCurated,
			})
			continue
		}

		slug := testCaseSlug(tc.Name, pgconv.UUIDString(tc.ID))
		caseFiles, err := s.portableFiles(ctx, tc, datasets, slug)
		if err != nil {
			return nil, nil, nil, err
		}
		files = append(files, caseFiles...)
		included = append(included, IncludedTestCase{
			TestCaseID: pgconv.UUIDString(tc.ID), Slug: slug, Name: tc.Name,
		})
	}
	return included, excluded, files, nil
}

func (s *Service) portableFiles(
	ctx context.Context, tc gen.TestCase, datasets []gen.Dataset, slug string,
) ([]exportFile, error) {
	// testlab's decoder and not encoding/json here: the column is that package's
	// to read, and "written one way, read another" is exactly the drift the
	// exported decoders exist to prevent.
	criteria, err := testlab.DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("test case %s: %w", slug, err)
	}
	out := make([]portableCriterion, 0, len(criteria))
	for _, c := range criteria {
		out = append(out, portableCriterion{ID: c.ID, Text: c.Text})
	}
	rubric, err := testlab.DecodeRubric(tc.Rubric)
	if err != nil {
		return nil, fmt.Errorf("test case %s: %w", slug, err)
	}

	files := make([]exportFile, 0, len(datasets)+1)
	refs := make([]portableDataset, 0, len(datasets))
	for _, ds := range datasets {
		name := dataFileName(ds.FileName)
		if name == "" {
			// A stored file name that cannot be written as a direct child of
			// data/ is skipped rather than repaired: a path that had to be fixed
			// is not the path anybody recorded, and the user unpacks this to disk.
			continue
		}
		data, err := s.Store.Get(ctx, ds.ObjectKey)
		if err != nil {
			return nil, fmt.Errorf("test case %s dataset %s: %w", slug, name, err)
		}
		sum := sha256.Sum256(data)
		files = append(files, exportFile{path: testCasesDir + slug + "/data/" + name, data: data})
		refs = append(refs, portableDataset{
			File: "data/" + name, SHA256: hex.EncodeToString(sum[:]), ContentType: ds.ContentType,
		})
	}

	body, err := json.MarshalIndent(portableTestCase{
		SchemaVersion: PortableTestCaseSchemaVersion,
		Slug:          slug,
		Name:          tc.Name,
		Origin:        "curated",
		UserPrompt:    tc.UserPrompt,
		Criteria:      out,
		Rubric:        rubric,
		Datasets:      refs,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(files, exportFile{path: testCasesDir + slug + "/case.json", data: append(body, '\n')}), nil
}

// testCaseSlug is the stable directory name under test-cases/. Stable is the
// requirement: packaging the same Test Case twice has to produce the same slug,
// or manifest_hash would differ between two builds of unchanged content.
//
// The id suffix is what makes it stable AND unique — two test cases can share a
// name, and a name can be entirely non-ASCII, in which case the slug is the
// suffix alone.
func testCaseSlug(name, id string) string {
	var b strings.Builder
	lastHyphen := true
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLower(r) && r < unicode.MaxASCII, unicode.IsDigit(r) && r < unicode.MaxASCII:
			b.WriteRune(r)
			lastHyphen = false
		case !lastHyphen:
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	suffix := strings.ReplaceAll(id, "-", "")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	if slug == "" {
		return "test-case-" + suffix
	}
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug + "-" + suffix
}

// dataFileName reduces a stored file name to a direct child of data/, or ""
// when it cannot be one. The contract constrains this path because the user
// unpacks it to their own disk (portable-test-case.schema.json).
func dataFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || hasDriveLetter(name) {
		return ""
	}
	return name
}
