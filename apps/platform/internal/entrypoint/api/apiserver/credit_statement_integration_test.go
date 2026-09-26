package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type statementBody struct {
	Entries []struct {
		ID           string `json:"id"`
		Kind         string `json:"kind"`
		Label        string `json:"label"`
		DeltaCredits int64  `json:"delta_credits"`
		Estimated    bool   `json:"estimated"`
		RunID        string `json:"run_id"`
	} `json:"entries"`
	NextBefore string `json:"next_before"`
	Note       string `json:"note"`
}

func getStatement(t *testing.T, c *client, before string) (int, statementBody, map[string]any) {
	t.Helper()
	target := c.base + "/me/credits/entries"
	if before != "" {
		target += "?before=" + url.QueryEscape(before)
	}
	resp, err := c.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(raw)
	var body statementBody
	_ = json.Unmarshal(encoded, &body)
	return resp.StatusCode, body, raw
}

func seedRunDebit(t *testing.T, pool *pgxpool.Pool, c *client, runID string, credits int64, at string) {
	t.Helper()
	ctx := context.Background()
	var costID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cost_events (kind, model, usd_micros, cost_source, workspace_id, user_id, ref_type, ref_id, idempotency_key)
		VALUES ('run', 'fixture-model', 1000, 'gateway', $1, $2, 'run', $3, gen_random_uuid()::text)
		RETURNING id`, mustUUID(t, c.workspaceID), mustUUID(t, c.userID), mustUUID(t, runID)).Scan(&costID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO credit_entries (user_id, kind, delta_credits, usd_micros, markup_bps, ref_type, ref_id,
		                            cost_event_id, idempotency_key, created_at)
		VALUES ($1, 'debit', $2, 1000, 0, 'run', $3, $4, gen_random_uuid()::text, $5::timestamptz)`,
		mustUUID(t, c.userID), -credits, mustUUID(t, runID), mustUUID(t, costID), at); err != nil {
		t.Fatal(err)
	}
}

func seedGrants(t *testing.T, pool *pgxpool.Pool, c *client, n int, at string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO credit_entries (user_id, kind, delta_credits, ref_type, idempotency_key, created_at)
		SELECT $1, 'grant', 1, 'operator_grant', gen_random_uuid()::text, $3::timestamptz
		FROM generate_series(1, $2)`, mustUUID(t, c.userID), n, at); err != nil {
		t.Fatal(err)
	}
}

func TestTheStatementLinksOnlyRunsTheReaderCanOpen(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "statement-owner")
	stranger := a.login(t, "statement-stranger")

	ownRun := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "statement-own-skill"))
	strangerRun := seedRun(t, pool, stranger.workspaceID, seedSkill(t, pool, stranger.workspaceID, "statement-other-skill"))
	missingRun := "00000000-0000-4000-8000-00000000abcd"

	seedGrants(t, pool, owner, 1, "2026-09-01T00:00:00Z")
	seedRunDebit(t, pool, owner, ownRun, 7, "2026-09-02T00:00:00Z")
	seedRunDebit(t, pool, owner, strangerRun, 5, "2026-09-03T00:00:00Z")
	seedRunDebit(t, pool, owner, missingRun, 3, "2026-09-04T00:00:00Z")

	code, body, raw := getStatement(t, owner, "")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits/entries: want 200, got %d: %v", code, raw)
	}
	if len(body.Entries) != 4 {
		t.Fatalf("entries = %+v, want 4", body.Entries)
	}
	byDelta := map[int64]string{}
	for _, e := range body.Entries {
		byDelta[e.DeltaCredits] = e.RunID
	}
	if byDelta[-7] != ownRun {
		t.Errorf("the debit for the reader's own run links to %q, want %s", byDelta[-7], ownRun)
	}
	if byDelta[-5] != "" {
		t.Errorf("a debit whose run belongs to another workspace links to %q; a link the reader cannot open is a dead end", byDelta[-5])
	}
	if byDelta[-3] != "" {
		t.Errorf("a debit whose run no longer exists links to %q", byDelta[-3])
	}
	if body.Entries[0].DeltaCredits != -3 || body.Entries[3].Kind != "grant" {
		t.Errorf("entries are not newest first: %+v", body.Entries)
	}
	if body.Entries[3].Label != "營運者授予" || body.Entries[0].Label != "試跑" {
		t.Errorf("labels = %q / %q, want 營運者授予 and 試跑", body.Entries[3].Label, body.Entries[0].Label)
	}
	if body.Note == "" {
		t.Error("the statement arrived without saying which number is authoritative")
	}
	encoded, _ := json.Marshal(raw)
	if strings.Contains(strings.ToLower(string(encoded)), "usd") || strings.Contains(string(encoded), "markup") {
		t.Errorf("the statement carries a dollar or markup field to a user: %s", encoded)
	}
}

func TestTheStatementPagesWithoutLosingEntriesThatShareATimestamp(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "statement-pages")

	seedGrants(t, pool, owner, 51, "2026-09-10T00:00:00Z")

	code, first, _ := getStatement(t, owner, "")
	if code != http.StatusOK || len(first.Entries) != 50 {
		t.Fatalf("first page: code %d, %d entries, want 200 and 50", code, len(first.Entries))
	}
	if first.NextBefore == "" {
		t.Fatal("a statement longer than one page gave no way to the next one")
	}
	code, second, _ := getStatement(t, owner, first.NextBefore)
	if code != http.StatusOK || len(second.Entries) != 1 {
		t.Fatalf("second page: code %d, %d entries, want 200 and 1", code, len(second.Entries))
	}
	if second.NextBefore != "" {
		t.Errorf("the last page still offers a next page: %q", second.NextBefore)
	}
	seen := map[string]bool{}
	for _, e := range append(first.Entries, second.Entries...) {
		if seen[e.ID] {
			t.Errorf("entry %s appeared on both pages", e.ID)
		}
		seen[e.ID] = true
	}
	if len(seen) != 51 {
		t.Errorf("the two pages together hold %d distinct entries, want 51", len(seen))
	}
}

func TestAStatementWithExactlyOnePageOffersNoNextPage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "statement-exact")

	seedGrants(t, pool, owner, 50, "2026-09-10T00:00:00Z")
	_, body, _ := getStatement(t, owner, "")
	if len(body.Entries) != 50 || body.NextBefore != "" {
		t.Errorf("50 entries: got %d with next %q, want 50 and no next page", len(body.Entries), body.NextBefore)
	}
}

func TestAStatementCursorThisEndpointDidNotIssueIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "statement-cursor")

	for _, before := range []string{"yesterday", "2026-09-10T00:00:00Z", fmt.Sprintf("2026-09-10T00:00:00Z_%s", "not-a-uuid")} {
		if code, _, _ := getStatement(t, owner, before); code != http.StatusBadRequest {
			t.Errorf("before=%q: want 400, got %d", before, code)
		}
	}
}
