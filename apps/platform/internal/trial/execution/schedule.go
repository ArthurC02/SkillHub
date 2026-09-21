package run

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const (
	defaultRuntime          = "claude_agent_sdk"
	defaultAgentIntegration = "in_sandbox_sdk"

	strongIsolation IsolationStrength = "strong"

	weakIsolation IsolationStrength = "weak"

	noIsolation IsolationStrength = "none"
)

type IsolationStrength string

var isolationRank = map[IsolationStrength]int{noIsolation: 1, weakIsolation: 2, strongIsolation: 3}

func (s IsolationStrength) meets(minimum IsolationStrength) bool {
	return isolationRank[s] >= isolationRank[minimum]
}

func requiredIsolation() IsolationStrength {
	switch {
	case cleanTestMode():
		return noIsolation
	case devDeployment():
		return weakIsolation
	default:
		return strongIsolation
	}
}

func devDeployment() bool { return os.Getenv("DEV_LOGIN") == "1" }

func cleanTestMode() bool { return os.Getenv("SKILLHUB_CLEAN_MODE") == "1" }

type curationTier string

const curatedTier curationTier = "curated"

var ErrContentNotCurated = errors.New("the clean test mode only runs curated material")

func (s *Service) requireCuratedContent(ctx context.Context, run gen.Run) error {
	released, err := s.curatedContentRefusal(ctx, run.WorkspaceID, run.SkillVersionID)
	if released != "" {
		slog.Warn("clean mode: an operator released this version to run with no isolation boundary",
			"run_id", pgconv.UUIDString(run.ID),
			"skill_version_id", pgconv.UUIDString(run.SkillVersionID),
			"reason", released)
	}
	return err
}

// curatedContentRefusal answers for a (workspace, version) pair alone, so the
// pre-run summary can give the same answer before a run exists.
func (s *Service) curatedContentRefusal(
	ctx context.Context, workspaceID, skillVersionID pgtype.UUID,
) (released string, err error) {
	if !cleanTestMode() {
		return "", nil
	}
	if s.ReadContentSource == nil {
		return "", fmt.Errorf("%w, and this deployment cannot tell where this material came from "+
			"(the content-source read is not configured)", ErrContentNotCurated)
	}
	source, found, readErr := s.ReadContentSource(ctx, workspaceID, skillVersionID)
	if readErr != nil {
		return "", fmt.Errorf("%w, and where this material came from could not be read: %w",
			ErrContentNotCurated, readErr)
	}
	if !found {
		return "", fmt.Errorf("%w, and this version's skill could not be found to check", ErrContentNotCurated)
	}
	if source.WorkspaceIsCatalog || (curationTier(source.CurationTier) == curatedTier && source.CuratedVersionIsThisOne) {
		return "", nil
	}
	versionID := pgconv.UUIDString(skillVersionID)
	if reason, ok := operatorReleased(versionID); ok {
		return reason, nil
	}

	return "", fmt.Errorf("%w: this one is %s. A skill in the public catalogue, or one whose "+
		"curation_tier is %q on the exact version being run, may run here; anything else needs a "+
		"deployment with a real sandbox — or %s",
		ErrContentNotCurated, describeContentSource(source), curatedTier, howToRelease(versionID))
}

const cleanModeReleaseFile = "SKILLHUB_CLEAN_MODE_RELEASES"

func operatorReleased(versionID string) (string, bool) {
	path := os.Getenv(cleanModeReleaseFile)
	if path == "" || versionID == "" {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {

		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("clean mode: the operator release list could not be read",
				"path", path, "error", err)
		}
		return "", false
	}

	text := strings.TrimPrefix(string(raw), "\ufeff")
	for i, line := range strings.Split(text, "\n") {

		line = strings.Trim(strings.TrimLeft(strings.TrimSpace(line), "-*• \t"), " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		id := releaseToken(parts[0])

		if !strings.EqualFold(id, versionID) {

			if strings.Contains(strings.ToLower(line), strings.ToLower(versionID)) {
				slog.Warn("clean mode: a line mentions this version but does not release it; "+
					"a release is the version id first, then the reason",
					"path", path, "line", i+1, "skill_version_id", versionID)
			}
			continue
		}
		reason := strings.TrimSpace(strings.Join(parts[1:], " "))
		if reason == "" {
			slog.Warn("clean mode: a release line with no reason is not a release",
				"path", path, "line", i+1, "skill_version_id", versionID)
			return "", false
		}
		return reason, true
	}
	return "", false
}

func releaseToken(s string) string { return strings.Trim(s, "`'\"“”‘’,;:") }

func howToRelease(versionID string) string {
	path := os.Getenv(cleanModeReleaseFile)
	if path == "" {
		return fmt.Sprintf("an operator may release this exact version by pointing %s at a file "+
			"and putting `%s <why>` in it (05 R-37); this deployment has not set that variable, "+
			"so nothing is released", cleanModeReleaseFile, versionID)
	}
	return fmt.Sprintf("an operator may release this exact version by adding `%s <why>` to %s "+
		"(05 R-37) — a line with no reason after the id is not a release", versionID, path)
}

func describeContentSource(source ContentSource) string {
	if curationTier(source.CurationTier) == curatedTier {
		return "curated at a different version than the one being run"
	}
	return fmt.Sprintf("outside the public catalogue with curation_tier %q", source.CurationTier)
}

type Requirements struct {
	Runtime          string
	AgentIntegration string
	Limits           ResourceLimits
	EgressMode       string

	EgressAllowed int
}

func requirementsFor(run gen.Run) (Requirements, policySnapshot, error) {
	var policy policySnapshot
	if err := json.Unmarshal(run.PolicySnapshot, &policy); err != nil {
		return Requirements{}, policy, fmt.Errorf("decode policy snapshot: %w", err)
	}
	return requirementsFromPolicy(policy), policy, nil
}

func requirementsFromPolicy(policy policySnapshot) Requirements {
	return Requirements{
		Runtime:          defaultRuntime,
		AgentIntegration: defaultAgentIntegration,
		Limits:           policy.ResourceLimits,
		EgressMode:       policy.Egress.Mode,
		EgressAllowed:    len(policy.Egress.Allow),
	}
}

func (s *Service) checkSchedulable(ctx context.Context, policy policySnapshot) error {
	reason, err := s.schedulableRefusal(ctx, policy)
	if err != nil {
		return refused(reason, err)
	}
	return nil
}

func (s *Service) schedulableRefusal(ctx context.Context, policy policySnapshot) (string, error) {
	if !policy.reachesAModel() {
		return ReasonCapabilityMismatch, ErrNoModelGateway
	}
	registry := s.providers()
	if len(registry.Providers) == 0 {
		return ReasonCapabilityMismatch, ErrNoProvider
	}
	_, _, _, err := registry.Select(ctx, requirementsFromPolicy(policy))
	if errors.Is(err, ErrNoCompatibleProvider) {
		return ReasonCapabilityMismatch, err
	}

	return "", nil
}

type providerRefusal struct {
	english     string
	inWords     string
	mayComeBack bool
}

func (r providerRefusal) Error() string { return r.english }

func cannotRun(english, inWords string) providerRefusal {
	return providerRefusal{english: english, inWords: inWords}
}

func notRightNow(english, inWords string) providerRefusal {
	return providerRefusal{english: english, inWords: inWords, mayComeBack: true}
}

func setAsideRefusal(name string, aside SetAsideProvider) providerRefusal {
	english := fmt.Sprintf("%s is set aside: %s", name, aside.Why)
	if aside.MayComeBack {
		return notRightNow(english, fmt.Sprintf("%s 暫時被排開了", name))
	}
	return cannotRun(english, fmt.Sprintf("%s 弄丟過這次試跑的上一次嘗試，不會再交給它", name))
}

type poolRefusal struct {
	sentinel error
	refusals []providerRefusal
}

func (p poolRefusal) Unwrap() error { return p.sentinel }

func (p poolRefusal) Error() string {
	why := make([]string, 0, len(p.refusals))
	for _, refusal := range p.refusals {
		why = append(why, refusal.english)
	}
	return fmt.Sprintf("%s: %s", p.sentinel, strings.Join(why, "; "))
}

func (p poolRefusal) inInterfaceLanguage() string {
	said := make([]string, 0, len(p.refusals))
	for _, refusal := range p.refusals {
		said = append(said, refusal.inWords)
	}
	return "不符的項目：" + strings.Join(said, "；") + "。"
}

func asProviderRefusal(name string, err error) providerRefusal {
	if refusal, ok := errors.AsType[providerRefusal](err); ok {
		return refusal
	}
	return cannotRun(fmt.Sprintf("%s: %s", name, err), fmt.Sprintf("%s 不符合這次試跑的要求", name))
}

func noneFit(refusals []providerRefusal) error {
	pool := poolRefusal{sentinel: ErrNoCompatibleProvider, refusals: refusals}
	for _, refusal := range refusals {
		if refusal.mayComeBack {
			pool.sentinel = ErrNoSandboxAvailableYet
		}
	}
	return pool
}

func Match(c ProviderCapability, req Requirements) (RuntimeProfile, error) {
	name := c.Provider
	if name == "" {
		name = "provider"
	}
	if c.Availability.Healthy != nil && !*c.Availability.Healthy {
		return RuntimeProfile{}, notRightNow(
			fmt.Sprintf("%s reports itself unhealthy", name),
			fmt.Sprintf("%s 回報自己不健康", name))
	}

	if !c.Isolation.Strength.meets(requiredIsolation()) {
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s isolates workloads %q, and this deployment runs nothing weaker than %q",
				name, c.Isolation.Strength, requiredIsolation()),
			fmt.Sprintf("%s 的隔離強度是 %q，這個部署不跑比 %q 更弱的",
				name, c.Isolation.Strength, requiredIsolation()))
	}
	if !c.Isolation.Rootless {
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s does not run workloads unprivileged", name),
			fmt.Sprintf("%s 不是以非特權身分執行工作負載", name))
	}

	if len(c.MaxResourcesUnenforced) > 0 && !cleanTestMode() {
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s declares resource ceilings it does not enforce (%s), which this deployment does not accept",
				name, strings.Join(c.MaxResourcesUnenforced, ", ")),
			fmt.Sprintf("%s 宣告了自己不強制的資源上限（%s），這個部署不接受",
				name, strings.Join(c.MaxResourcesUnenforced, "、")))
	}

	if c.Network.EgressUnenforced && !cleanTestMode() {
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s declares egress modes it does not enforce, which this deployment does not accept", name),
			fmt.Sprintf("%s 宣告了自己不強制的網路出口模式，這個部署不接受", name))
	}
	if !egressSatisfied(c.Network.EgressModes, req) {
		if req.EgressAllowed > 0 {
			return RuntimeProfile{}, cannotRun(
				fmt.Sprintf("%s cannot enforce %s network egress with an allow list", name, req.EgressMode),
				fmt.Sprintf("%s 沒辦法在帶允許清單的情況下強制 %s 網路出口", name, req.EgressMode))
		}
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s cannot enforce %s network egress", name, req.EgressMode),
			fmt.Sprintf("%s 沒辦法強制 %s 網路出口", name, req.EgressMode))
	}

	profile := RuntimeProfile{
		Runtime:          req.Runtime,
		AgentIntegration: req.AgentIntegration,
		Model:            RunModel(),
	}
	var supported bool
	for _, rt := range c.Runtimes {
		if rt.Runtime != req.Runtime || len(rt.Versions) == 0 {
			continue
		}
		if len(rt.AgentIntegration) > 0 && !slices.Contains(rt.AgentIntegration, req.AgentIntegration) {
			return RuntimeProfile{}, cannotRun(
				fmt.Sprintf("%s runs %s but not in %s mode", name, req.Runtime, req.AgentIntegration),
				fmt.Sprintf("%s 跑得動 %s，但不支援 %s 這個接法", name, req.Runtime, req.AgentIntegration))
		}

		profile.RuntimeVersion = rt.Versions[len(rt.Versions)-1]
		supported = true
		break
	}
	if !supported {
		return RuntimeProfile{}, cannotRun(
			fmt.Sprintf("%s does not support the %s runtime", name, req.Runtime),
			fmt.Sprintf("%s 不支援 %s 這個執行環境", name, req.Runtime))
	}

	for _, check := range []struct {
		what, inWords   string
		needed, offered float64
	}{
		{"vCPU", "vCPU", req.Limits.VCPU, c.MaxResources.VCPU},
		{"memory", "記憶體", float64(req.Limits.MemoryBytes), float64(c.MaxResources.MemoryBytes)},
		{"disk", "磁碟", float64(req.Limits.DiskBytes), float64(c.MaxResources.DiskBytes)},
		{"processes", "行程數", float64(req.Limits.MaxPIDs), float64(c.MaxResources.MaxPIDs)},
		{"open files", "開檔數", float64(req.Limits.MaxOpenFiles), float64(c.MaxResources.MaxOpenFiles)},
		{"soft wall clock", "軟性執行時限", float64(req.Limits.WallClockSoftSeconds), float64(c.MaxResources.WallClockSoftSeconds)},
		{"hard wall clock", "硬性執行時限", float64(req.Limits.WallClockHardSeconds), float64(c.MaxResources.WallClockHardSeconds)},
		{"total artifact bytes", "產出總位元組", float64(req.Limits.ArtifactTotalBytes), float64(c.MaxResources.ArtifactTotalBytes)},
		{"artifact file bytes", "單一產出位元組", float64(req.Limits.ArtifactFileBytes), float64(c.MaxResources.ArtifactFileBytes)},
		{"input tokens", "輸入 Token", float64(req.Limits.TokenBudget.MaxInputTokens), float64(c.MaxResources.TokenBudget.MaxInputTokens)},
		{"output tokens", "輸出 Token", float64(req.Limits.TokenBudget.MaxOutputTokens), float64(c.MaxResources.TokenBudget.MaxOutputTokens)},
	} {
		if check.offered <= 0 {
			return RuntimeProfile{}, cannotRun(
				fmt.Sprintf("%s does not declare a %s ceiling", name, check.what),
				fmt.Sprintf("%s 沒有宣告 %s 的上限", name, check.inWords))
		}
		if check.needed > check.offered {
			return RuntimeProfile{}, cannotRun(
				fmt.Sprintf("%s caps %s below what this run needs", name, check.what),
				fmt.Sprintf("%s 的 %s 上限低於這次試跑需要的", name, check.inWords))
		}
	}
	return profile, nil
}

type Placement struct {
	Provider   SandboxProvider
	Capability ProviderCapability
	Profile    RuntimeProfile
}

func (p Placement) freeSlots() int { return p.Capability.Availability.ConcurrentRunSlots }

func (r *Registry) Select(ctx context.Context, req Requirements) (SandboxProvider, ProviderCapability, RuntimeProfile, error) {
	compatible, err := r.compatible(ctx, req, nil)
	if err != nil {
		return nil, ProviderCapability{}, RuntimeProfile{}, err
	}
	return compatible[0].Provider, compatible[0].Capability, compatible[0].Profile, nil
}

func (r *Registry) Place(ctx context.Context, req Requirements, setAside map[string]SetAsideProvider) ([]Placement, error) {
	compatible, err := r.compatible(ctx, req, setAside)
	if err != nil {
		return nil, err
	}
	free := slices.DeleteFunc(compatible, func(p Placement) bool { return p.freeSlots() <= 0 })
	if len(free) == 0 {
		return nil, ErrNoFreeSlot
	}
	slices.SortStableFunc(free, func(a, b Placement) int { return cmp.Compare(b.freeSlots(), a.freeSlots()) })
	return free, nil
}

func (r *Registry) compatible(ctx context.Context, req Requirements, setAside map[string]SetAsideProvider) ([]Placement, error) {
	if len(r.Providers) == 0 {
		return nil, ErrNoProvider
	}
	var placements []Placement
	refusals := make([]providerRefusal, 0, len(r.Providers))
	for _, p := range r.Providers {
		if aside, ok := setAside[p.Name()]; ok {
			refusals = append(refusals, setAsideRefusal(p.Name(), aside))
			continue
		}
		capability, err := r.Capability(ctx, p)
		if err != nil {
			refusals = append(refusals, notRightNow(
				fmt.Sprintf("%s is unreachable", p.Name()),
				fmt.Sprintf("%s 沒有回應", p.Name())))
			continue
		}
		if capability.Provider == "" {
			capability.Provider = p.Name()
		}
		profile, err := Match(capability, req)
		if err != nil {
			refusals = append(refusals, asProviderRefusal(p.Name(), err))
			continue
		}
		placements = append(placements, Placement{Provider: p, Capability: capability, Profile: profile})
	}
	if len(placements) == 0 {
		return nil, noneFit(refusals)
	}
	return placements, nil
}

type runtimeSnapshot struct {
	Provider          string            `json:"provider"`
	Runtime           RuntimeProfile    `json:"runtime"`
	IsolationStrength IsolationStrength `json:"isolation_strength"`
	Rootless          bool              `json:"rootless"`
	SelectedAt        string            `json:"selected_at"`
}

func (s *Service) buildRunRequest(
	ctx context.Context, run gen.Run, attempt gen.RunAttempt, profile RuntimeProfile,
	policy policySnapshot, budgetUSD float64,
) (RunRequest, error) {
	if s.ReadVersion == nil {
		return RunRequest{}, errRegistryReadNotConfigured
	}
	if err := s.requireTestLab(); err != nil {
		return RunRequest{}, err
	}
	version, found, err := s.ReadVersion(ctx, run.WorkspaceID, run.SkillVersionID)
	if !found && err == nil {
		return RunRequest{}, ErrNotFound
	}
	if err != nil {
		return RunRequest{}, err
	}
	snapshot, err := s.TestLab.ReadSnapshot(ctx, run.WorkspaceID, run.TestCaseSnapshotID)
	if err != nil {
		return RunRequest{}, err
	}
	refs, err := testlab.DecodeDatasetRefs(snapshot.DatasetRefs)
	if err != nil {
		return RunRequest{}, err
	}

	ttl := time.Duration(policy.ResourceLimits.WallClockHardSeconds)*time.Second + grantSlack
	grants, datasetKeys, err := s.grantsFor(ctx, run, attempt, version, refs, ttl)
	if err != nil {
		return RunRequest{}, err
	}
	datasets := make([]datasetRef, 0, len(refs))
	for i, d := range refs {
		datasets = append(datasets, datasetRef{
			DatasetID: d.DatasetID, FileName: d.FileName, ContentHash: d.ContentHash,
			ObjectKey: datasetKeys[i],
		})
	}

	var gatewayGrant *ModelGatewayGrant
	if s.Gateway != nil {
		gatewayGrant, err = s.Gateway.Issue(ctx, pgconv.UUIDString(run.ID), pgconv.UUIDString(attempt.ID), ttl, budgetUSD)
		if err != nil {
			return RunRequest{}, err
		}
	}

	return RunRequest{
		RunID:        pgconv.UUIDString(run.ID),
		RunAttemptID: pgconv.UUIDString(attempt.ID),
		Attempt:      int(attempt.AttemptNumber),
		WorkspaceID:  pgconv.UUIDString(run.WorkspaceID),

		IdempotencyKey: pgconv.UUIDString(attempt.ID),
		SkillVersion: PackageRef{
			SkillVersionID: pgconv.UUIDString(version.ID),
			ContentHash:    version.ContentHash,
			ObjectKey:      version.PackageObjectKey,
		},
		TestCaseSnapshot: TestCaseSnapshotRef{
			TestCaseSnapshotID: pgconv.UUIDString(snapshot.ID),
			ContentHash:        snapshot.ContentHash,
			UserPrompt:         snapshot.UserPrompt,
			DatasetRefs:        datasets,
		},
		Runtime:        profile,
		ResourceLimits: policy.ResourceLimits,
		Egress:         policy.Egress,
		ObjectGrants:   grants,
		ModelGateway:   gatewayGrant,

		Trace: TracePolicy{
			Level: "standard",
			IngestionURL: s.TraceSigner.IngestionURL(
				s.TraceIngestBaseURL, run.ID, int(attempt.AttemptNumber), time.Now()),
		},
	}, nil
}

func pinnedRuntime(p SandboxProvider, c ProviderCapability, profile RuntimeProfile) ([]byte, error) {
	return json.Marshal(runtimeSnapshot{
		Provider:          p.Name(),
		Runtime:           profile,
		IsolationStrength: c.Isolation.Strength,
		Rootless:          c.Isolation.Rootless,
		SelectedAt:        nowUTC(),
	})
}

func alreadyPinned(run gen.Run) bool {
	var snapshot map[string]json.RawMessage
	return json.Unmarshal(run.RuntimeSnapshot, &snapshot) == nil && len(snapshot) > 0
}

func egressSatisfied(offered []string, req Requirements) bool {
	switch {
	case req.EgressMode == "":
		return true
	case len(offered) == 0:
		return false
	case slices.Contains(offered, req.EgressMode):
		return true
	default:
		return req.EgressMode == "default_deny" && req.EgressAllowed == 0 && slices.Contains(offered, "none")
	}
}
