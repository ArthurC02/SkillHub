package run

import (
	"context"
	"errors"
	"testing"

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

func TestInjectedSecretsFollowTheGrantAndNotAConstant(t *testing.T) {
	withGateway := policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "model_gateway", URL: "http://gateway.invalid"}},
	}}
	if got := injectedSecretsFor(withGateway); len(got) != 2 {
		t.Errorf("a run with a gateway grant receives both secrets, got %v", got)
	}

	none := policySnapshot{Egress: EgressPolicy{Mode: "default_deny", Allow: []egressAllow{}}}
	got := injectedSecretsFor(none)
	if len(got) != 0 {
		t.Errorf("no gateway means no secrets are injected, but the summary claims %v", got)
	}

	if got == nil {
		t.Error("an empty disclosure must still be a list; a missing row reads as a question never asked")
	}

	other := policySnapshot{Egress: EgressPolicy{
		Mode:  "default_deny",
		Allow: []egressAllow{{Purpose: "something_else", URL: "http://elsewhere.invalid"}},
	}}
	if got := injectedSecretsFor(other); len(got) != 0 {
		t.Errorf("only a model_gateway grant injects these, got %v", got)
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
	const want = "找不到這個 Skill 版本或 Test Case"
	if err.Error() != want {
		t.Errorf("404 body = %q, want %q", err.Error(), want)
	}
}
