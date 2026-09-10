package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const PortableTestCaseSchemaVersion = "1.0"

const (
	ExcludedUserUploadedDataset = "user_uploaded_dataset"
	ExcludedNotCurated          = "not_curated"
	ExcludedUserOptedOut        = "user_opted_out"

	ExcludedUnsafeDatasetFileName = "unsafe_dataset_file_name"
)

type IncludedTestCase struct {
	TestCaseID string `json:"-"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
}

type ExcludedTestCase struct {
	TestCaseID string `json:"-"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
	Label      string `json:"label"`
	Note       string `json:"note"`
}

var excludedCaseWords = map[string][2]string{
	ExcludedUserUploadedDataset: {"含你上傳的資料集",
		"你上傳的資料不能隨套件散布（授權未定），這個 Test Case 因此不打包。"},
	ExcludedNotCurated: {"未經策展",
		"只有平台策展的 Test Case 會隨套件散布，你自己的 Test Case 留在工作區。"},
	ExcludedUserOptedOut: {"你選擇不包含",
		"打包選項沒有勾選散布 Test Case。"},
	ExcludedUnsafeDatasetFileName: {"資料集檔名無法安全寫入",
		"有一個資料集的檔名不能原樣放進 data/，平台不改名，所以整個 Test Case 不打包。"},
}

func (e ExcludedTestCase) withWords() ExcludedTestCase {
	if w, ok := excludedCaseWords[e.Reason]; ok {
		e.Label, e.Note = w[0], w[1]
		return e
	}
	e.Label, e.Note = e.Reason, "這個平台版本沒有這個排除原因的說明，值照原樣顯示，不猜測它的意思。"
	return e
}

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

type portableCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type portableDataset struct {
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

type caseSource struct {
	workspaceID pgtype.UUID
	skillID     pgtype.UUID
	curated     bool
}

func (s *Service) selectTestCases(
	ctx context.Context, ws identity.Workspace, skill SkillFacts, include bool,
) (included []IncludedTestCase, excluded []ExcludedTestCase, files []exportFile, err error) {
	included, excluded, files = []IncludedTestCase{}, []ExcludedTestCase{}, nil

	sources := []caseSource{{workspaceID: ws.ID, skillID: skill.ID, curated: ws.IsCatalog}}
	if skill.ForkedFromSkillID.Valid {
		src, found, err := s.CuratedSource(ctx, skill.ForkedFromSkillID)
		if err != nil {
			return nil, nil, nil, err
		}

		if found {
			sources = append(sources, caseSource{
				workspaceID: src.WorkspaceID, skillID: src.SkillID, curated: true,
			})
		}
	}

	for _, src := range sources {
		rows, err := s.TestLab.CasesForSkill(ctx, src.workspaceID, src.skillID)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, tc := range rows {
			datasets, err := s.TestLab.CaseDatasets(ctx, src.workspaceID, tc.ID)
			if err != nil {
				return nil, nil, nil, err
			}
			switch {
			case !include:
				excluded = append(excluded, ExcludedTestCase{
					TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedUserOptedOut,
				}.withWords())
				continue
			case !src.curated && len(datasets) > 0:

				excluded = append(excluded, ExcludedTestCase{
					TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedUserUploadedDataset,
				}.withWords())
				continue
			case !src.curated:
				excluded = append(excluded, ExcludedTestCase{
					TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name, Reason: ExcludedNotCurated,
				}.withWords())
				continue
			case unsafeDatasetName(datasets):
				excluded = append(excluded, ExcludedTestCase{
					TestCaseID: pgconv.UUIDString(tc.ID), Name: tc.Name,
					Reason: ExcludedUnsafeDatasetFileName,
				}.withWords())
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
	}
	return included, excluded, files, nil
}

func (s *Service) portableFiles(
	ctx context.Context, tc testlab.Case, datasets []testlab.Dataset, slug string,
) ([]exportFile, error) {

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

			return nil, fmt.Errorf("test case %s: dataset file name %q cannot be packaged", slug, ds.FileName)
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

func unsafeDatasetName(datasets []testlab.Dataset) bool {
	for _, ds := range datasets {
		if dataFileName(ds.FileName) == "" {
			return true
		}
	}
	return false
}

func dataFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || hasDriveLetter(name) {
		return ""
	}
	return name
}
