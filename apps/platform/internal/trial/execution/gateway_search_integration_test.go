package run

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func searchProofGateway(t *testing.T) *Gateway {
	t.Helper()
	endpoint := os.Getenv("SKILLHUB_TEST_SEARCH_GATEWAY_URL")
	if endpoint == "" {
		if os.Getenv("SKILLHUB_REQUIRE_SEARCH_GATEWAY") == "1" {
			t.Fatal("the local fake-provider search gateway is required")
		}
		t.Skip("SKILLHUB_TEST_SEARCH_GATEWAY_URL selects the local gateway with an isolated fake search provider; no paid provider is used")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
		t.Fatal("search proof requires the disposable local gateway on 127.0.0.1")
	}
	return NewGateway(GatewayConfig{AdminBaseURL: endpoint, SandboxBaseURL: endpoint,
		AdminKey: "sk-local-search-proof-master", Model: "trial-model", HTTP: &http.Client{Timeout: 10 * time.Second}})
}

func searchProofCall(t *testing.T, gateway *Gateway, token, tool string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.sandboxBaseURL+"/v1/search/"+tool,
		strings.NewReader(`{"query":"local source proof","max_results":1}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := gateway.client.Do(req)
	if err != nil {
		t.Fatal("local search request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, body
}

func TestRealGatewaySearchUsesScopedKeysAndReportsGatewaySpend(t *testing.T) {
	gateway := searchProofGateway(t)
	if err := gateway.post(t.Context(), "/team/new", teamCreationRequest{TeamID: string(runTeam), TeamAlias: string(runTeam), BudgetDuration: "1d"}, nil); err != nil {
		t.Fatal(err)
	}
	denied, err := gateway.Issue(t.Context(), "search-proof-run", "search-team-denied", AttemptKeyTerms{TTL: 2 * time.Minute, SearchAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := searchProofCall(t, gateway, denied.VirtualKey, RunSearchToolName); code != http.StatusForbidden {
		t.Fatalf("an existing team without a search grant reached the provider: HTTP %d", code)
	}
	if err := gateway.post(t.Context(), "/team/update", map[string]any{"team_id": string(runTeam), "object_permission": gatewayObjectPermission{SearchTools: []string{RunSearchToolName}}}, nil); err != nil {
		t.Fatal(err)
	}
	grant, err := gateway.Issue(t.Context(), "search-proof-run", "search-allowed", AttemptKeyTerms{TTL: 2 * time.Minute, SearchAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	code, headers, body := searchProofCall(t, gateway, grant.VirtualKey, RunSearchToolName)
	if code != http.StatusOK || !strings.Contains(string(body), "https://example.test/checked-source") {
		t.Fatalf("the scoped search did not return the local provider's source: HTTP %d", code)
	}
	cost := reportedSearchCost(t, headers)
	assertSearchSpendRecorded(t, gateway, cost)
	assertSearchCannotExceedTheAttemptBudget(t, gateway)
	if code, _, _ := searchProofCall(t, gateway, grant.VirtualKey, "other-search"); code != http.StatusForbidden {
		t.Fatalf("the scoped key reached a different registered search tool: HTTP %d", code)
	}
	assertOtherKeysCannotSearch(t, gateway)
	if err := gateway.Revoke(t.Context(), "search-allowed"); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := searchProofCall(t, gateway, grant.VirtualKey, RunSearchToolName); code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("a revoked search key still worked: HTTP %d", code)
	}
}

func reportedSearchCost(t *testing.T, headers http.Header) float64 {
	t.Helper()
	cost, err := strconv.ParseFloat(headers.Get("x-litellm-response-cost"), 64)
	if err != nil || cost <= 0 || math.IsInf(cost, 0) || math.IsNaN(cost) {
		t.Fatal("the gateway did not report a finite positive search cost")
	}
	return cost
}

func assertSearchCannotExceedTheAttemptBudget(t *testing.T, gateway *Gateway) {
	t.Helper()
	grant, err := gateway.Issue(t.Context(), "search-proof-run", "search-budget-exhausted", AttemptKeyTerms{TTL: time.Minute, BudgetUSD: .001, SearchAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.post(t.Context(), "/key/update", map[string]any{"key": grant.VirtualKey, "spend": .01}, nil); err != nil {
		t.Fatal(err)
	}
	code, _, body := searchProofCall(t, gateway, grant.VirtualKey, RunSearchToolName)
	if code < 400 || code >= 500 || !strings.Contains(strings.ToLower(string(body)), "budget") {
		t.Fatalf("search bypassed the attempt's exhausted budget: HTTP %d", code)
	}
}

func assertSearchSpendRecorded(t *testing.T, gateway *Gateway, cost float64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		usage, err := gateway.Usage(t.Context(), "search-allowed", time.Now().Add(-time.Hour))
		if err == nil && usage.CostReported && !usage.Incomplete && math.Abs(usage.ModelCostUSD-cost) < 1e-12 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the reported search cost never reached the attempt's spend records: %+v", usage)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertOtherKeysCannotSearch(t *testing.T, gateway *Gateway) {
	t.Helper()
	terms := CreationKeyTerms{TTL: 2 * time.Minute, Model: "trial-model"}
	runKey, runErr := gateway.Issue(t.Context(), "search-proof-run", "ordinary-run", AttemptKeyTerms{TTL: 2 * time.Minute})
	creationKey, creationErr := gateway.IssueCreationForModel(t.Context(), "session", "creation-proof", terms)
	agentKey, agentErr := gateway.IssueAgentRun(t.Context(), "agent-proof", terms)
	if runErr != nil || creationErr != nil || agentErr != nil {
		t.Fatal("could not issue the negative-control keys")
	}
	for _, grant := range []*ModelGatewayGrant{runKey, creationKey, agentKey} {
		if code, _, _ := searchProofCall(t, gateway, grant.VirtualKey, RunSearchToolName); code != http.StatusForbidden {
			t.Errorf("a key without explicit search permission reached the provider: HTTP %d", code)
		}
		assertNativeSearchDenied(t, gateway, grant.VirtualKey)
	}
	assertModelCallStillWorks(t, gateway, runKey.VirtualKey)
	encoded, _ := json.Marshal([]string{runKey.SearchTool, creationKey.SearchTool, agentKey.SearchTool})
	if string(encoded) != `["","",""]` {
		t.Error("a negative-control key advertised search access")
	}
}

func assertNativeSearchDenied(t *testing.T, gateway *Gateway, token string) {
	t.Helper()
	for _, probe := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"trial-model","input":"local native search probe","tools":[{"type":"web_search_preview"}]}`},
		{"/v1/responses", `{"model":"trial-model","input":"local native search probe","tools":[{"type":"web_search"}],"guardrails":[]}`},
		{"/v1/chat/completions", `{"model":"trial-model","messages":[{"role":"user","content":"local native search probe"}],"web_search_options":{}}`},
		{"/v1/chat/completions", `{"model":"trial-model","messages":[{"role":"user","content":"local native search probe"}],"web_search_options":{},"guardrails":[]}`},
		{"/v1/messages", `{"model":"trial-model","max_tokens":8,"messages":[{"role":"user","content":"local native search probe"}],"tools":[{"name":"web_search","type":"web_search_20250305","max_uses":1}]}`},
	} {
		code, body := modelProofResponse(t, gateway, token, probe.path, probe.body)
		droppedOption := strings.Contains(probe.body, "web_search_options") && strings.Contains(string(body), "local model proof")
		if code != http.StatusForbidden && !strings.Contains(string(body), "not allowed to access search tool") && !droppedOption {
			t.Errorf("an ungranted key requested native search on %s: HTTP %d", probe.path, code)
		}
	}
}

func assertModelCallStillWorks(t *testing.T, gateway *Gateway, token string) {
	t.Helper()
	for _, body := range []string{
		`{"model":"trial-model","messages":[{"role":"user","content":"local model proof"}],"max_tokens":8}`,
		`{"model":"trial-model","messages":[{"role":"user","content":"local model proof"}],"tools":[{"type":"function","function":{"name":"format_result","parameters":{"type":"object","properties":{}}}}],"max_tokens":8}`,
	} {
		if code, _ := modelProofResponse(t, gateway, token, "/v1/chat/completions", body); code != http.StatusOK {
			t.Fatalf("the search guard also disabled an ordinary model call: HTTP %d", code)
		}
	}
}

func modelProofResponse(t *testing.T, gateway *Gateway, token, path, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.sandboxBaseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	response, err := gateway.client.Do(req)
	if err != nil {
		t.Fatal("local model API probe failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}
