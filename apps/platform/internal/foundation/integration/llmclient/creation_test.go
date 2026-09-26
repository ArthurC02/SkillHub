package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreationStepSendsBothCredentials(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer service" {
			t.Fatalf("authorization=%q", got)
		}
		if got := r.Header.Get("X-Creation-Gateway-Key"); got != "short-lived" {
			t.Fatalf("gateway key=%q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"messages", "references", "allowed_tools"} {
			if _, ok := body[key].([]any); !ok {
				t.Fatalf("%s was not an array: %#v", key, body[key])
			}
		}
		_, _ = w.Write([]byte(`{"outcome":"clarification","message":"need detail","brief":"","diagram_understanding":"","model":"m","prompt_version":"p"}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, Token: "service"}
	if _, err := c.CreationStep(context.Background(), CreationStepRequest{GatewayKey: "short-lived"}); err != nil {
		t.Fatal(err)
	}
}

func TestALinearDiagramInterpretationShipsEmptyListsAsArrays(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DiagramInterpretation map[string]any `json:"diagram_interpretation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"nodes", "conditions", "branches", "uncertainties"} {
			if _, ok := body.DiagramInterpretation[key].([]any); !ok {
				t.Errorf("%s was not an array: %#v", key, body.DiagramInterpretation[key])
			}
		}
		_, _ = w.Write([]byte(`{"outcome":"clarification","message":"need detail","brief":"","diagram_understanding":"","model":"m","prompt_version":"p"}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, Token: "service"}
	in := CreationStepRequest{GatewayKey: "short-lived", DiagramInterpretation: &DiagramInterpretation{Nodes: []string{"receive", "send"}}}
	if _, err := c.CreationStep(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func TestAFailedCreationStepNamesOnlyAFixedReason(t *testing.T) {
	cases := []struct {
		name, body, want, mustNotContain string
		status                           int
	}{
		{name: "a fixed reason is carried", status: http.StatusBadGateway, body: `{"detail":"creation model returned unusable output"}`, want: "creation step returned 502: creation model returned unusable output"},
		{name: "a validation echo is dropped", status: http.StatusUnprocessableEntity, body: `{"detail":[{"loc":["body","messages"],"input":"user wrote a secret"}]}`, want: "creation step returned 422", mustNotContain: "secret"},
		{name: "a body that is not json is dropped", status: http.StatusBadGateway, body: `upstream exploded: user wrote a secret`, want: "creation step returned 502", mustNotContain: "secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, err := (&Client{BaseURL: s.URL}).CreationStep(context.Background(), CreationStepRequest{GatewayKey: "k"})
			if err == nil || !strings.HasSuffix(err.Error(), tc.want) {
				t.Fatalf("err = %v, want suffix %q", err, tc.want)
			}
			if tc.mustNotContain != "" && strings.Contains(err.Error(), tc.mustNotContain) {
				t.Fatalf("err leaked the request echo: %v", err)
			}
		})
	}
}

func TestCreationStepRefusesMissingGatewayKey(t *testing.T) {
	if _, err := (&Client{}).CreationStep(context.Background(), CreationStepRequest{}); err == nil {
		t.Fatal("missing gateway key was sent")
	}
}
