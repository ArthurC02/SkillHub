package apiserver_test

// Three identity-layer facts that had no assertion: the operator refusal leaves
// no trace, /me gates a disclosure on an entitlement, and PersonalWorkspace picks
// a workspace when ADR-011's invariant has broken instead of saying so.

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

// RequireOperator answers 404 so that an operator endpoint's existence is not
// disclosed (02:SEC-011), which means a probe leaves no trace on the outside —
// and, until now, none on the inside either. "Has anybody been trying the
// dispatch halt" had no answer at all.
//
// The asymmetry is the design: only the caller with a resolvable session writes
// a row, because only they can be named. An unauthenticated 404 writes nothing,
// or any path scanner becomes an amplifier against a 400-day table.
func TestAnOperatorRefusalIsAudited(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool) // no OPERATOR_USER_IDS: nobody is an operator
	client := a.login(t, "operator-refusal-probe")

	// A real operator route, reached by a signed-in account that is not one.
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

	// The metadata is the route pattern, never the URL: this row exists for
	// probes, and a probe's path is attacker-chosen (iron rule 11).
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

// An anonymous probe is refused identically and writes nothing.
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

// public.yaml, verbatim: "generate_skill (ADR-052) is an entry point: it says a
// route exists. clean_mode (ADR-060) is not... A client that treats clean_mode as
// something to unlock has read it backwards." The server was doing exactly that:
// /me handed out the whole features map only past the BETA-001 invite check, so
// an uninvited visitor on a clean-mode deployment was not told the environment
// has no isolation and verifies no signature.
func TestMeDisclosesCleanModeToAnUninvitedCallerAndStillGatesTheEntryPoint(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		// A closed beta this caller is not in.
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

// ADR-011 gives an account exactly one workspace, and PersonalWorkspace is where
// every workspace scope in the platform comes from (iron rule 3): /me, feedback,
// the download funnel. ListWorkspacesByOwner orders by created_at with no
// tie-breaker, so if two rows ever existed for one owner the "first" would be
// whichever Postgres returned — a request's scope, non-deterministic and silent.
//
// The 2026-08-29 audit filed this as "does a unique constraint exist? 未查證",
// with severity resting on the answer. It does: 0002 creates
// workspaces_owner_user_id_key, so the state is unreachable and this is the
// assertion with teeth — the app-side len(ws)>1 refusal is defence behind it and
// cannot be exercised while the index stands.
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

	// The database is what holds ADR-011's 1:1, not signup's good behaviour.
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
