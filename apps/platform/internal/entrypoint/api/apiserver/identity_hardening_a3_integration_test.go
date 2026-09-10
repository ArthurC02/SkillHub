package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
)

func TestAnOperatorRefusalIsAudited(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	client := a.login(t, "operator-refusal-probe")

	resp, err := client.Get(a.URL + "/admin/dispatch")
	if err != nil {
		t.Fatalf("GET /admin/dispatch: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an operator route answered %d to a non-operator; SEC-011 requires 404", resp.StatusCode)
	}

	var rows int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE action = $1 AND resource_type = $2 AND actor_user_id IS NOT NULL`,
		audit.ActionOperatorRefused, audit.ResourceOperatorRoute).Scan(&rows); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if rows == 0 {
		t.Fatal("a signed-in account was refused an operator route and nothing recorded it")
	}

	var metadata []byte
	if err := pool.QueryRow(ctx, `
		SELECT metadata FROM audit_events WHERE action = $1 ORDER BY created_at DESC LIMIT 1`,
		audit.ActionOperatorRefused).Scan(&metadata); err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err != nil {
		t.Fatalf("metadata is not an object: %v", err)
	}
	if route, _ := meta["route"].(string); route == "" {
		t.Errorf("the audit row does not name the route it refused: %v", meta)
	}
}

func TestAnAnonymousOperatorProbeIsNotAudited(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	var before int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE action = $1`, audit.ActionOperatorRefused).Scan(&before); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(a.URL + "/admin/dispatch")
	if err != nil {
		t.Fatalf("GET /admin/dispatch: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("anonymous probe answered %d, want 404", resp.StatusCode)
	}
	var after int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE action = $1`, audit.ActionOperatorRefused).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("an anonymous 404 wrote %d audit rows; there is no actor to name, and a row per probe makes any scanner an amplifier", after-before)
	}
}

func TestMeDisclosesCleanModeToAnUninvitedCallerAndStillGatesTheEntryPoint(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {

		d.Auth.Invited = map[string]bool{"somebody-else": true}
		d.Auth.Features = map[string]bool{"generate_skill": true}
		d.Auth.Disclosures = map[string]bool{"clean_mode": true}
	})
	client := a.login(t, "uninvited-clean-mode-visitor")

	resp, err := client.Get(a.URL + "/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Features map[string]bool `json:"features"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	if !body.Features["clean_mode"] {
		t.Error("an uninvited caller was not told this deployment is in clean mode; a disclosure is not a permission")
	}
	if _, present := body.Features["generate_skill"]; present {
		t.Error("an uninvited caller was shown the generation entry point, which POST /skills/generate would refuse")
	}
}

func TestOneAccountCannotHaveTwoWorkspaces(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	svc := &identity.Service{Pool: pool}

	token, err := svc.LoginOrSignup(ctx, identity.ExternalIdentity{
		Provider: "github", ProviderUserID: "two-workspaces",
		Email: "two-workspaces@example.test", Name: "Two", Login: "two",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	user, err := svc.UserForToken(ctx, token)
	if err != nil {
		t.Fatalf("UserForToken: %v", err)
	}
	if _, err := svc.PersonalWorkspace(ctx, user); err != nil {
		t.Fatalf("one workspace must still resolve: %v", err)
	}

	var extra pgtype.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO workspaces (owner_user_id, name) VALUES ($1, $2) RETURNING id`,
		user.ID, "second").Scan(&extra)
	if err == nil {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, extra)
		t.Fatal("a second workspace was accepted for one owner; ADR-011's 1:1 is held by " +
			"workspaces_owner_user_id_key (0002) and every workspace scope is derived from it")
	}
	if !strings.Contains(err.Error(), "workspaces_owner_user_id_key") {
		t.Errorf("the second workspace was refused by something other than the unique index: %v", err)
	}
}
