package run

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestEgressAllowIsRenderedAsPurposeAndURL(t *testing.T) {
	lines := egressAllowLines([]egressAllow{
		{Purpose: "model_gateway", URL: "https://gateway.internal/v1"},
		{Purpose: "artifact_upload", URL: "https://objects.internal/put"},
	})
	want := []string{
		"model_gateway: https://gateway.internal/v1",
		"artifact_upload: https://objects.internal/put",
	}
	if len(lines) != len(want) {
		t.Fatalf("rendered %d lines, want %d: %v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}

	if got := egressAllowLines(nil); got == nil || len(got) != 0 {
		t.Errorf("empty allow list rendered as %#v, want an empty slice", got)
	}
}

func providerDeclaring(t *testing.T, injects string) *Service {
	t.Helper()
	body := `{"provider":"declared","runtimes":[{"runtime":"claude_agent_sdk","versions":["0.1.0"],` +
		`"agent_integration":["in_sandbox_sdk"]}],"max_resources":` + declaredResourcesJSON() +
		`,"isolation":{"strength":"strong","rootless":true,"dedicated_workspace_per_run":true},` +
		`"network":{"egress_modes":["default_deny"],"private_network":true},` +
		`"availability":{"concurrent_run_slots":4,"healthy":true},"injects":` + injects + `}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Service{Providers: NewRegistry(NewProvider("declared", srv.URL, ""))}
}

func declaredResourcesJSON() string {
	limits, err := json.Marshal(DefaultResourceLimits())
	if err != nil {
		panic(err)
	}
	return string(limits)
}

func withGatewayGrant() policySnapshot {
	return policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "model_gateway", URL: "http://gateway.invalid"}},
	}}
}

func TestTheSummaryNamesTheSecretsTheProviderSaysItInjects(t *testing.T) {
	svc := providerDeclaring(t, `["SOMETHING_ELSE","ANOTHER_ONE"]`)

	got := svc.injectedSecretsFor(context.Background(), withGatewayGrant())
	if want := []string{"ANOTHER_ONE", "SOMETHING_ELSE"}; !slices.Equal(got, want) {
		t.Errorf("the summary claims %v, want %v: it is a claim about what the sandbox is given, "+
			"so only the sandbox can make it", got, want)
	}
}

func TestNoGatewayGrantMeansNothingFromItIsInjected(t *testing.T) {
	svc := providerDeclaring(t, `["ANTHROPIC_BASE_URL"]`)

	none := policySnapshot{Egress: EgressPolicy{Mode: "default_deny", Allow: []egressAllow{}}}
	got := svc.injectedSecretsFor(context.Background(), none)
	if len(got) != 0 {
		t.Errorf("no gateway means nothing from it is injected, but the summary claims %v", got)
	}
	if got == nil {
		t.Error("an empty disclosure must still be a list; a missing row reads as a question never asked")
	}

	other := policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "something_else", URL: "http://elsewhere.invalid"}},
	}}
	if got := svc.injectedSecretsFor(context.Background(), other); len(got) != 0 {
		t.Errorf("only a model_gateway grant injects these, got %v", got)
	}
}

func TestAProviderThatCannotBeAskedClaimsNoSecrets(t *testing.T) {
	svc := &Service{Providers: NewRegistry()}

	if got := svc.injectedSecretsFor(context.Background(), withGatewayGrant()); len(got) != 0 {
		t.Errorf("with nothing to ask, the summary still claims %v; an unverifiable claim about "+
			"secrets must not be made", got)
	}
}

func TestPreflightMissingVersionIsPreflightTargetNotFound(t *testing.T) {
	svc := &Service{
		TestLab: &testlab.Service{},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{}, false, nil
		},
	}
	var ws, skill, versionID, testCaseID pgtype.UUID
	for _, u := range []*pgtype.UUID{&ws, &skill, &versionID, &testCaseID} {
		if err := u.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
			t.Fatal(err)
		}
	}

	_, err := svc.PermissionSummaryFor(context.Background(), ws, skill, versionID, testCaseID)
	if !errors.Is(err, ErrPreflightTargetNotFound) {
		t.Fatalf("err = %v, want ErrPreflightTargetNotFound", err)
	}
	if strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		t.Errorf("a domain sentinel is carrying interface copy: %q", err)
	}
	if got := notFoundMessage(err); got != "找不到這個 Skill 版本或 Test Case" {
		t.Errorf("404 body = %q; the handler owns the sentence a reader sees", got)
	}
}
