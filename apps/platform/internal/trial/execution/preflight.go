package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

var ErrPermissionsNotConfirmed = errors.New(
	"the pre-run permission summary must be confirmed before the run can start")

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)

	Remove(ctx context.Context, key string) error
}

var injectedSecretNames = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}

func injectedSecretsFor(snap policySnapshot) []string {
	for _, a := range snap.Egress.Allow {
		if a.Purpose == "model_gateway" {
			return injectedSecretNames
		}
	}
	return []string{}
}

type PermissionSummaryContent struct {
	SkillVersionID    string                `json:"skill_version_id"`
	SkillContentHash  string                `json:"skill_content_hash"`
	TestCaseID        string                `json:"test_case_id"`
	Datasets          []testlab.DatasetFile `json:"datasets"`
	DatasetTotalBytes int64                 `json:"dataset_total_bytes"`
	Scripts           ScriptSummary         `json:"scripts"`
	Tools             []string              `json:"tools"`
	MCPServers        []string              `json:"mcp_servers"`
	Network           NetworkSummary        `json:"network"`
	InjectedSecrets   []string              `json:"injected_secrets"`
	Provider          ProviderSummary       `json:"provider"`
	ResourceLimits    ResourceLimits        `json:"resource_limits"`
}

type ScriptSummary struct {
	Status   string   `json:"status"`
	Findings []string `json:"findings"`
}

type NetworkSummary struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow"`
}

func egressAllowLines(allow []egressAllow) []string {
	lines := make([]string, 0, len(allow))
	for _, a := range allow {
		lines = append(lines, a.Purpose+": "+a.URL)
	}
	return lines
}

type ProviderSummary struct {
	Name           string `json:"name"`
	IsolationLevel string `json:"isolation_level,omitempty"`
	Rootless       bool   `json:"rootless"`
	Runtime        string `json:"runtime,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`

	DetachedDescendantsSurvive bool `json:"detached_descendants_survive,omitempty"`
}

type PermissionSummary struct {
	Content       PermissionSummaryContent `json:"summary"`
	Hash          string                   `json:"summary_hash"`
	EstimatedCost CostEstimate             `json:"estimated_cost"`

	Quota *policy.QuotaView `json:"quota,omitempty"`
	Notes []string          `json:"notes"`
}

type CostEstimate struct {
	LowCredits     int64 `json:"low_credits"`
	TypicalCredits int64 `json:"typical_credits"`
	HighCredits    int64 `json:"high_credits"`

	Basis string `json:"basis"`
}

const (
	estimatedCostLowUSD     = 0.01
	estimatedCostTypicalUSD = 0.06
	estimatedCostHighUSD    = 0.30
)

const (
	baselineMedianUSD = 0.0566
	baselineMeanUSD   = 0.0702
	baselineMaxUSD    = 0.2367
)

func defaultCostEstimate(credits func(float64) (int64, bool)) (CostEstimate, error) {
	conv := func(usd float64) (int64, error) {
		c, ok := credits(usd)
		if !ok {
			return 0, fmt.Errorf("run: cost estimate cannot be expressed in credits (%v USD)", usd)
		}
		return c, nil
	}
	low, err := conv(estimatedCostLowUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	typical, err := conv(estimatedCostTypicalUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	high, err := conv(estimatedCostHighUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	median, err := conv(baselineMedianUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	mean, err := conv(baselineMeanUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	max, err := conv(baselineMaxUSD)
	if err != nil {
		return CostEstimate{}, err
	}
	return CostEstimate{
		LowCredits:     low,
		TypicalCredits: typical,
		HighCredits:    high,
		Basis: fmt.Sprintf(
			"估計值,非報價。來源:M2 基準試跑 45 個 Skill 各一次的閘道實付分布"+
				"(中位數 %d 點、平均 %d 點、最大 %d 點,mini 級模型)。"+
				"首次執行與重複執行因 prompt caching 可差約 8 倍,故為區間;"+
				"實際扣點以這個 Run 結算時的實付換算為準。",
			median, mean, max),
	}, nil
}

func (s *Service) store() ObjectStore { return s.Store }

func (s *Service) PermissionSummaryFor(
	ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID,
) (PermissionSummary, error) {
	if s.ReadVersion == nil {
		return PermissionSummary{}, errRegistryReadNotConfigured
	}
	if err := s.requireTestLab(); err != nil {
		return PermissionSummary{}, err
	}
	return s.permissionSummaryFor(ctx, workspaceID, skillID, versionID, testCaseID, nil)
}

// heldInputs carries rows a caller already read on its own transaction, so this
// function can skip taking a second pool connection while that transaction
// still holds one.
type heldInputs struct {
	draft   testlab.Draft
	version VersionFacts
}

func (s *Service) permissionSummaryFor(
	ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID, held *heldInputs,
) (PermissionSummary, error) {
	var version VersionFacts
	if held != nil {
		version = held.version
	} else {
		var (
			found bool
			err   error
		)
		version, found, err = s.ReadVersion(ctx, workspaceID, versionID)
		if !found && err == nil {

			return PermissionSummary{}, ErrPreflightTargetNotFound
		}
		if err != nil {
			return PermissionSummary{}, err
		}
	}

	if skillID.Valid && version.SkillID != skillID {
		return PermissionSummary{}, ErrNotFound
	}

	var draft testlab.Draft
	if held != nil {
		draft = held.draft
	} else {
		var err error
		draft, err = s.TestLab.ReadDraft(ctx, workspaceID, testCaseID)
		if errors.Is(err, testlab.ErrNotFound) {

			return PermissionSummary{}, ErrPreflightTargetNotFound
		}
		if err != nil {
			return PermissionSummary{}, err
		}
	}
	if skillID.Valid && draft.SkillID != skillID {
		return PermissionSummary{}, ErrNotFound
	}

	snap := defaultPolicy()

	content := PermissionSummaryContent{
		SkillVersionID:    pgconv.UUIDString(version.ID),
		SkillContentHash:  version.ContentHash,
		TestCaseID:        pgconv.UUIDString(draft.TestCaseID),
		Datasets:          draft.Datasets,
		DatasetTotalBytes: draft.DatasetTotalBytes,
		Scripts:           s.scriptSummary(ctx, version.PackageObjectKey),

		Tools: []string{"sandbox filesystem (/work, /out)", "sandbox shell"},

		MCPServers:      []string{},
		Network:         NetworkSummary{Mode: snap.Egress.Mode, Allow: egressAllowLines(snap.Egress.Allow)},
		InjectedSecrets: injectedSecretsFor(snap),
		Provider:        s.providerSummary(ctx, snap),
		ResourceLimits:  snap.ResourceLimits,
	}

	body, err := json.Marshal(content)
	if err != nil {
		return PermissionSummary{}, err
	}
	sum := sha256.Sum256(body)

	var quota *policy.QuotaView
	if held != nil {
		// Skipped here: a second pool read would deadlock a caller already
		// inside a transaction on a single-connection pool.
		quota = nil
	} else if state, enforced, err := s.QuotaFor(ctx, workspaceID); err != nil {
		slog.Warn("quota unavailable for the pre-run summary", "error", err)
	} else if enforced {
		view := state.View()
		quota = &view
	}

	if s.Credits == nil {

		return PermissionSummary{}, errors.New("run: no credit conversion wired; the pre-run screen cannot state a cost")
	}
	estimate, err := defaultCostEstimate(s.Credits)
	if err != nil {
		return PermissionSummary{}, err
	}
	return PermissionSummary{
		Content:       content,
		Hash:          hex.EncodeToString(sum[:]),
		EstimatedCost: estimate,
		Quota:         quota,
		Notes:         permissionSummaryNotes,
	}, nil
}

var permissionSummaryNotes = []string{
	"預估成本是區間估計值,不是報價;實際費用以模型閘道記錄的實付金額為準。",

	"Token 上限可跑的輪數取決於每輪的工具呼叫次數(每次工具結果都會重送整個前綴):純對話約 15 輪,每輪 1 次工具呼叫約 7.7 輪,每輪 2 次約 5 輪。",
	"MVP 不支援 MCP Server,因此工具清單只有 Sandbox 內建的檔案與 Shell 存取。",
	"Secrets 只顯示注入項目的名稱;實際值是每個 Run 專屬的短效憑證,不會出現在任何畫面、Log 或 Trace。",
	"網路為預設封鎖,允許清單為空表示 Sandbox 不能連出任何位址。",
	"以上任何一項變更(例如換一份 Dataset)都會產生新的摘要,必須重新確認才能開始 Run。",
}

func (s *Service) scriptSummary(ctx context.Context, objectKey string) ScriptSummary {
	report, ok := s.packageReport(ctx, objectKey)
	if !ok {
		return ScriptSummary{Status: "unavailable", Findings: []string{}}
	}
	findings := []string{}
	for _, f := range report.Findings {
		if f.Code == "script-file" || f.Code == "embedded-script" {
			findings = append(findings, f.Code+": "+f.Path)
		}
	}

	sort.Strings(findings)
	if len(findings) == 0 {
		return ScriptSummary{Status: "none", Findings: findings}
	}
	return ScriptSummary{Status: "present", Findings: findings}
}

func (s *Service) providerSummary(ctx context.Context, policy policySnapshot) ProviderSummary {
	registry := s.providers()
	if len(registry.Providers) == 0 {
		return ProviderSummary{Name: providerUnassigned}
	}
	p, capability, profile, err := registry.Select(ctx, requirementsFromPolicy(policy))
	if err != nil {
		return ProviderSummary{Name: providerUnassigned}
	}
	return ProviderSummary{
		Name:                       p.Name,
		IsolationLevel:             capability.Isolation.Level,
		Rootless:                   capability.Isolation.Rootless,
		Runtime:                    profile.Runtime,
		RuntimeVersion:             profile.RuntimeVersion,
		DetachedDescendantsSurvive: detachedDescendantsSurvive(capability),
	}
}

func detachedDescendantsSurvive(c ProviderCapability) bool {
	return c.Isolation.ReapsDetachedDescendants != nil && !*c.Isolation.ReapsDetachedDescendants
}

func (s *Service) ConfirmPermissions(
	ctx context.Context, workspaceID, actor, skillID, versionID, testCaseID pgtype.UUID, hash string,
) (gen.RunPermissionConfirmation, error) {
	summary, err := s.PermissionSummaryFor(ctx, workspaceID, skillID, versionID, testCaseID)
	if err != nil {
		return gen.RunPermissionConfirmation{}, err
	}
	if hash != summary.Hash {
		return gen.RunPermissionConfirmation{}, ErrPermissionsNotConfirmed
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.RunPermissionConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	row, err := q.ConfirmRunPermissions(ctx, gen.ConfirmRunPermissionsParams{
		WorkspaceID: workspaceID, SkillVersionID: versionID, TestCaseID: testCaseID,
		SummaryHash: hash, ConfirmedBy: actor,
	})
	if err != nil {
		return gen.RunPermissionConfirmation{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: workspaceID, Action: audit.ActionRunPermissionsConfirm,
		ResourceType: audit.ResourceTestCase, ResourceID: testCaseID,
		Metadata: map[string]any{"summary_hash": hash, "skill_version_id": pgconv.UUIDString(versionID)},
	}); err != nil {
		return gen.RunPermissionConfirmation{}, err
	}
	return row, tx.Commit(ctx)
}

func (s *Service) requirePermissionConfirmation(ctx context.Context, q *gen.Queries, p CreateParams, draft testlab.Draft, version VersionFacts) error {
	summary, err := s.permissionSummaryFor(ctx, p.WorkspaceID, p.SkillID, p.VersionID, p.TestCaseID,
		&heldInputs{draft: draft, version: version})
	if err != nil {
		return err
	}
	if p.ConfirmedSummaryHash != summary.Hash {
		return fmt.Errorf("%w: the permissions changed since it was confirmed", ErrPermissionsNotConfirmed)
	}
	_, err = q.GetRunPermissionConfirmation(ctx, gen.GetRunPermissionConfirmationParams{
		WorkspaceID: p.WorkspaceID, SkillVersionID: p.VersionID, TestCaseID: p.TestCaseID,
		SummaryHash: summary.Hash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPermissionsNotConfirmed
	}
	return err
}

func (h *Handler) Preflight(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, versionID, testCaseID, ok := preflightIDs(w, r, r.URL.Query().Get("version_id"), r.URL.Query().Get("test_case_id"))
	if !ok {
		return
	}
	summary, err := h.Svc.PermissionSummaryFor(r.Context(), ws.ID, skillID, versionID, testCaseID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPreflightTargetNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "permission summary failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, summary)
}

func (h *Handler) ConfirmPreflight(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var body struct {
		VersionID   string `json:"version_id"`
		TestCaseID  string `json:"test_case_id"`
		SummaryHash string `json:"summary_hash"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest,
			"body must be JSON with version_id, test_case_id and summary_hash")
		return
	}
	skillID, versionID, testCaseID, ok := preflightIDs(w, r, body.VersionID, body.TestCaseID)
	if !ok {
		return
	}
	row, err := h.Svc.ConfirmPermissions(r.Context(), ws.ID, user.ID, skillID, versionID, testCaseID, body.SummaryHash)
	switch {
	case errors.Is(err, ErrNotFound) || errors.Is(err, ErrPreflightTargetNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return

	case errors.Is(err, ErrPermissionsNotConfirmed):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"summary_hash does not match the current permission summary; read it again and confirm that")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "confirmation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"confirmed":    true,
		"summary_hash": row.SummaryHash,
		"confirmed_at": pgconv.RFC3339(row.ConfirmedAt),
	})
}

func preflightIDs(w http.ResponseWriter, r *http.Request, version, testCase string) (skillID, versionID, testCaseID pgtype.UUID, ok bool) {
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return skillID, versionID, testCaseID, false
	}
	if versionID.Scan(version) != nil || testCaseID.Scan(testCase) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "version_id and test_case_id must be UUIDs")
		return skillID, versionID, testCaseID, false
	}
	return skillID, versionID, testCaseID, true
}
