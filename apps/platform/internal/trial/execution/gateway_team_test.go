package run

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type teamGateway struct {
	gateway     *Gateway
	teamsAsked  []map[string]any
	keyRequests []map[string]any
}

func gatewayAnsweringTeamCreation(t *testing.T, status int, body string) *teamGateway {
	t.Helper()
	tg := &teamGateway{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /team/new", func(w http.ResponseWriter, r *http.Request) {
		var asked map[string]any
		_ = json.NewDecoder(r.Body).Decode(&asked)
		tg.teamsAsked = append(tg.teamsAsked, asked)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("POST /key/generate", func(w http.ResponseWriter, r *http.Request) {
		var asked map[string]any
		_ = json.NewDecoder(r.Body).Decode(&asked)
		tg.keyRequests = append(tg.keyRequests, asked)
		_, _ = w.Write([]byte(`{"key":"sk-virtual-test"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	tg.gateway = &Gateway{adminBaseURL: srv.URL, adminKey: "sk-master-test", client: srv.Client()}
	return tg
}

func issueRunKey(t *testing.T, g *Gateway) {
	t.Helper()
	if _, err := g.Issue(context.Background(), "run-1", "attempt-1", AttemptKeyTerms{TTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
}

func TestEachEntryIssuesItsKeysInsideItsOwnGatewayTeam(t *testing.T) {
	terms := CreationKeyTerms{TTL: time.Minute, BudgetUSD: .1, Model: "gpt-5.4-mini"}
	for _, tc := range []struct {
		entry string
		issue func(*Gateway) error
		team  string
	}{
		{"trial run", func(g *Gateway) error {
			_, err := g.Issue(context.Background(), "run-1", "attempt-1", AttemptKeyTerms{TTL: time.Minute})
			return err
		}, "skillhub-run"},
		{"creation step", func(g *Gateway) error {
			_, err := g.IssueCreationForModel(context.Background(), "session-1", "receipt-1", terms)
			return err
		}, "skillhub-creation"},
		{"platform agent", func(g *Gateway) error {
			_, err := g.IssueAgentRun(context.Background(), "agent-run-1", terms)
			return err
		}, "skillhub-platform-agent"},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			tg := gatewayAnsweringTeamCreation(t, http.StatusOK, `{}`)

			if err := tc.issue(tg.gateway); err != nil {
				t.Fatal(err)
			}

			if len(tg.teamsAsked) != 1 || tg.teamsAsked[0]["team_id"] != tc.team || tg.teamsAsked[0]["budget_duration"] != "1d" {
				t.Errorf("team creation = %v, want one request for %s resetting daily", tg.teamsAsked, tc.team)
			}
			if len(tg.keyRequests) != 1 || tg.keyRequests[0]["team_id"] != tc.team {
				t.Errorf("key request = %v, want it issued inside %s", tg.keyRequests, tc.team)
			}
		})
	}
}

func TestATeamTheGatewayAlreadyHasStillHoldsTheKey(t *testing.T) {
	tg := gatewayAnsweringTeamCreation(t, http.StatusBadRequest,
		`{"error":{"message":"{'error': 'Team id = skillhub-run already exists. Please use a different team id.'}","code":"400"}}`)

	issueRunKey(t, tg.gateway)

	if tg.keyRequests[0]["team_id"] != "skillhub-run" {
		t.Errorf("key request = %v, want it inside the existing skillhub-run team", tg.keyRequests[0])
	}
}

func TestAKeyIsIssuedOutsideATeamTheGatewayCouldNotCreate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"refused for another reason", http.StatusBadRequest, `{"error":{"message":"budget_duration is invalid"}}`},
		{"failing inside", http.StatusInternalServerError, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tg := gatewayAnsweringTeamCreation(t, tc.status, tc.body)

			issueRunKey(t, tg.gateway)

			if _, inTeam := tg.keyRequests[0]["team_id"]; inTeam {
				t.Errorf("key request = %v, want no team_id: a key inside a team the gateway lacks is refused on every call", tg.keyRequests[0])
			}
		})
	}
}

func TestAGatewayTeamIsCreatedOnceAndRetriedUntilItExists(t *testing.T) {
	ready := gatewayAnsweringTeamCreation(t, http.StatusOK, `{}`)
	issueRunKey(t, ready.gateway)
	issueRunKey(t, ready.gateway)
	if len(ready.teamsAsked) != 1 {
		t.Errorf("a created team was asked for %d times over two keys, want 1", len(ready.teamsAsked))
	}

	failing := gatewayAnsweringTeamCreation(t, http.StatusInternalServerError, `{}`)
	issueRunKey(t, failing.gateway)
	issueRunKey(t, failing.gateway)
	if len(failing.teamsAsked) != 2 {
		t.Errorf("a team the gateway could not create was asked for %d times over two keys, want 2", len(failing.teamsAsked))
	}
}
