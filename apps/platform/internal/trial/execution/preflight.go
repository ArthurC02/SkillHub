package run

// 02:TEST-005 / 03:TEST-008,009: what a Run is allowed to touch, shown before it
// starts, agreed to by the user, and re-agreed to whenever it changes.
//
// The summary is built here rather than in internal/testlab because half of what
// it discloses is run policy — provider, egress, resource ceilings, injected
// secrets — and internal/run already owns those. The other half is the test case
// and its files, which this reads through testlab.ReadDraft: that package owns
// them, and a second reader of test_cases and datasets is a second definition of
// what a draft is (ADR-032).

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
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

// ErrPermissionsNotConfirmed is SEC-002 gate B: the run request carries no
// agreement to the permission summary, or carries one for a summary that has
// since changed. Both block, and both are answered the same way — the fix in
// either case is to read the current summary and confirm it.
var ErrPermissionsNotConfirmed = errors.New(
	"the pre-run permission summary must be confirmed before the run can start")

// ObjectStore is the slice of object storage the summary needs: the stored
// package, so "does this skill carry a script" is answered by scanning the exact
// bytes that will be executed rather than by a projected flag. Nothing here
// executes any of it (iron rule 1).
//
// The two Presign methods are SBX-008's other half: the short-lived, per-path
// authorizations a dispatch hands the execution plane. They are here rather than
// in a second interface because one deployment credential backs both, and a
// deployment that can read a package can always sign a URL for it.
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Remove is what makes the owner's delete of a Run output actually delete
	// (02:SEC-006 1). Without it the row would stop being visible while the file
	// stayed, which is the half of "deleted" that matters least.
	Remove(ctx context.Context, key string) error
}

// Secrets the platform injects into every sandbox (SBX-008, PDM-003). Names only:
// the value is a per-run Virtual Key minted at dispatch and never leaves the
// gateway boundary, and iron rule 11 keeps it out of anything a user can read.
var injectedSecretNames = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}

// PermissionSummaryContent is the hashed body. A struct, not a map, so field
// order — and therefore the hash — is fixed by the type and not by the encoder
// (same reason as snapshotContent in internal/testlab).
//
// What is deliberately *not* in here: the user prompt and the acceptance
// criteria. Editing either changes what the run is asked to do, not what it is
// allowed to touch, and invalidating a permission confirmation over a typo fix
// would train users to click through the one screen that must not be reflexive.
// Datasets is testlab's own type and not a local copy: the files a run may read
// are the draft's files, and two structs describing them is how the two sides
// end up disagreeing about which fields the user was shown. The JSON is
// unchanged by that move, which matters — every outstanding confirmation is a
// hash over these bytes.
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

// ScriptSummary answers "does the package carry runnable code" from the stored
// package itself. Status is one of:
//
//	none        — scanned, no script file and no embedded code
//	present     — scanned, and Findings names what was found
//	unavailable — the package could not be read, so nothing is claimed either way
//
// `unavailable` is never rendered as `none`: an unreadable package is not a clean
// one (DISC-004 不得自行推定為通過).
type ScriptSummary struct {
	Status   string   `json:"status"`
	Findings []string `json:"findings"`
}

// NetworkSummary is the egress policy the sandbox will be held to. Allow is one
// "purpose: url" line per permitted destination (see egressAllowLines).
type NetworkSummary struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow"`
}

// egressAllowLines renders the allow list as text a user can read, and that the
// hash can depend on. `%v` on the struct was the earlier version: invisible while
// the list is empty, and the moment SBX-005/006 mints a grant it would have put
// `{model_gateway http://...}` on the screen and into the confirmed hash.
func egressAllowLines(allow []egressAllow) []string {
	lines := make([]string, 0, len(allow))
	for _, a := range allow {
		lines = append(lines, a.Purpose+": "+a.URL)
	}
	return lines
}

// ProviderSummary is who will run the workload and how strongly it is isolated.
type ProviderSummary struct {
	Name           string `json:"name"`
	IsolationLevel string `json:"isolation_level,omitempty"`
	Rootless       bool   `json:"rootless"`
	Runtime        string `json:"runtime,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`
}

// PermissionSummary is the whole answer: the hashed body, its hash, and the
// display-only material that explains it. Notes and the cost estimate are outside
// the hash on purpose — rewording a sentence, or recalibrating an estimate against
// a newer sample, must not invalidate every outstanding confirmation.
type PermissionSummary struct {
	Content       PermissionSummaryContent `json:"summary"`
	Hash          string                   `json:"summary_hash"`
	EstimatedCost CostEstimate             `json:"estimated_cost"`
	// Quota is what the account has left (PDM-010), on the screen where a user
	// decides to start a run — the same kind of fact estimated_cost is, and outside
	// the hash for the same reason (TEST-011): the hash covers what the run may
	// touch, and an allowance is a state, not a permission. Another run finishing
	// elsewhere in the workspace must not invalidate a confirmation in flight.
	//
	// Absent when this deployment enforces no allowance. Absent and not zeroed: a
	// number here is a claim that the platform applies it, and putting up one it
	// does not apply is exactly 04 乙-2.
	Quota *policy.QuotaView `json:"quota,omitempty"`
	Notes []string          `json:"notes"`
}

// CostEstimate is PDM-005 §5.3's "預估成本區間", and §5.2a-6 is why it is a range
// and not a number: prompt caching makes a first run and a repeat run of the same
// skill differ by roughly 8x, so a single figure would be wrong for one of them
// every time.
//
// It is NOT part of the hashed content, by the same rule that keeps the user
// prompt out: the hash covers what the run is *allowed to touch*, and this is a
// prediction about what it will cost. Recalibrating it against a larger sample
// must not silently revoke every confirmation a user has outstanding, and a
// prediction changing is not a permission changing.
type CostEstimate struct {
	// Currency is fixed at USD: the gateway prices in it and the baseline was
	// measured in it. A converted number would be an exchange rate the platform
	// does not own, presented as a fact about a run.
	Currency   string  `json:"currency"`
	LowUSD     float64 `json:"low"`
	TypicalUSD float64 `json:"typical"`
	HighUSD    float64 `json:"high"`
	// Basis says where the numbers came from, so nobody reads them as a quote.
	Basis string `json:"basis"`
}

// Measured, not modelled: the M2 baseline ran all 45 catalogue skills once each
// through this exact path (mini tier, real sandbox, real gateway) and the gateway's
// own per-key spend gave the distribution — median $0.0566, mean $0.0702, max
// $0.2367 (docs/plans/mvp/m2/content-baseline-report.md §5.2).
//
// The published range is deliberately wider than that sample on both ends. The low
// end is where a cache-warm repeat of a small skill lands; the high end is rounded
// up from the observed maximum, because a sample of 45 is not a bound. What it is
// not is the per-Run budget ceiling: that is ResourceLimits' territory and the
// gateway's max_budget, and quoting the ceiling as the estimate would tell every
// user their run costs half a dollar when the median is six cents.
//
// ponytail: three constants, not a query over historical runs. A live percentile
// per skill is a real improvement and needs EVAL-012's cost comparison to exist
// first; until then a stated, sourced, honestly-labelled estimate beats a
// statistic computed from too little data and presented as if it were more.
const (
	estimatedCostLowUSD     = 0.01
	estimatedCostTypicalUSD = 0.06
	estimatedCostHighUSD    = 0.30
)

func defaultCostEstimate() CostEstimate {
	return CostEstimate{
		Currency:   "USD",
		LowUSD:     estimatedCostLowUSD,
		TypicalUSD: estimatedCostTypicalUSD,
		HighUSD:    estimatedCostHighUSD,
		Basis: "估計值,非報價。來源:M2 基準試跑 45 個 Skill 各一次的閘道實付分布" +
			"(中位數 $0.0566、平均 $0.0702、最大 $0.2367,mini 級模型)。" +
			"首次執行與重複執行因 prompt caching 可差約 8 倍,故為區間;" +
			"實際費用以閘道每把金鑰的 spend 為準。",
	}
}

func (s *Service) store() ObjectStore { return s.Store }

// PermissionSummaryFor builds the pre-run summary for one (version, test case)
// pair. Every read is workspace scoped from the session's workspace (iron rule 3);
// a version or draft outside it is ErrNotFound, the same answer as one that does
// not exist.
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

func (s *Service) permissionSummaryFor(
	ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID, locked *testlab.Draft,
) (PermissionSummary, error) {
	version, found, err := s.ReadVersion(ctx, workspaceID, versionID)
	if !found && err == nil {
		return PermissionSummary{}, ErrNotFound
	}
	if err != nil {
		return PermissionSummary{}, err
	}
	// Same guard as Create: the version must belong to the skill in the URL, or a
	// summary could be produced for a pairing the run itself would refuse.
	if skillID.Valid && version.SkillID != skillID {
		return PermissionSummary{}, ErrNotFound
	}
	// Reads the draft rather than a snapshot: the summary describes the run the
	// user is about to start, and the snapshot only exists once it has started.
	// The dataset order — and therefore the hash over it — is testlab's guarantee,
	// stated once at ReadDraft rather than restated here.
	var draft testlab.Draft
	if locked != nil {
		draft = *locked
	} else {
		var err error
		draft, err = s.TestLab.ReadDraft(ctx, workspaceID, testCaseID)
		if errors.Is(err, testlab.ErrNotFound) {
			return PermissionSummary{}, ErrNotFound
		}
		if err != nil {
			return PermissionSummary{}, err
		}
	}
	if skillID.Valid && draft.SkillID != skillID {
		return PermissionSummary{}, ErrNotFound
	}

	// The same policy Create freezes onto the run and the scheduler matches
	// against, read from its one definition. Rebuilding the literal here is what
	// would let a user confirm a summary of yesterday's policy and still pass the
	// hash check, which is the one failure this whole screen exists to prevent.
	// Named snap, not policy: internal/policy is the Policy & Usage context and
	// this is a run policy snapshot — two different things that used to want the
	// same identifier.
	snap := defaultPolicy()

	content := PermissionSummaryContent{
		SkillVersionID:    pgconv.UUIDString(version.ID),
		SkillContentHash:  version.ContentHash,
		TestCaseID:        pgconv.UUIDString(draft.TestCaseID),
		Datasets:          draft.Datasets,
		DatasetTotalBytes: draft.DatasetTotalBytes,
		Scripts:           s.scriptSummary(ctx, version.PackageObjectKey),
		// The agent's own file and shell tools, inside the sandbox and nowhere
		// else. There is no per-tool grant to show because there is no per-tool
		// grant to make: the isolation boundary is the container, not a tool list.
		Tools: []string{"sandbox filesystem (/work, /out)", "sandbox shell"},
		// MVP has no MCP at all (AGENTS.md 範圍注意, 02:TEST-003 後 MVP). An empty
		// list shown as empty is the honest disclosure; omitting the row would let
		// a user assume the question was never asked.
		MCPServers:      []string{},
		Network:         NetworkSummary{Mode: snap.Egress.Mode, Allow: egressAllowLines(snap.Egress.Allow)},
		InjectedSecrets: injectedSecretNames,
		Provider:        s.providerSummary(ctx, snap),
		ResourceLimits:  snap.ResourceLimits,
	}

	body, err := json.Marshal(content)
	if err != nil {
		return PermissionSummary{}, err
	}
	sum := sha256.Sum256(body)

	// Read after the hash is taken, so it is structurally impossible for the
	// allowance to reach the hashed body (TEST-011's rule for estimated_cost). A
	// failure here does not fail the screen: the summary's job is to say what the
	// run may touch, and that answer does not depend on how many runs are left.
	var quota *policy.QuotaView
	if state, enforced, err := s.QuotaFor(ctx, workspaceID); err != nil {
		slog.Warn("quota unavailable for the pre-run summary", "error", err)
	} else if enforced {
		view := state.View()
		quota = &view
	}

	return PermissionSummary{
		Content:       content,
		Hash:          hex.EncodeToString(sum[:]),
		EstimatedCost: defaultCostEstimate(),
		Quota:         quota,
		Notes: []string{
			"預估成本是區間估計值,不是報價;實際費用以模型閘道記錄的實付金額為準。",
			"MVP 不支援 MCP Server,因此工具清單只有 Sandbox 內建的檔案與 Shell 存取。",
			"Secrets 只顯示注入項目的名稱;實際值是每個 Run 專屬的短效憑證,不會出現在任何畫面、Log 或 Trace。",
			"網路為預設封鎖,允許清單為空表示 Sandbox 不能連出任何位址。",
			"以上任何一項變更(例如換一份 Dataset)都會產生新的摘要,必須重新確認才能開始 Run。",
		},
	}, nil
}

// scriptSummary re-scans the stored package. A package that cannot be read does
// not fail the request: the summary says the scan is unavailable, which is a
// different statement from "no scripts" and must stay one.
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
	// Findings arrive in walk order, which is stable for a given package, but
	// sorting costs nothing and removes the question entirely.
	sort.Strings(findings)
	if len(findings) == 0 {
		return ScriptSummary{Status: "none", Findings: findings}
	}
	return ScriptSummary{Status: "present", Findings: findings}
}

// providerSummary names the provider this run would go to. Best effort: an empty
// fleet, or one that is unreachable right now, reports `unassigned` rather than
// failing the summary — refusing incompatible work is checkSchedulable's job, on
// the run request itself.
//
// ponytail: the provider identity is inside the hash, so a provider flapping in
// and out of reachability between the summary and the run costs the user one
// re-confirmation. That is the safe direction (who runs your code is a permission
// fact) and rare with a single-provider fleet; revisit if multi-provider fleets
// make it noisy.
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
		Name:           p.Name,
		IsolationLevel: capability.Isolation.Level,
		Rootless:       capability.Isolation.Rootless,
		Runtime:        profile.Runtime,
		RuntimeVersion: profile.RuntimeVersion,
	}
}

// ConfirmPermissions records the user's agreement to the summary they were shown.
// The hash they send is checked against a freshly built summary, so a client
// cannot confirm a hash it invented or one that has already gone stale.
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

// requirePermissionConfirmation is SEC-002 gate B, called from Create before a run
// row exists. It rebuilds the summary from what is true *now* and requires both
// that the caller echoed that exact hash and that an agreement to it is on record.
//
// Rebuilding rather than trusting the request is the whole mechanism: a dataset
// added after the confirmation changes the hash here, the old agreement no longer
// matches, and the run is refused until the user has seen the new summary.
func (s *Service) requirePermissionConfirmation(ctx context.Context, q *gen.Queries, p CreateParams, draft testlab.Draft) error {
	summary, err := s.permissionSummaryFor(ctx, p.WorkspaceID, p.SkillID, p.VersionID, p.TestCaseID, &draft)
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

// --- HTTP ------------------------------------------------------------------

// Preflight handles GET /skills/{id}/runs/preflight?version_id=&test_case_id=
// (03:TEST-008).
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
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "permission summary failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, summary)
}

// ConfirmPreflight handles POST /skills/{id}/runs/preflight/confirm (03:TEST-009).
// A user who declines simply does not call it, and without a record here the run
// cannot start.
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
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	// 422 rather than 409: the request is well formed, but it agrees to a summary
	// that is not the current one, so the client has to re-read and re-confirm.
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

// preflightIDs parses the three ids these two routes share. A malformed id is
// answered like one that belongs to someone else (WS-006).
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
