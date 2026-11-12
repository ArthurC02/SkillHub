package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

type searchTermsGateway struct {
	*providertest.Gateway
	terms []run.AttemptKeyTerms
}

func (g *searchTermsGateway) Issue(ctx context.Context, runID, attemptID string, terms run.AttemptKeyTerms) (*run.ModelGatewayGrant, error) {
	g.terms = append(g.terms, terms)
	grant, err := g.Gateway.Issue(ctx, runID, attemptID, terms)
	if grant != nil && terms.SearchAllowed {
		grant.SearchTool = "skillhub-web-search"
	}
	return grant, err
}

func searchRunFixture(t *testing.T, pool *pgxpool.Pool, compatibility string) (*api, fixture, *providertest.Fake, *searchTermsGateway) {
	t.Helper()
	a := newAPI(t, pool)
	owner := a.login(t, "search-owner")
	skill, version := importFiles(t, a, pool, owner, map[string]string{
		"SKILL.md": "---\nname: search-report\ndescription: Summarise public sources.\nlicense: MIT\ncompatibility: |\n  " + compatibility + "\n---\n\nUse the permitted tools.\n",
	})
	f := fixture{client: owner, skillID: skill, versionID: version, testCaseID: seedTestCase(t, pool, owner.workspaceID, skill)}
	fake := providertest.New("search-runtime", "test-token")
	t.Cleanup(fake.Close)
	a.runs.Providers = run.NewRegistry(fake.Provider())
	a.runs.Queue = nil
	gateway := &searchTermsGateway{Gateway: providertest.NewGateway()}
	a.runs.Gateway = gateway
	return a, f, fake, gateway
}

func TestADeclaredSearchRequiresRuntimeSupportAndRecordedConsent(t *testing.T) {
	pool := requireDB(t)
	a, f, fake, gateway := searchRunFixture(t, pool, "skillhub-web-search")
	code, blocked := f.preflight(t)
	if code != http.StatusOK || blocked.Blocked != "capability_mismatch" {
		t.Fatalf("old runtime preflight = %d %+v, want a search capability refusal", code, blocked)
	}
	if code, _ := f.startWithHash(t, blocked.Hash); code != http.StatusUnprocessableEntity {
		t.Fatalf("an unsupported runtime accepted a declared search: HTTP %d", code)
	}
	capability := providertest.DefaultCapability(fake.Name)
	capability.Runtimes[0].WebSearch = "supported"
	fake.Capability = &capability
	a.runs.Providers = run.NewRegistry(fake.Provider())
	code, ready := f.preflight(t)
	if code != http.StatusOK || ready.Blocked != "" {
		t.Fatalf("search-capable preflight = %d %+v", code, ready)
	}
	assertSearchDisclosure(t, ready)
	if code, _ := f.startWithHash(t, ready.Hash); code != http.StatusUnprocessableEntity {
		t.Fatalf("knowing the hash without confirming permissions started a search Run: HTTP %d", code)
	}
	if code, _ := f.confirm(t, ready.Hash); code != http.StatusCreated {
		t.Fatalf("confirm search permissions: HTTP %d", code)
	}
	if code, _ := f.startWithHash(t, strings.Repeat("0", 64)); code != http.StatusUnprocessableEntity {
		t.Fatalf("a different permission hash started a search Run: HTTP %d", code)
	}
	if len(gateway.terms) != 0 || fake.Dispatches() != 0 {
		t.Fatal("a refused or unconfirmed Run received a key or was dispatched")
	}
	code, created := f.startWithHash(t, ready.Hash)
	if code != http.StatusCreated {
		t.Fatalf("confirmed search Run = %d %+v", code, created)
	}
	assertSearchPolicySnapshot(t, pool, f, created.RunID, true)
	if err := driveThroughPolls(t.Context(), a.runs.Drive, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatal(err)
	}
	assertSearchDispatched(t, fake, gateway, created.RunID)
}

func assertSearchDisclosure(t *testing.T, summary preflightView) {
	t.Helper()
	disclosure := strings.Join(summary.Summary.Tools, "\n")
	for _, required := range []string{"公開資料搜尋", "查詢送至搜尋供應商", "私人 Trace", "Run 預算"} {
		if !strings.Contains(disclosure, required) {
			t.Fatalf("the permission summary omits %q: %s", required, disclosure)
		}
	}
}

func assertSearchDispatched(t *testing.T, fake *providertest.Fake, gateway *searchTermsGateway, id string) {
	t.Helper()
	req, found := fake.Request(id, 1)
	if len(gateway.terms) != 1 || !gateway.terms[0].SearchAllowed || !found || req.ModelGateway == nil || req.ModelGateway.SearchTool != "skillhub-web-search" {
		t.Fatalf("confirmed search did not reach the scoped attempt grant: terms=%+v request=%+v found=%v", gateway.terms, req.ModelGateway, found)
	}
}

func TestSearchIsNotGrantedWithoutTheDeclaredSkill(t *testing.T) {
	for _, tc := range []struct {
		name, compatibility string
		withheld, snapshot  bool
	}{
		{"prose is not a declaration", "Does not require skillhub-web-search", false, false},
		{"withheld skill", "skillhub-web-search", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := requireDB(t)
			a, f, fake, gateway := searchRunFixture(t, pool, tc.compatibility)
			capability := providertest.DefaultCapability(fake.Name)
			capability.Runtimes[0].WebSearch = "supported"
			fake.Capability = &capability
			hash := f.confirmPermissions(t)
			created, err := a.runs.Create(t.Context(), run.CreateParams{
				WorkspaceID: mustUUID(t, f.workspaceID), Actor: mustUUID(t, f.userID), SkillID: mustUUID(t, f.skillID),
				VersionID: mustUUID(t, f.versionID), TestCaseID: mustUUID(t, f.testCaseID),
				ConfirmedSummaryHash: hash, WithholdSkill: tc.withheld,
			})
			if err != nil {
				t.Fatal(err)
			}
			id := uuidText(created.ID)
			assertSearchPolicySnapshot(t, pool, f, id, tc.snapshot)
			if err := driveThroughPolls(t.Context(), a.runs.Drive, mustUUID(t, f.workspaceID), created.ID); err != nil {
				t.Fatal(err)
			}
			req, found := fake.Request(id, 1)
			if len(gateway.terms) != 1 || gateway.terms[0].SearchAllowed || !found || req.ModelGateway == nil || req.ModelGateway.SearchTool != "" {
				t.Fatalf("search was granted without its declared Skill: terms=%+v request=%+v found=%v", gateway.terms, req.ModelGateway, found)
			}
		})
	}
}

func assertSearchPolicySnapshot(t *testing.T, pool *pgxpool.Pool, f fixture, id string, want bool) {
	t.Helper()
	snapshot, err := gen.New(pool).GetRunSnapshots(t.Context(), gen.GetRunSnapshotsParams{
		RunID: mustUUID(t, id), WorkspaceID: mustUUID(t, f.workspaceID),
	})
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		WebSearch bool `json:"web_search"`
	}
	if err := json.Unmarshal(snapshot.PolicySnapshot, &policy); err != nil || policy.WebSearch != want {
		t.Fatalf("stored search policy = %s, want search=%v, err=%v", snapshot.PolicySnapshot, want, err)
	}
}
