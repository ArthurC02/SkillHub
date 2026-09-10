package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const (
	defaultRuntime          = "claude_agent_sdk"
	defaultAgentIntegration = "in_sandbox_sdk"

	productionIsolation = "gvisor"

	cleanIsolation = "clean"

	weakIsolation = "container"
)

func devDeployment() bool { return os.Getenv("DEV_LOGIN") == "1" }

func cleanTestMode() bool { return os.Getenv("SKILLHUB_CLEAN_MODE") == "1" }

const curatedTier = "curated"

var ErrContentNotCurated = errors.New("the clean test mode only runs curated material")

func (s *Service) requireCuratedContent(ctx context.Context, run gen.Run) error {
	if !cleanTestMode() {
		return nil
	}
	if s.ReadContentSource == nil {
		return fmt.Errorf("%w, and this deployment cannot tell where this material came from "+
			"(the content-source read is not configured)", ErrContentNotCurated)
	}
	source, found, err := s.ReadContentSource(ctx, run.WorkspaceID, run.SkillVersionID)
	if err != nil {
		return fmt.Errorf("%w, and where this material came from could not be read: %w", ErrContentNotCurated, err)
	}
	if !found {
		return fmt.Errorf("%w, and this version's skill could not be found to check", ErrContentNotCurated)
	}
	if source.WorkspaceIsCatalog || (source.CurationTier == curatedTier && source.CuratedVersionIsThisOne) {
		return nil
	}
	versionID := pgconv.UUIDString(run.SkillVersionID)
	if reason, released := operatorReleased(versionID); released {

		slog.Warn("clean mode: an operator released this version to run with no isolation boundary",
			"run_id", pgconv.UUIDString(run.ID),
			"skill_version_id", versionID,
			"reason", reason)
		return nil
	}

	return fmt.Errorf("%w: this one is %s. A skill in the public catalogue, or one whose "+
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
	if source.CurationTier == curatedTier {
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

func DefaultRequirements() Requirements {
	return requirementsFromPolicy(defaultPolicy())
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
	registry := s.providers()
	if len(registry.Providers) == 0 {
		return nil
	}
	_, _, _, err := registry.Select(ctx, requirementsFromPolicy(policy))
	if errors.Is(err, ErrNoCompatibleProvider) {
		return err
	}

	return nil
}

func Match(c ProviderCapability, req Requirements) (RuntimeProfile, error) {
	name := c.Provider
	if name == "" {
		name = "provider"
	}
	if c.Availability.Healthy != nil && !*c.Availability.Healthy {
		return RuntimeProfile{}, fmt.Errorf("%s reports itself unhealthy", name)
	}

	switch c.Isolation.Level {
	case productionIsolation:
	case weakIsolation:
		if !devDeployment() {
			return RuntimeProfile{}, fmt.Errorf(
				"%s isolates workloads with the host kernel (isolation %q), which this deployment does not accept", name, c.Isolation.Level)
		}
	case cleanIsolation:
		if !cleanTestMode() {
			return RuntimeProfile{}, fmt.Errorf(
				"%s does not isolate workloads at all (isolation %q), which this deployment does not accept", name, c.Isolation.Level)
		}
	default:
		return RuntimeProfile{}, fmt.Errorf("%s does not isolate workloads strongly enough (isolation %q)", name, c.Isolation.Level)
	}
	if !c.Isolation.Rootless {
		return RuntimeProfile{}, fmt.Errorf("%s does not run workloads unprivileged", name)
	}

	if len(c.MaxResourcesUnenforced) > 0 && !cleanTestMode() {
		return RuntimeProfile{}, fmt.Errorf(
			"%s declares resource ceilings it does not enforce (%s), which this deployment does not accept",
			name, strings.Join(c.MaxResourcesUnenforced, ", "))
	}

	if c.Network.EgressUnenforced && !cleanTestMode() {
		return RuntimeProfile{}, fmt.Errorf(
			"%s declares egress modes it does not enforce, which this deployment does not accept", name)
	}
	if !egressSatisfied(c.Network.EgressModes, req) {
		if req.EgressAllowed > 0 {
			return RuntimeProfile{}, fmt.Errorf(
				"%s cannot enforce %s network egress with an allow list", name, req.EgressMode)
		}
		return RuntimeProfile{}, fmt.Errorf("%s cannot enforce %s network egress", name, req.EgressMode)
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
		if len(rt.AgentIntegration) > 0 && !contains(rt.AgentIntegration, req.AgentIntegration) {
			return RuntimeProfile{}, fmt.Errorf("%s runs %s but not in %s mode", name, req.Runtime, req.AgentIntegration)
		}

		profile.RuntimeVersion = rt.Versions[len(rt.Versions)-1]
		supported = true
		break
	}
	if !supported {
		return RuntimeProfile{}, fmt.Errorf("%s does not support the %s runtime", name, req.Runtime)
	}

	for _, check := range []struct {
		what            string
		needed, offered float64
	}{
		{"vCPU", req.Limits.VCPU, c.MaxResources.VCPU},
		{"memory", float64(req.Limits.MemoryBytes), float64(c.MaxResources.MemoryBytes)},
		{"disk", float64(req.Limits.DiskBytes), float64(c.MaxResources.DiskBytes)},
		{"processes", float64(req.Limits.MaxPIDs), float64(c.MaxResources.MaxPIDs)},
		{"open files", float64(req.Limits.MaxOpenFiles), float64(c.MaxResources.MaxOpenFiles)},
		{"soft wall clock", float64(req.Limits.WallClockSoftSeconds), float64(c.MaxResources.WallClockSoftSeconds)},
		{"hard wall clock", float64(req.Limits.WallClockHardSeconds), float64(c.MaxResources.WallClockHardSeconds)},
		{"total artifact bytes", float64(req.Limits.ArtifactTotalBytes), float64(c.MaxResources.ArtifactTotalBytes)},
		{"artifact file bytes", float64(req.Limits.ArtifactFileBytes), float64(c.MaxResources.ArtifactFileBytes)},
		{"input tokens", float64(req.Limits.TokenBudget.MaxInputTokens), float64(c.MaxResources.TokenBudget.MaxInputTokens)},
		{"output tokens", float64(req.Limits.TokenBudget.MaxOutputTokens), float64(c.MaxResources.TokenBudget.MaxOutputTokens)},
	} {
		if check.offered <= 0 {
			return RuntimeProfile{}, fmt.Errorf("%s does not declare a %s ceiling", name, check.what)
		}
		if check.needed > check.offered {
			return RuntimeProfile{}, fmt.Errorf("%s caps %s below what this run needs", name, check.what)
		}
	}
	return profile, nil
}

func (r *Registry) Select(ctx context.Context, req Requirements) (*Provider, ProviderCapability, RuntimeProfile, error) {
	return r.SelectExcluding(ctx, req, nil)
}

func (r *Registry) SelectExcluding(
	ctx context.Context, req Requirements, halted map[string]gen.DispatchHalt,
) (*Provider, ProviderCapability, RuntimeProfile, error) {
	if len(r.Providers) == 0 {
		return nil, ProviderCapability{}, RuntimeProfile{}, ErrNoProvider
	}
	reasons := make([]string, 0, len(r.Providers))
	for _, p := range r.Providers {
		if halt, ok := halted[p.Name]; ok {
			reasons = append(reasons, fmt.Sprintf("%s is drained (%s)", p.Name, halt.Source))
			continue
		}
		capability, err := r.Capability(ctx, p)
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("%s is unreachable", p.Name))
			continue
		}
		if capability.Provider == "" {
			capability.Provider = p.Name
		}
		profile, err := Match(capability, req)
		if err != nil {
			reasons = append(reasons, err.Error())
			continue
		}
		return p, capability, profile, nil
	}
	return nil, ProviderCapability{}, RuntimeProfile{},
		fmt.Errorf("%w: %s", ErrNoCompatibleProvider, strings.Join(reasons, "; "))
}

type runtimeSnapshot struct {
	Provider       string         `json:"provider"`
	Runtime        RuntimeProfile `json:"runtime"`
	IsolationLevel string         `json:"isolation_level"`
	Rootless       bool           `json:"rootless"`
	SelectedAt     string         `json:"selected_at"`
}

func (s *Service) buildRunRequest(
	ctx context.Context, run gen.Run, attempt gen.RunAttempt, profile RuntimeProfile, policy policySnapshot,
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
		gatewayGrant, err = s.Gateway.Issue(ctx, pgconv.UUIDString(run.ID), pgconv.UUIDString(attempt.ID), ttl)
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

func (s *Service) pinProvider(
	ctx context.Context, run gen.Run, p *Provider, c ProviderCapability, profile RuntimeProfile,
) (gen.Run, error) {
	if alreadyPinned(run) {
		return run, nil
	}
	snapshot, err := json.Marshal(runtimeSnapshot{
		Provider:       p.Name,
		Runtime:        profile,
		IsolationLevel: c.Isolation.Level,
		Rootless:       c.Isolation.Rootless,
		SelectedAt:     nowUTC(),
	})
	if err != nil {
		return run, err
	}
	updated, err := s.queries().SetRunProvider(ctx, gen.SetRunProviderParams{
		ID: run.ID, WorkspaceID: run.WorkspaceID, Provider: p.Name, RuntimeSnapshot: snapshot,
	})
	if err != nil {

		return run, err
	}
	return updated, nil
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
	case contains(offered, req.EgressMode):
		return true
	default:
		return req.EgressMode == "default_deny" && req.EgressAllowed == 0 && contains(offered, "none")
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
