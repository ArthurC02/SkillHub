package worker

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func testDeps(t *testing.T) (*pgxpool.Pool, Deps) {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	store, err := objstore.New("localhost:8333", "key", "secret", "skillhub", false)
	if err != nil {
		t.Fatalf("objstore.New: %v", err)
	}
	return pool, Deps{
		Providers:          &run.Registry{},
		Store:              store,
		TraceSigner:        &trace.Signer{Secret: []byte("test")},
		TraceIngestBaseURL: "https://control.invalid",
		LLM:                &llmclient.Client{BaseURL: "https://llm.invalid", Token: "t"},
	}
}

func TestBuildWorkersInjectsEveryDependencyThisProcessOwns(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := BuildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}

	switch {
	case set.Runs.Pool == nil:
		t.Error("run service has no pool")
	case set.Runs.Queue == nil:
		t.Error("run service has no queue: cleanup and the supervisor's re-enqueue are silently skipped (RUN-007)")
	case set.Runs.Providers == nil:
		t.Error("run service has no provider registry")
	case set.Runs.Store == nil:
		t.Error("run service has no object store")
	case set.Runs.ActiveArtifactReferences == nil:
		t.Error("run service has no packaging artifact reference counter")
	case set.Runs.ReadSkill == nil || set.Runs.ReadVersion == nil:
		t.Error("run service has no Registry owner reads")
	case set.Runs.ReadContentSource == nil:

		t.Error("run service has no content-source read: clean mode would refuse every dispatch")
	case set.Registry == nil || set.Registry.CatalogWorkspaces == nil:
		t.Error("registry service has no catalog workspace read: every catalog skill reads as not found")
	case set.CreationSearch == nil || set.CreationSearch.CatalogWorkspaces == nil:
		t.Error("creation search has no catalog workspace read: every creation knowledge search fails")
	case set.Runs.TestLab == nil:
		t.Error("run service has no Test Lab owner reads")
	case set.Runs.Trace == nil:
		t.Error("run service has no Trace masking activity reader")
	case set.Runs.TraceSigner == nil || set.Runs.TraceIngestBaseURL == "":
		t.Error("run service was wired with half a trace configuration")
	}

	switch {
	case set.Evaluations.Pool == nil || set.Evaluations.Store == nil:
		t.Error("evaluation service is missing its persistence")
	case set.Evaluations.Trace == nil:
		t.Error("evaluation service has no trace context")
	case set.Runs.Trace != set.Evaluations.Trace:
		t.Error("run and evaluation were not wired to the shared Trace service")
	case set.Evaluations.Trace.ReadRunState == nil || set.Evaluations.Trace.ReadIngestRunState == nil ||
		set.Evaluations.Trace.ReadRunTransitions == nil:
		t.Error("trace service is missing a Run-owned fact reader")
	case set.Evaluations.ReadRunFacts == nil || set.Evaluations.ReadEvaluationInput == nil:
		t.Error("evaluation service is missing Run-owned fact readers")
	case set.Evaluations.ReadVersion == nil || set.Evaluations.ReadLatestVersion == nil ||
		set.Evaluations.ReadSkill == nil || set.Evaluations.ReadRuntimeCompatibility == nil:
		t.Error("evaluation service is missing Registry-owned fact readers")
	case set.Evaluations.TestLab == nil:
		t.Error("evaluation service is missing Test Lab owner reads")
	case set.Evaluations.Judge == nil || set.Evaluations.Suggester == nil:
		t.Error("LLM was configured but the judge or the suggester did not get it")
	}
	if set.Packaging == nil || set.Packaging.TestLab == nil {
		t.Error("packaging service is missing Test Lab owner reads")
	} else if set.Runs.TestLab != set.Evaluations.TestLab || set.Runs.TestLab != set.Packaging.TestLab {
		t.Error("run, evaluation and packaging were not wired to the shared Test Lab service")
	}

	if set.Objects.ListExpiredArtifacts == nil || set.Objects.ListClaimedArtifacts == nil ||
		set.Objects.ListClaimedDatasets == nil || set.Objects.RecordArtifactPurged == nil ||
		set.Objects.RecordDatasetLost == nil || set.Objects.GuardArtifactRemoval == nil {
		t.Error("object reconciler is missing an owner read/write function")
	}

	if set.RunEvents.HasCurrentEvaluation == nil || set.RunEvents.Insert == nil {
		t.Error("run event consumer is missing HasCurrentEvaluation or Insert")
	}
}

func TestBuildWorkersLeavesTheJudgeUnsetWithoutAnLLM(t *testing.T) {
	pool, deps := testDeps(t)
	deps.LLM = nil
	set, err := BuildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}
	if set.Evaluations.Judge != nil || set.Evaluations.Suggester != nil {
		t.Error("no LLM service configured, yet the evaluation service holds a judge or a suggester")
	}
}

func TestOutboxDispatchAccountsForEveryEventType(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := BuildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}
	if err := set.Events.Validate(); err != nil {
		t.Errorf("outbox dispatch leaves part of the catalogue unaccounted for: %v", err)
	}
}

func TestRiverConfigPollOnlyDefaultsToListenMode(t *testing.T) {
	if got := riverConfig(river.NewWorkers(), nil, false).PollOnly; got {
		t.Error("PollOnly defaulted to true; cmd/worker never sets Deps.PollOnly and depends on this staying false")
	}
	if got := riverConfig(river.NewWorkers(), nil, true).PollOnly; !got {
		t.Error("PollOnly:true did not reach river.Config; cmd/api's clean test mode needs this to avoid a second LISTEN connection")
	}
}

func TestEveryScheduledJobHasAWorker(t *testing.T) {
	pool, deps := testDeps(t)
	set, err := BuildWorkers(pool, deps)
	if err != nil {
		t.Fatalf("BuildWorkers: %v", err)
	}

	want := map[string]bool{
		eval.RecoveryArgs{}.Kind():      true,
		run.SuperviseArgs{}.Kind():      true,
		run.OrphanScanArgs{}.Kind():     true,
		outbox.PublishArgs{}.Kind():     true,
		objreconcile.Args{}.Kind():      false,
		PartitionCreateArgs{}.Kind():    true,
		EnrichmentBackfillArgs{}.Kind(): false,
		credit.RecomputeArgs{}.Kind():   false,
	}
	if !maps.Equal(set.Scheduled, want) {
		t.Errorf("scheduled periodic jobs (kind -> RunOnStart) are %v, want %v", set.Scheduled, want)
	}
	for kind := range set.Scheduled {
		if !set.WorkerKinds[kind] {
			t.Errorf("periodic job %q is scheduled but no worker is registered for it", kind)
		}
	}
}

func TestThePartitionJobNeverDrops(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(
		"..", "..", "foundation", "persistence", "partition", "partition.go"))
	if err != nil {
		t.Fatalf("read partition.go: %v", err)
	}
	body := functionBody(t, string(src), "func CreateUpcoming(")
	for _, forbidden := range []string{"DROP", "expiredMonths"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("CreateUpcoming mentions %q; the create-only half must never remove a partition", forbidden)
		}
	}

	if !strings.Contains(string(src), "func MaintainMonthly(") ||
		!strings.Contains(functionBody(t, string(src), "func MaintainMonthly("), "DROP TABLE") {
		t.Error("MaintainMonthly no longer drops; the retention half of partition rotation has gone missing")
	}
}

func functionBody(t *testing.T, src, decl string) string {
	t.Helper()
	i := strings.Index(src, decl)
	if i < 0 {
		t.Fatalf("%q not found", decl)
	}
	var body []string
	for _, line := range strings.Split(src[i:], "\n") {
		body = append(body, line)
		if len(body) > 1 && line == "}" {
			return strings.Join(body, "\n")
		}
	}
	t.Fatalf("no closing brace for %q", decl)
	return ""
}
