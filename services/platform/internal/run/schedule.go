package run

// RUN-005: scheduling. What a run needs, what a provider offers, and the decision
// that puts the two together - refused before dispatch with a reason a user can
// read when nothing matches (ADR-004, threat model gate B).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
)

// PDM-004 (which runtimes and versions the SelfHostedProvider supports) is still
// open, so the platform asks for a family and an integration mode and lets the
// provider name the version. Pinning a version here would be inventing the answer
// to a decision nobody has taken; resolving it from the capability answer is the
// honest form of the same thing, and what got resolved is frozen into
// runs.runtime_snapshot so a past run stays explainable.
const (
	defaultRuntime          = "claude_agent_sdk"
	defaultAgentIntegration = "in_sandbox_sdk"
	// Iron rule 1: untrusted skills never run in a plain process. gVisor is the
	// production baseline (ADR-015); `container` is the dev provider's honest name
	// for what a developer machine can do; `process` is refused outright.
	forbiddenIsolation = "process"
)

// Requirements is what one run needs from a provider. Derived entirely from the
// run's own frozen policy_snapshot, so scheduling matches against what the user
// was shown before starting, not against today's defaults (ADR-003).
type Requirements struct {
	Runtime          string
	AgentIntegration string
	Limits           ResourceLimits
	EgressMode       string
	// EgressAllowed is how many destinations the run is permitted. It is part of
	// the requirement, not decoration: a request that allows nothing can be met by
	// a provider with no egress route at all, and one that allows something cannot.
	EgressAllowed int
}

// requirementsFor reads the run's frozen policy back out.
func requirementsFor(run gen.Run) (Requirements, policySnapshot, error) {
	var policy policySnapshot
	if err := json.Unmarshal(run.PolicySnapshot, &policy); err != nil {
		return Requirements{}, policy, fmt.Errorf("decode policy snapshot: %w", err)
	}
	return requirementsFromPolicy(policy), policy, nil
}

// DefaultRequirements is what a run created today asks of a provider. Exported so
// the contract suite can put a real provider's capability through the real matcher
// — a provider whose endpoints all pass and that the scheduler still refuses is
// not usable, and that gap is invisible from either side alone.
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

// checkSchedulable refuses work no configured provider can run, before it is
// queued and with a reason a user can read (ADR-004, RUN-005).
//
// An empty registry is not a refusal. A deployment with no sandbox at all is an
// operator problem, not a malformed request: the run is accepted, and it fails
// saying "no sandbox provider is configured" where the user can see it, which is
// more useful than a 422 blaming them for asking.
func (s *Service) checkSchedulable(ctx context.Context, policy policySnapshot) error {
	registry := s.providers()
	if len(registry.Providers) == 0 {
		return nil
	}
	_, _, _, err := registry.Select(ctx, requirementsFromPolicy(policy))
	if errors.Is(err, ErrNoCompatibleProvider) {
		return err
	}
	// A provider that is merely unreachable right now is not a reason to reject the
	// request: queue it, and let the dispatch retry policy deal with the outage.
	return nil
}

// Match reports whether a provider can run these requirements, and if so which
// runtime version it resolved to. The error is the user-facing reason: it names
// the provider and the one thing that did not fit, never a stack of internals.
func Match(c ProviderCapability, req Requirements) (RuntimeProfile, error) {
	name := c.Provider
	if name == "" {
		name = "provider"
	}
	if c.Availability.Healthy != nil && !*c.Availability.Healthy {
		return RuntimeProfile{}, fmt.Errorf("%s reports itself unhealthy", name)
	}
	if c.Isolation.Level == "" || c.Isolation.Level == forbiddenIsolation {
		return RuntimeProfile{}, fmt.Errorf("%s does not isolate workloads strongly enough (isolation %q)", name, c.Isolation.Level)
	}
	if !c.Isolation.Rootless {
		return RuntimeProfile{}, fmt.Errorf("%s does not run workloads unprivileged", name)
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
		Model:            RunModel(), // empty: the gateway's default tier (PDM-003)
	}
	var supported bool
	for _, rt := range c.Runtimes {
		if rt.Runtime != req.Runtime || len(rt.Versions) == 0 {
			continue
		}
		if len(rt.AgentIntegration) > 0 && !contains(rt.AgentIntegration, req.AgentIntegration) {
			return RuntimeProfile{}, fmt.Errorf("%s runs %s but not in %s mode", name, req.Runtime, req.AgentIntegration)
		}
		// Last declared version wins. Providers list oldest first, and a run should
		// get the newest thing the provider is prepared to support.
		profile.RuntimeVersion = rt.Versions[len(rt.Versions)-1]
		supported = true
		break
	}
	if !supported {
		return RuntimeProfile{}, fmt.Errorf("%s does not support the %s runtime", name, req.Runtime)
	}

	// Resource ceilings. A zero on the provider's side means "did not declare",
	// which is not the same as "allows nothing" - see ProviderCapability.
	for _, check := range []struct {
		what            string
		needed, offered float64
	}{
		{"vCPU", req.Limits.VCPU, c.MaxResources.VCPU},
		{"memory", float64(req.Limits.MemoryBytes), float64(c.MaxResources.MemoryBytes)},
		{"disk", float64(req.Limits.DiskBytes), float64(c.MaxResources.DiskBytes)},
		{"processes", float64(req.Limits.MaxPIDs), float64(c.MaxResources.MaxPIDs)},
		{"open files", float64(req.Limits.MaxOpenFiles), float64(c.MaxResources.MaxOpenFiles)},
		{"wall clock", float64(req.Limits.WallClockHardSeconds), float64(c.MaxResources.WallClockHardSeconds)},
	} {
		if check.offered > 0 && check.needed > check.offered {
			return RuntimeProfile{}, fmt.Errorf("%s caps %s below what this run needs", name, check.what)
		}
	}
	return profile, nil
}

// Select picks the first compatible provider. First, not best: with one or two
// configured providers a scoring function would be untestable ceremony, and
// ADR-004 forbids silently re-routing to a different provider anyway.
//
// ponytail: first-fit selection, no load balancing across providers. Revisit when
// more than one provider is configured in production and slots actually contend.
func (r *Registry) Select(ctx context.Context, req Requirements) (*Provider, ProviderCapability, RuntimeProfile, error) {
	return r.SelectExcluding(ctx, req, nil)
}

// SelectExcluding is Select with the drained nodes taken out (SEC-012 action ②,
// ADR-022 X-04 ①「該節點停止接受新 Run（drain），其他節點不受影響」). `halted` is keyed
// by provider name, which is the shape halt.go already holds the switch in.
//
// Draining shows up in the refusal reason like any other mismatch, so a run that
// ends up with nowhere to go says which nodes were drained rather than reporting a
// fleet that mysteriously supports nothing. The whole-pool case never reaches here
// — the caller stops first and leaves the run queued.
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

// runtimeSnapshot is what got matched, frozen onto the run (ADR-003). Enough to
// explain a past scheduling decision after the provider's capability has moved on.
type runtimeSnapshot struct {
	Provider       string         `json:"provider"`
	Runtime        RuntimeProfile `json:"runtime"`
	IsolationLevel string         `json:"isolation_level"`
	Rootless       bool           `json:"rootless"`
	SelectedAt     string         `json:"selected_at"`
}

// buildRunRequest assembles the provider-neutral RunRequest for one attempt.
//
// Every read is workspace scoped from the run's own workspace_id (iron rule 3),
// and everything that travels is a reference or a hash: package bytes and dataset
// bytes move through object storage, never through this body.
func (s *Service) buildRunRequest(
	ctx context.Context, run gen.Run, attempt gen.RunAttempt, profile RuntimeProfile, policy policySnapshot,
) (RunRequest, error) {
	q := s.queries()
	version, err := q.GetSkillVersion(ctx, gen.GetSkillVersionParams{
		ID: run.SkillVersionID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil {
		return RunRequest{}, err
	}
	snapshot, err := q.GetTestCaseSnapshot(ctx, gen.GetTestCaseSnapshotParams{
		ID: run.TestCaseSnapshotID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil {
		return RunRequest{}, err
	}
	refs, err := testlab.DecodeDatasetRefs(snapshot.DatasetRefs)
	if err != nil {
		return RunRequest{}, err
	}
	// SBX-008. The grants are minted first because both halves are fail-closed:
	// a dispatch that cannot authorize its own inputs, or cannot mint the model
	// credential the egress policy assumes, must not reach a sandbox at all.
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
		gatewayGrant, err = s.Gateway.Issue(ctx, uuidString(run.ID), uuidString(attempt.ID), ttl)
		if err != nil {
			return RunRequest{}, err
		}
	}

	return RunRequest{
		RunID:        uuidString(run.ID),
		RunAttemptID: uuidString(attempt.ID),
		Attempt:      int(attempt.AttemptNumber),
		WorkspaceID:  uuidString(run.WorkspaceID),
		// The attempt id *is* the idempotency key: one permanent platform id per
		// dispatch, so a re-send after an uncertain first call cannot start a
		// second sandbox (ADR-004 on failure and retry).
		IdempotencyKey: uuidString(attempt.ID),
		SkillVersion: PackageRef{
			SkillVersionID: uuidString(version.ID),
			ContentHash:    version.ContentHash,
			ObjectKey:      version.PackageObjectKey,
		},
		TestCaseSnapshot: TestCaseSnapshotRef{
			TestCaseSnapshotID: uuidString(snapshot.ID),
			ContentHash:        snapshot.ContentHash,
			UserPrompt:         snapshot.UserPrompt,
			DatasetRefs:        datasets,
		},
		Runtime:        profile,
		ResourceLimits: policy.ResourceLimits,
		Egress:         policy.Egress,
		ObjectGrants:   grants,
		ModelGateway:   gatewayGrant,
		// TRACE-002: the collection destination, with a signed credential scoped
		// to this one (run, attempt) embedded in the URL. A new attempt gets a new
		// token, so a re-dispatched run cannot post events under the old one.
		//
		// `standard` is the only level in use. The contract's `verbose` adds the
		// safety-processed raw events on top; the raw events are already what the
		// advanced view (TRACE-007) shows, so nothing yet distinguishes the two and
		// claiming otherwise would be a setting that does nothing.
		Trace: TracePolicy{
			Level: "standard",
			IngestionURL: s.TraceSigner.IngestionURL(
				s.TraceIngestBaseURL, run.ID, int(attempt.AttemptNumber), time.Now()),
		},
	}, nil
}

// pinProvider records the scheduling decision on the run before the first dispatch.
func (s *Service) pinProvider(
	ctx context.Context, run gen.Run, p *Provider, c ProviderCapability, profile RuntimeProfile,
) (gen.Run, error) {
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
		// The run went terminal while we were scheduling. The caller's next
		// transition will hit the same wall and stop there.
		return run, err
	}
	return updated, nil
}

// egressSatisfied reads the two egress modes as ordered rather than as
// alternatives: `none` (no route out at all) is strictly stronger than
// `default_deny` with an empty allow list.
//
// So a provider that offers only `none` — the dev DockerProvider on
// `--network none`, and any node without an egress proxy — can carry a run that
// is allowed to reach nothing, and cannot carry one that names a destination,
// because it has no route to offer. The substitution only ever runs in this
// direction: a weaker mode is never accepted for a stronger request, whatever
// the request said.
//
// A provider that declares no egress modes at all has not answered the question,
// which is treated the same way as an undeclared resource ceiling.
func egressSatisfied(offered []string, req Requirements) bool {
	switch {
	case req.EgressMode == "" || len(offered) == 0:
		return true
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
