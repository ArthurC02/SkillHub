package testlab

// TEST-002: acceptance criteria the platform proposes, so a user does not face an
// empty box. Everything policy-shaped stays here — what is sent to the model,
// what is stored, how it is labelled — and the model only ever returns text
// (ADR-016 iron rule 6).

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// ErrSuggestUnavailable: the suggestion could not be produced. Never fatal to the
// user's work — 02:TEST-001 makes automatic suggestion 可選強化, so the manual
// path (AddCriterion) is always the fallback and the message says so.
var ErrSuggestUnavailable = errors.New("acceptance criteria suggestions are unavailable right now; add them manually")

const (
	// suggestTimeout bounds the internal call to the Python service. Go owns the
	// timeout and the cancellation, not Python (iron rule 7).
	suggestTimeout = 30 * time.Second
	// maxSkillSummaryBytes trims the skill description handed to the model. A
	// summary is a paragraph; anything longer is a document that wandered into the
	// column, and paying to send it buys nothing.
	maxSkillSummaryBytes = 4000
	// maxOutlineFields caps the columns described per file. A hundred-column sheet
	// does not produce a hundred useful criteria.
	maxOutlineFields = 40
	// datasetHeadBytes is how much of a file is read to find its header row. The
	// bytes are parsed in this process and never leave it; only the field NAMES do.
	datasetHeadBytes = 64 << 10
)

// SuggestCriteria proposes acceptance criteria for one draft and appends them
// with source = "suggested" (TEST-002).
//
// What travels to the model, and what does not:
//
//   - The skill's name and summary, and the user's own prompt. Both are the
//     user's or the catalogue's own descriptive text.
//   - For each attached file: its name, its content type, and the NAMES of its
//     columns with a type inferred from the first data row.
//   - Never a cell value, never a row, never the file itself. The inference reads
//     one row in this process and reports "number" or "text"; the value that led
//     to that word is dropped here and has no field to travel in (iron rule 11,
//     NFR-002). A dataset is the user's private data, and a suggestion feature is
//     not a reason to hand it to a model.
//
// Suggested criteria arrive unconfirmed. Confirming, editing (which re-labels the
// criterion as the user's own) and deleting are the existing TEST-003 operations.
func (s *Service) SuggestCriteria(ctx context.Context, ws gen.Workspace, id pgtype.UUID) (gen.TestCase, error) {
	if s.LLM == nil {
		return gen.TestCase{}, ErrSuggestUnavailable
	}
	q := gen.New(s.Pool)
	tc, err := q.GetTestCase(ctx, gen.GetTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	if err != nil {
		return gen.TestCase{}, err
	}
	skill, err := q.GetSkill(ctx, gen.GetSkillParams{ID: tc.SkillID, WorkspaceID: ws.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, err
	}
	datasets, err := q.ListDatasets(ctx, gen.ListDatasetsParams{TestCaseID: tc.ID, WorkspaceID: ws.ID})
	if err != nil {
		return gen.TestCase{}, err
	}

	req := llmclient.SuggestCriteriaRequest{
		SkillName:    skill.Name,
		SkillSummary: truncate(derefString(skill.Summary), maxSkillSummaryBytes),
		UserPrompt:   tc.UserPrompt,
		Datasets:     s.outlineDatasets(ctx, datasets),
	}

	callCtx, cancel := context.WithTimeout(ctx, suggestTimeout)
	defer cancel()
	resp, err := s.LLM.SuggestCriteria(callCtx, req)
	if err != nil {
		return gen.TestCase{}, fmt.Errorf("%w: %w", ErrSuggestUnavailable, err)
	}

	return s.mutateCriteria(ctx, ws, id, func(list []Criterion) ([]Criterion, error) {
		existing := make(map[string]bool, len(list))
		for _, c := range list {
			existing[c.Text] = true
		}
		for _, proposed := range resp.Criteria {
			if len(list) >= MaxCriteria {
				break
			}
			text, err := validateCriterion(proposed.Text)
			// A proposal that breaks an input rule is dropped, not surfaced as an
			// error: the rest of the batch is still useful, and the user never asked
			// for the model's formatting problem.
			if err != nil || existing[text] {
				continue
			}
			existing[text] = true
			list = append(list, Criterion{ID: newCriterionID(), Text: text, Source: SourceSuggested})
		}
		return list, nil
	})
}

// outlineDatasets describes each attached file by its shape. A file that cannot
// be read, or is not tabular text, is still listed by name and type: knowing a
// PDF is attached is useful to the model, and claiming to know its columns is not.
func (s *Service) outlineDatasets(ctx context.Context, rows []gen.Dataset) []llmclient.DatasetOutline {
	out := make([]llmclient.DatasetOutline, 0, len(rows))
	for _, d := range rows {
		outline := llmclient.DatasetOutline{FileName: d.FileName, ContentType: d.ContentType}
		if s.Store != nil && strings.HasPrefix(d.ContentType, "text/") {
			if data, err := s.Store.Get(ctx, d.ObjectKey); err == nil {
				outline.Fields = inferFields(data)
			}
		}
		out = append(out, outline)
	}
	return out
}

// inferFields reads the header row and one data row of a delimited text file and
// returns the column names with a coarse type.
//
// The data row is read here and dropped here. Nothing in the returned value can
// carry a cell value: DatasetField has a name and a type word and no third field.
//
// ponytail: CSV/TSV only. JSON and YAML datasets come back with no field list,
// which reads as "we could not describe the columns" and is true. Add a JSON key
// walk if suggestions on JSON datasets turn out to matter.
func inferFields(data []byte) []llmclient.DatasetField {
	if len(data) > datasetHeadBytes {
		data = data[:datasetHeadBytes]
	}
	text := string(data)
	firstLine, _, _ := strings.Cut(text, "\n")
	if firstLine == "" {
		return nil
	}
	comma := ','
	if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		comma = '\t'
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = comma
	// A truncated tail leaves a short last record; that is expected, not an error.
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil || len(header) < 2 {
		// One column is what any prose file looks like to a CSV reader. Reporting
		// "this file has a column called `Dear customer,`" would be worse than
		// reporting nothing.
		return nil
	}
	sample, err := r.Read()
	if err != nil && !errors.Is(err, io.EOF) {
		sample = nil
	}

	fields := make([]llmclient.DatasetField, 0, len(header))
	for i, name := range header {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		value := ""
		if i < len(sample) {
			value = sample[i]
		}
		fields = append(fields, llmclient.DatasetField{Name: name, InferredType: inferType(value)})
		if len(fields) == maxOutlineFields {
			break
		}
	}
	return fields
}

// inferType turns one cell into a type word. The cell itself goes no further.
func inferType(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return "unknown"
	case strings.EqualFold(value, "true"), strings.EqualFold(value, "false"):
		return "boolean"
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return "number"
	}
	return "text"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
