package mcptools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

const ReferencePrefix = "mcp__"

const referenceSeparator = "__"

var (
	ErrShape    = errors.New("ingest: invalid MCP tool")
	ErrConflict = errors.New("ingest: MCP tool already listed")
	ErrNotFound = errors.New("ingest: MCP tool not found")

	serverName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolName   = regexp.MustCompile(`^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$`)
	serverURL  = regexp.MustCompile(`^https://[^\s/?#]+`)
)

type Tool struct {
	ID           string
	Server       string
	Tool         string
	Purpose      string
	Parameters   string
	ServerURL    string
	Reason       string
	CreatedAt    time.Time
	RetiredAt    *time.Time
	RetireReason string
}

func (t Tool) Reference() string {
	return ReferencePrefix + t.Server + referenceSeparator + t.Tool
}

type Draft struct {
	Server     string
	Tool       string
	Purpose    string
	Parameters string
	ServerURL  string
	Note       string
}

type Service struct {
	Pool *pgxpool.Pool
}

func (s *Service) List(ctx context.Context) ([]Tool, error) {
	rows, err := gen.New(s.Pool).ListMCPTools(ctx)
	return toolsOf(rows), err
}

func Active(ctx context.Context, db gen.DBTX) ([]Tool, error) {
	rows, err := gen.New(db).ListActiveMCPTools(ctx)
	return toolsOf(rows), err
}

func (s *Service) Add(ctx context.Context, operator pgtype.UUID, d Draft) (Tool, error) {
	d, err := validDraft(d)
	if err != nil {
		return Tool{}, err
	}
	var added gen.McpTool
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		added, err = gen.New(tx).InsertMCPTool(ctx, gen.InsertMCPToolParams{
			Server: d.Server, Tool: d.Tool, Purpose: d.Purpose, Parameters: d.Parameters, ServerUrl: d.ServerURL, Reason: d.Note,
		})
		if err != nil {
			return err
		}
		return logChange(ctx, tx, operator, audit.ActionMCPToolAdd, added)
	})
	if pgconv.IsUniqueViolation(err) {
		return Tool{}, fmt.Errorf("%w: %s 已經在清單上，要換掉請先退役舊的那一筆", ErrConflict, ReferencePrefix+d.Server+referenceSeparator+d.Tool)
	}
	return toolOf(added), err
}

func (s *Service) Retire(ctx context.Context, operator, id pgtype.UUID, note string) (Tool, error) {
	note = strings.TrimSpace(note)
	if note == "" || utf8.RuneCountInString(note) > limits.MCPToolRetirementNoteMaxLength {
		return Tool{}, fmt.Errorf("%w: 退役理由要 1 到 %d 個字", ErrShape, limits.MCPToolRetirementNoteMaxLength)
	}
	var retired gen.McpTool
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		retired, err = gen.New(tx).RetireMCPTool(ctx, gen.RetireMCPToolParams{ID: id, RetireReason: &note})
		if err != nil {
			return err
		}
		return logChange(ctx, tx, operator, audit.ActionMCPToolRetire, retired)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Tool{}, ErrNotFound
	}
	return toolOf(retired), err
}

func Unlisted(declared []string, active []Tool) []string {
	offered := make(map[string]bool, len(active))
	for _, t := range active {
		offered[t.Reference()] = true
	}
	var unlisted []string
	for _, name := range declared {
		if strings.HasPrefix(name, ReferencePrefix) && !offered[name] {
			unlisted = append(unlisted, name)
		}
	}
	return unlisted
}

func validDraft(d Draft) (Draft, error) {
	d = Draft{
		Server: strings.TrimSpace(d.Server), Tool: strings.TrimSpace(d.Tool), Purpose: strings.TrimSpace(d.Purpose),
		Parameters: strings.TrimSpace(d.Parameters), ServerURL: strings.TrimSpace(d.ServerURL), Note: strings.TrimSpace(d.Note),
	}
	for _, f := range []struct {
		value, label string
		max          int
		form         *regexp.Regexp
	}{
		{d.Server, "伺服器名稱（小寫英數，以單一 - 連接）", limits.MCPToolDraftServerMaxLength, serverName},
		{d.Tool, "工具名稱（英數，以單一 _ 或 - 連接）", limits.MCPToolDraftToolMaxLength, toolName},
		{d.Purpose, "用途", limits.MCPToolDraftPurposeMaxLength, nil},
		{d.ServerURL, "伺服器位址（https://）", limits.MCPToolDraftServerUrlMaxLength, serverURL},
		{d.Note, "新增理由", limits.MCPToolDraftNoteMaxLength, nil},
	} {
		if f.value == "" || utf8.RuneCountInString(f.value) > f.max || (f.form != nil && !f.form.MatchString(f.value)) {
			return d, fmt.Errorf("%w: %s要 1 到 %d 個字並符合格式", ErrShape, f.label, f.max)
		}
	}
	if utf8.RuneCountInString(d.Parameters) > limits.MCPToolDraftParametersMaxLength {
		return d, fmt.Errorf("%w: 參數摘要最多 %d 個字", ErrShape, limits.MCPToolDraftParametersMaxLength)
	}
	return d, nil
}

func logChange(ctx context.Context, tx pgx.Tx, operator pgtype.UUID, action string, t gen.McpTool) error {
	reason := t.Reason
	if t.RetireReason != nil {
		reason = *t.RetireReason
	}
	return audit.Log(ctx, tx, audit.Event{
		Actor: operator, Action: action, ResourceType: audit.ResourceMCPTool, ResourceID: t.ID,
		Metadata: map[string]any{
			"scope": audit.ScopeOperator, "reference": toolOf(t).Reference(), "server_url": t.ServerUrl, "reason": reason,
		},
	})
}

func toolsOf(rows []gen.McpTool) []Tool {
	out := make([]Tool, 0, len(rows))
	for _, r := range rows {
		out = append(out, toolOf(r))
	}
	return out
}

func toolOf(r gen.McpTool) Tool {
	t := Tool{
		ID: pgconv.UUIDString(r.ID), Server: r.Server, Tool: r.Tool, Purpose: r.Purpose, Parameters: r.Parameters,
		ServerURL: r.ServerUrl, Reason: r.Reason, CreatedAt: r.CreatedAt.Time,
	}
	if r.RetiredAt.Valid {
		at := r.RetiredAt.Time
		t.RetiredAt = &at
	}
	if r.RetireReason != nil {
		t.RetireReason = *r.RetireReason
	}
	return t
}
