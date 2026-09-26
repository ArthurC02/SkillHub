package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestCreationStepRefusesMissingGatewayKey(t *testing.T) {
	if _, err := (&Client{}).CreationStep(context.Background(), CreationStepRequest{}); err == nil {
		t.Fatal("missing gateway key was sent")
	}
}
