package run

// RUN-005: scheduling. What a run needs, what a provider offers, and the decision
// that puts the two together - refused before dispatch with a reason a user can
// read when nothing matches (ADR-004, threat model gate B).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
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
	// for what a developer machine can do, and is accepted only where the
	// deployment says it is one. `process` no longer needs a constant of its own:
	// it is refused because it is not in the switch below, along with every other
	// value nobody wrote down.
	//
	// productionIsolation is the only level a deployment accepts without opting
	// in to something weaker. Adding a second one — a MicroVM baseline, say — is
	// a deliberate edit here, which is the point: see the switch in Match.
	productionIsolation = "gvisor"
	// cleanIsolation is the clean test mode's honest name for having no boundary
	// at all: a spawned process on the host, reaped by process group or job
	// object. It is not a sandbox and must never carry untrusted content; what it
	// carries is curated demo material (02:PORT-007) on a machine that cannot run
	// a container. Gated by its own variable rather than by DEV_LOGIN, so an
	// existing development machine does not silently acquire it.
	cleanIsolation = "clean"
	// weakIsolation is what a provider declares when it is running workloads under
	// the host kernel — plain runc, because SKILLHUB_SANDBOX_RUNTIME was unset or
	// misspelled on that node. The declaration is honest and the sandbox does not
	// lie about it; the gap was on this side, where `container` was neither ""
	// nor `process` and so simply passed. A node that lost the variable ran every
	// untrusted skill on a shared kernel and said so in one startup log line.
	weakIsolation = "container"
)

// devDeployment reports whether this deployment has declared itself an offline
// development one, which is the only kind that may dispatch to weakIsolation.
//
// DEV_LOGIN is that declaration and it already exists (ADR-020; cmd/api calls it
// "never in production"), so this reuses it rather than adding a second variable
// an operator could set correctly while getting this one wrong. There is
// deliberately no production escape hatch: opting in to a shared kernel means
// opting in to the offline login provider too, which no production deployment
// can quietly do by accident. Read per call, like RunModel and GatewayURL.
func devDeployment() bool { return os.Getenv("DEV_LOGIN") == "1" }

// cleanTestMode reports whether this deployment has declared itself the clean
// test mode (02:PORT-001..009) — a machine that cannot install a container
// runtime, running curated content for a demo.
//
// Deliberately not DEV_LOGIN. `clean` is weaker than `container`: it is no
// boundary at all. Reusing the development opt-in would hand it to every
// machine that already has DEV_LOGIN exported, which is most of them.
func cleanTestMode() bool { return os.Getenv("SKILLHUB_CLEAN_MODE") == "1" }

// curatedTier is the one value of skills.curation_tier (0042) that means a
// human went through PDM-002 on this material. The other one is `indexed`,
// which means nothing more than "it is in the database".
const curatedTier = "curated"

// ErrContentNotCurated is 02:PORT-010's fifth acceptance criterion, which until
// now had no enforcement point anywhere: 「不得承載不受信任的內容。該模式只跑
// PORT-007 允許的策展素材。」
//
// Three documents each named a different one of the others as the gate and none
// of them was one (04 丙-85). The reason it could not be here in Match is worth
// keeping: Match is a function of a *provider capability*, and content is a
// property of the run. No amount of reading isolation.level can see a skill.
var ErrContentNotCurated = errors.New("the clean test mode only runs curated material")

// requireCuratedContent refuses to hand uncurated material to a driver that has
// no isolation boundary at all (02:PORT-010, 02:PORT-007).
//
// Only in the clean test mode. Every other deployment runs untrusted skills for
// a living behind gVisor (ADR-015), and applying this there would break the
// product; the branch below is the whole of its effect on the normal path.
//
// Fail-closed on every unknown, unlike the ordinary registry reads: the cost of
// a wrong "no" is a demo that will not start, and the cost of a wrong "yes" is
// somebody else's code running on the operator's laptop as the operator.
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
	// What was refused and what would pass, because the operator reading this is
	// the person who has to fix it. Identifiers and the tier only - never the
	// skill's name or anything out of the package (iron rule 11).
	return fmt.Errorf("%w: this one is %s. A skill in the public catalogue, or one whose "+
		"curation_tier is %q on the exact version being run, may run here; anything else needs a "+
		"deployment with a real sandbox",
		ErrContentNotCurated, describeContentSource(source), curatedTier)
}

// describeContentSource says which half of the test failed, in the fewest words
// that still distinguish "never reviewed" from "reviewed, but not these bytes".
// The second is the one that would otherwise look like a bug.
func describeContentSource(source ContentSource) string {
	if source.CurationTier == curatedTier {
		return "curated at a different version than the one being run"
	}
	return fmt.Sprintf("outside the public catalogue with curation_tier %q", source.CurationTier)
}

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
	// An allow list, not a deny list. The deny list refused "", `process` and
	// `container` and let everything else through, so a provider declaring
	// `gvsior` was dispatched to exactly as if it had said gvisor. That never
	// happened, because sandboxd derives the value from a two-way branch — but
	// the shape is the one that already caused an incident here once, and the
	// fix that time was to add one more value to the deny list rather than to
	// invert it. A new level now has to be written down before it can run
	// anything.
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
	// A ceiling the node names here is a number in max_resources with nothing
	// holding it - unbounded, whatever the declaration says. 02:PORT-010 asks the
	// declaration to reflect what was actually detected, and the point of asking
	// was so a gate could refuse; a field only the node's own log reads is the
	// defect ADR-059 decision 3 recorded, not the fix for it.
	//
	// Same shape as the isolation branches above: refused everywhere except the
	// deployment that has opted into having no boundary at all, where "the CPU
	// ceiling is not enforced either" is not news.
	if len(c.MaxResourcesUnenforced) > 0 && !cleanTestMode() {
		return RuntimeProfile{}, fmt.Errorf(
			"%s declares resource ceilings it does not enforce (%s), which this deployment does not accept",
			name, strings.Join(c.MaxResourcesUnenforced, ", "))
	}
	// The egress half of the branch above, and it has to be its own check rather
	// than a clause in that one: a node can enforce every resource ceiling and
	// still filter no traffic, and the two are read by different parts of the
	// deployment's threat model. Same gate, same reason - accepted only where
	// having no boundary at all was already accepted (04 丙-98, 05 R-32).
	//
	// Ordered before egressSatisfied on purpose. Both would refuse a clean node
	// in production, and the error a person reads should name the reason that
	// will still be true after they fix their allow list.
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

	// Every ceiling is required by the provider contract. Missing capability is
	// not permission to dispatch an unbounded run.
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
		// The attempt id *is* the idempotency key: one permanent platform id per
		// dispatch, so a re-send after an uncertain first call cannot start a
		// second sandbox (ADR-004 on failure and retry).
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

// pinProvider records the scheduling decision on the run before the first
// dispatch — and only before the first one.
//
// dispatch() is re-enterable: execute() routes a `queued`/`provisioning` run with
// no live attempt back here, which is where a job retried after
// SetAttemptProviderRunID failed ends up. It used to call this every time, and
// SetRunProvider overwrites runtime_snapshot in place, so a re-dispatch that
// matched a different runtime version left attempt 1's unrecoverable. That column
// exists precisely to freeze what was matched so a past run stays explicable after
// provider capabilities change (RUN-005, ADR-003); overwriting it defeats the
// column rather than serving it.
//
// The whole decision is skipped, not half of it: runs.provider and
// runtime_snapshot are one statement about one scheduling decision, and a
// provider name paired with another attempt's runtime would be worse than either.
// Which provider each dispatch actually went to is on run_attempts.provider,
// which is per attempt and is what cleanup and follow read.
//
// ponytail: the thorough answer is a runtime column on run_attempts, so attempt 2
// records its own instead of inheriting silence. That is a migration; see the
// report accompanying this change.
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
		// The run went terminal while we were scheduling. The caller's next
		// transition will hit the same wall and stop there.
		return run, err
	}
	return updated, nil
}

// alreadyPinned reports whether this run's scheduling decision has been recorded.
// CreateRun writes `{}` and the column is NOT NULL, so "empty object" is the
// unpinned state; parsed rather than string-compared because `{}` is what the
// platform writes, not the only jsonb Postgres can hand back.
func alreadyPinned(run gen.Run) bool {
	var snapshot map[string]json.RawMessage
	return json.Unmarshal(run.RuntimeSnapshot, &snapshot) == nil && len(snapshot) > 0
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
// which is treated the same way as an undeclared resource ceiling: a refusal.
// That sentence was here before the code did it — the switch below opened with
// `len(offered) == 0: return true`, so egress was the one capability that failed
// OPEN while an undeclared ceiling three functions up is a hard error, and
// nothing noticed because schedule_test only ever passed a non-empty list
// (M2 audit, 2026-08-24). ADR-022: 做不到的地方一律走 fail-closed.
//
// A run that names no egress mode is a different thing from a provider that
// names none, and only the first is a pass.
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
