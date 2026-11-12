package run

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestASearchGrantUsesTheSameScopedAttemptKeyAndBudget(t *testing.T) {
	server := gatewayAnsweringTeamCreation(t, http.StatusOK, `{}`)
	server.gateway.model = "trial-model"
	grant, err := server.gateway.Issue(t.Context(), "run", "attempt", AttemptKeyTerms{TTL: time.Minute, BudgetUSD: .12, TPMLimit: 1234, SearchAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	request := server.keyRequests[0]
	if grant.SearchTool != "skillhub-web-search" || grant.VirtualKey != "sk-virtual-test" || len(server.keyRequests) != 1 || request["max_budget"] != .12 || request["tpm_limit"] != float64(1234) || request["duration"] != "60s" || !reflect.DeepEqual(request["models"], []any{"trial-model"}) {
		t.Fatalf("search changed the attempt key or its existing brakes: grant=%+v request=%v", grant, request)
	}
	permission, _ := json.Marshal(request["object_permission"])
	teamPermission, _ := json.Marshal(server.teamsAsked[0]["object_permission"])
	if string(permission) != `{"search_tools":["skillhub-web-search"]}` || string(teamPermission) != string(permission) {
		t.Fatalf("search grant named a different or broader tool: key=%s team=%s", permission, teamPermission)
	}
}

func TestSearchGrantDoesNotRewriteAnExistingGatewayTeam(t *testing.T) {
	var paths []string
	var key map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/team/new" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"team already exists"}}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&key); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"key":"scoped-search-key"}`))
	}))
	defer srv.Close()
	gateway := &Gateway{adminBaseURL: srv.URL, client: srv.Client()}
	_, err := gateway.Issue(t.Context(), "run", "attempt", AttemptKeyTerms{TTL: time.Minute, SearchAllowed: true})
	if err != nil || !reflect.DeepEqual(paths, []string{"/team/new", "/key/generate"}) || key["team_id"] != "skillhub-run" {
		t.Fatalf("the key issuer rewrote an existing team's permissions: paths=%v err=%v", paths, err)
	}
}

func TestOrdinaryRunCreationAndAgentKeysExplicitlyGrantNoSearchTool(t *testing.T) {
	for _, entry := range []string{"trial", "creation", "platform agent"} {
		t.Run(entry, func(t *testing.T) {
			server := gatewayAnsweringTeamCreation(t, http.StatusOK, `{}`)
			var err error
			switch entry {
			case "trial":
				_, err = server.gateway.Issue(context.Background(), "run", "attempt", AttemptKeyTerms{TTL: time.Minute})
			case "creation":
				_, err = server.gateway.IssueCreationForModel(context.Background(), "session", "attempt", CreationKeyTerms{TTL: time.Minute, Model: "creation"})
			case "platform agent":
				_, err = server.gateway.IssueAgentRun(context.Background(), "agent", CreationKeyTerms{TTL: time.Minute, Model: "agent"})
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(server.keyRequests[0]["object_permission"])
			if string(encoded) != `{"search_tools":[]}` {
				t.Fatalf("an ordinary key did not explicitly deny search: %s", encoded)
			}
		})
	}
}
