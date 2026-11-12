package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type provider struct {
	mu     sync.Mutex
	calls  []map[string]any
	models []map[string]any
}

const (
	proofHTTPTimeout  = 5 * time.Second
	proofStatsLimit   = 8 << 20
	proofRequestLimit = 1 << 20
	proofInputTokens  = 4
	proofOutputTokens = 3
	proofTotalTokens  = 7
	responseContent   = "content"
	responseType      = "type"
	responseCompleted = "completed"
	responseStatus    = "status"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 3 && os.Args[1] == "--get":
		err = get(os.Args[2])
	case len(os.Args) == 1:
		err = (&http.Server{Addr: ":8000", Handler: &provider{}, ReadHeaderTimeout: proofHTTPTimeout, ReadTimeout: proofHTTPTimeout, WriteTimeout: proofHTTPTimeout}).ListenAndServe()
	default:
		err = errors.New("usage: search-proof-provider [--get URL]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func get(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	_, err = io.Copy(os.Stdout, io.LimitReader(response.Body, proofStatsLimit))
	return err
}

func (p *provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet && r.URL.Path == "/stats" {
		_ = json.NewEncoder(w).Encode(map[string]any{"calls": p.calls, "model_calls": p.models})
		return
	}
	var body map[string]any
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, proofRequestLimit))
	if err != nil || json.Unmarshal(raw, &body) != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/search" {
		p.calls = append(p.calls, map[string]any{"query": body["query"], "path": r.URL.Path})
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"title": "Checked source", "url": "https://example.test/checked-source", responseContent: "Local fake-provider evidence."}}})
		return
	}
	p.models = append(p.models, map[string]any{"path": r.URL.Path, "tools": body["tools"], "web_search_options": body["web_search_options"]})
	denied := bytes.Contains(raw, []byte("not allowed to access search tool"))
	requested := searchRequested(body)
	text := "local model proof"
	if denied {
		text = "not allowed to access search tool"
	}
	tool := ""
	if requested && !denied {
		tool = "litellm_web_search"
	}
	var result any
	if r.URL.Path == "/v1/chat/completions" {
		result = chatResponse(text, tool)
	} else {
		result = responsesResponse(text, tool)
	}
	_ = json.NewEncoder(w).Encode(result)
}

func searchRequested(body map[string]any) bool {
	tools, _ := body["tools"].([]any)
	for _, value := range tools {
		tool, _ := value.(map[string]any)
		name, _ := tool["name"].(string)
		if function, ok := tool["function"].(map[string]any); ok {
			name, _ = function["name"].(string)
		}
		if name == "litellm_web_search" {
			return true
		}
	}
	return false
}

func chatResponse(text, tool string) any {
	message := map[string]any{"role": "assistant", responseContent: text}
	finish := "stop"
	if tool != "" {
		message[responseContent] = nil
		message["tool_calls"] = []any{map[string]any{"id": "call_proof", responseType: "function", "function": map[string]any{"name": tool, "arguments": `{"query":"native local source proof"}`}}}
		finish = "tool_calls"
	}
	return map[string]any{"id": "chatcmpl-proof", "object": "chat.completion", "created": time.Now().Unix(), "model": "gpt-4o-mini", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": proofInputTokens, "completion_tokens": proofOutputTokens, "total_tokens": proofTotalTokens}}
}

func responsesResponse(text, tool string) any {
	output := []any{map[string]any{responseType: "message", "id": "msg_proof", "role": "assistant", responseStatus: responseCompleted, responseContent: []any{map[string]any{responseType: "output_text", "text": text, "annotations": []any{}}}}}
	if tool != "" {
		output = []any{map[string]any{responseType: "function_call", "id": "fc_proof", "call_id": "call_proof", "name": tool, "arguments": `{"query":"native local source proof"}`, responseStatus: responseCompleted}}
	}
	return map[string]any{"id": "resp_proof", "object": "response", "created_at": time.Now().Unix(), responseStatus: responseCompleted, "model": "gpt-4o-mini", "output": output, "usage": map[string]any{"input_tokens": proofInputTokens, "output_tokens": proofOutputTokens, "total_tokens": proofTotalTokens}}
}
