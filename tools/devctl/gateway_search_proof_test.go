package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchProofConfigChecksEveryProductionRouteBeforeReplacingTheProvider(t *testing.T) {
	for _, tc := range []struct {
		name, second  string
		unguarded     []string
		unintercepted []string
	}{
		{"all routes protected", `{"model_name":"second","litellm_params":{"model":"openai/other","additional_drop_params":["web_search_options"]}}`, nil, nil},
		{"later route unguarded", `{"model_name":"second","litellm_params":{"model":"openai/other"}}`, []string{"second"}, nil},
		{"later provider not intercepted", `{"model_name":"second","litellm_params":{"model":"other/model","additional_drop_params":["web_search_options"]}}`, nil, []string{"other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"model_list":[{"model_name":"first","litellm_params":{"model":"openai/model","additional_drop_params":["web_search_options"]}},` + tc.second + `,{"model_name":"embedding","litellm_params":{"model":"openai/text-embedding-3-small"}}],"litellm_settings":{"websearch_interception_params":{"enabled_providers":["openai"]}},"general_settings":{"search_tool_deny_by_default":true}}`
			config, coverage, err := searchProofConfig([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			var missing map[string][]string
			if err := json.Unmarshal(coverage, &missing); err != nil {
				t.Fatal(err)
			}
			if strings.Join(missing["unguarded_models"], ",") != strings.Join(tc.unguarded, ",") || strings.Join(missing["unintercepted_providers"], ",") != strings.Join(tc.unintercepted, ",") {
				t.Fatalf("production routes escaped coverage: %s", coverage)
			}
			assertLocalSearchProofConfig(t, config)
		})
	}
}

func assertLocalSearchProofConfig(t *testing.T, config []byte) {
	t.Helper()
	var out struct {
		Models []struct {
			Name   string `json:"model_name"`
			Params struct {
				Model   string `json:"model"`
				APIKey  string `json:"api_key"`
				APIBase string `json:"api_base"`
			} `json:"litellm_params"`
		} `json:"model_list"`
		Settings map[string]bool `json:"general_settings"`
		Search   []any           `json:"search_tools"`
	}
	if err := json.Unmarshal(config, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Models) != 1 || out.Models[0].Name != "trial-model" || !out.Settings["search_tool_deny_by_default"] || len(out.Search) != 2 {
		t.Fatalf("the local proof changed production authorization or left live routes: %s", config)
	}
	if out.Models[0].Params.Model != "openai/gpt-4o-mini" || out.Models[0].Params.APIKey != "local-model-only" || out.Models[0].Params.APIBase != "http://mock:8000/v1" {
		t.Fatalf("the local proof did not replace the live provider: %s", config)
	}
}

func TestSearchProofRefusesMissingRouteEvidence(t *testing.T) {
	for _, input := range []string{`{`, `{}`, `{"model_list":[]}`, `{"model_list":[{"model_name":"broken","litellm_params":{}}]}`} {
		if _, _, err := searchProofConfig([]byte(input)); err == nil {
			t.Fatalf("missing production route evidence was accepted: %s", input)
		}
	}
}

func TestSearchProofStatsRequireAuthorizedSearchAndPreserveOrdinaryTools(t *testing.T) {
	for _, tc := range []struct{ name, calls, models, problem string }{
		{"complete evidence", `[{"query":"local source proof","path":"/search"}]`, `[{"tools":[{"type":"function","function":{"name":"format_result"}}]}]`, ""},
		{"no backend calls", `[]`, `[]`, "unexpected backend search calls"},
		{"unexpected backend query", `[{"query":"unapproved","path":"/search"}]`, `[]`, "unexpected backend search calls"},
		{"ordinary tools were lost", `[{"query":"local source proof","path":"/search"}]`, `[]`, "ordinary function tools did not reach"},
		{"native option forwarded", `[{"query":"local source proof","path":"/search"}]`, `[{"web_search_options":{}}]`, "native search parameters reached"},
		{"native tool forwarded", `[{"query":"local source proof","path":"/search"}]`, `[{"tools":[{"type":"web_search"}]}]`, "native search parameters reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &searchProof{dir: t.TempDir(), out: io.Discard}
			if err := os.WriteFile(filepath.Join(p.dir, "native-search-coverage.json"), []byte(`{"unguarded_models":[],"unintercepted_providers":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			err := p.checkStats([]byte(`{"calls":` + tc.calls + `,"model_calls":` + tc.models + `}`))
			if tc.problem == "" && err != nil || tc.problem != "" && (err == nil || !strings.Contains(err.Error(), tc.problem)) {
				t.Fatalf("backend observations produced the wrong verdict: %v", err)
			}
		})
	}
}

func TestSearchProofRejectsUnknownArgumentsBeforeStartingDocker(t *testing.T) {
	root := t.TempDir()
	code, err := runCommand(root, "gateway-search-proof", []string{"unknown"})
	if code != 0 || err == nil || err.Error() != "usage: devctl gateway-search-proof" {
		t.Fatalf("unexpected command dispatch: code=%d err=%v", code, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".devctl")); !os.IsNotExist(err) {
		t.Fatal("argument rejection created proof resources")
	}
}
