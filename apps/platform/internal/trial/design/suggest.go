package testlab

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var ErrSuggestUnavailable = errors.New("目前無法自動建議驗收條件，請自己手動輸入")

const (
	suggestTimeout = 40 * time.Second // budget-over: app.SUGGEST_CRITERIA_TIMEOUT_SECONDS

	maxSkillSummaryBytes = 4000

	maxOutlineFields = 40

	datasetHeadBytes = 64 << 10
)

type Suggestion struct {
	Text string `json:"text"`
}

func (s *Service) SuggestCriteria(ctx context.Context, ws identity.Workspace, id pgtype.UUID) ([]Suggestion, error) {
	if s.LLM == nil {
		return nil, ErrSuggestUnavailable
	}
	if s.ReadSkill == nil {
		return nil, errRegistryReadNotConfigured
	}
	q := gen.New(s.Pool)
	tc, err := q.GetTestCase(ctx, gen.GetTestCaseParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	skill, _, err := s.ReadSkill(ctx, ws.ID, tc.SkillID)
	if err != nil {
		return nil, err
	}

	datasets, err := q.ListDatasets(ctx, gen.ListDatasetsParams{TestCaseID: tc.ID, WorkspaceID: ws.ID})
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("%w: %w", ErrSuggestUnavailable, err)
	}

	current, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(current))
	for _, c := range current {
		seen[c.Text] = true
	}
	out := make([]Suggestion, 0, len(resp.Criteria))
	for _, proposed := range resp.Criteria {
		if len(out) >= MaxCriteria {
			break
		}
		text, err := validateCriterion(proposed.Text)

		if err != nil || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, Suggestion{Text: text})
	}
	return out, nil
}

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

	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil || len(header) < 2 {

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
