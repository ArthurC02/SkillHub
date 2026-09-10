package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultKeyBudgetUSD = 0.50
	defaultKeyTPMLimit  = 200_000
)

type Gateway struct {
	AdminBaseURL string

	adminKey string

	SandboxBaseURL string

	Model        string
	MaxBudgetUSD float64
	TPMLimit     int
	HTTP         *http.Client
}

func GatewayFromEnv() *Gateway {
	base := strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/")
	key := os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY")
	if base == "" || key == "" {
		return nil
	}
	admin := strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_ADMIN_URL"), "/")
	if admin == "" {
		admin = base
	}
	g := &Gateway{
		AdminBaseURL:   admin,
		adminKey:       key,
		SandboxBaseURL: base,
		Model:          RunModel(),
		MaxBudgetUSD:   defaultKeyBudgetUSD,
		TPMLimit:       defaultKeyTPMLimit,
		HTTP:           &http.Client{Timeout: 20 * time.Second},
	}
	if v, err := strconv.ParseFloat(os.Getenv("SKILLHUB_RUN_MAX_BUDGET_USD"), 64); err == nil && v > 0 {
		g.MaxBudgetUSD = v
	}
	if v, err := strconv.Atoi(os.Getenv("SKILLHUB_RUN_TPM_LIMIT")); err == nil && v > 0 {
		g.TPMLimit = v
	}
	return g
}

func RunModel() string { return os.Getenv("SKILLHUB_RUN_MODEL") }

func GatewayURL() string {
	if os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY") == "" {
		return ""
	}
	return strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/")
}

func RunBudgetUSD() float64 {
	if raw := os.Getenv("SKILLHUB_RUN_MAX_BUDGET_USD"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			return v
		}
	}
	return defaultKeyBudgetUSD
}

func keyAlias(runAttemptID string) string { return "skillhub-attempt-" + runAttemptID }

func (g *Gateway) Issue(ctx context.Context, runID, runAttemptID string, ttl time.Duration) (*ModelGatewayGrant, error) {
	return g.issue(ctx, runAttemptID, ttl, map[string]string{"run_id": runID, "run_attempt_id": runAttemptID})
}

func (g *Gateway) IssueCreation(ctx context.Context, sessionID, attemptID string, ttl time.Duration) (*ModelGatewayGrant, error) {
	return g.issue(ctx, attemptID, ttl, map[string]string{"creation_session_id": sessionID, "creation_attempt_id": attemptID})
}
func (g *Gateway) issue(ctx context.Context, runAttemptID string, ttl time.Duration, metadata map[string]string) (*ModelGatewayGrant, error) {

	if ttl <= 0 {
		ttl = time.Hour
	}
	body := map[string]any{
		"key_alias":  keyAlias(runAttemptID),
		"duration":   strconv.Itoa(int(ttl.Seconds())) + "s",
		"max_budget": g.MaxBudgetUSD,
		"tpm_limit":  g.TPMLimit,

		"metadata": metadata,
	}
	if g.Model != "" {

		body["models"] = []string{g.Model}
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := g.post(ctx, "/key/generate", body, &out); err != nil {
		return nil, fmt.Errorf("mint virtual key: %w", err)
	}
	if out.Key == "" {
		return nil, errors.New("mint virtual key: gateway returned no key")
	}
	return &ModelGatewayGrant{
		BaseURL:      g.SandboxBaseURL,
		VirtualKey:   out.Key,
		MaxBudgetUSD: g.MaxBudgetUSD,
		TPMLimit:     g.TPMLimit,
		ExpiresAt:    time.Now().UTC().Add(ttl),
	}, nil
}

func (g *Gateway) Revoke(ctx context.Context, runAttemptID string) error {
	err := g.post(ctx, "/key/delete", map[string]any{
		"key_aliases": []string{keyAlias(runAttemptID)},
	}, nil)
	if ge, ok := errors.AsType[*gatewayError](err); ok && ge.notFound() {
		return nil
	}
	return err
}

type AttemptUsage struct {
	InputTokens  int
	OutputTokens int

	SpendUSD float64

	SpendReported bool
}

const (
	usagePageSize = 200

	maxUsagePages = 5

	usageResponseLimit = 8 << 20

	usageDateFormat = "2006-01-02 15:04:05"
)

func (g *Gateway) AttemptUsage(ctx context.Context, runAttemptID string, since time.Time) (AttemptUsage, error) {
	q := url.Values{}
	q.Set("key_alias", keyAlias(runAttemptID))
	q.Set("start_date", since.UTC().Format(usageDateFormat))

	q.Set("end_date", time.Now().UTC().Add(time.Minute).Format(usageDateFormat))
	q.Set("page_size", strconv.Itoa(usagePageSize))

	q.Set("sort_by", "startTime")
	q.Set("sort_order", "asc")

	var total AttemptUsage
	for page := 1; page <= maxUsagePages; page++ {
		q.Set("page", strconv.Itoa(page))
		var out struct {
			Data []struct {
				PromptTokens     int      `json:"prompt_tokens"`
				CompletionTokens int      `json:"completion_tokens"`
				Spend            *float64 `json:"spend"`
			} `json:"data"`
			TotalPages int `json:"total_pages"`
		}
		if err := g.get(ctx, "/spend/logs/v2?"+q.Encode(), &out); err != nil {
			return AttemptUsage{}, fmt.Errorf("read attempt usage: %w", err)
		}
		for _, row := range out.Data {
			total.InputTokens += row.PromptTokens
			total.OutputTokens += row.CompletionTokens
			if row.Spend != nil {
				total.SpendUSD += *row.Spend
				total.SpendReported = true
			}
		}
		if len(out.Data) == 0 || page >= out.TotalPages {
			break
		}
	}
	return total, nil
}

type gatewayError struct {
	Status  int
	Message string
}

func (e *gatewayError) Error() string {
	return fmt.Sprintf("gateway returned %d: %s", e.Status, e.Message)
}

// The gateway answers an unknown key alias with 400 and a message, not
// always 404, so both shapes are checked.
func (e *gatewayError) notFound() bool {
	if e.Status == http.StatusNotFound {
		return true
	}
	lower := strings.ToLower(e.Message)
	return e.Status == http.StatusBadRequest &&
		(strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist"))
}

func (g *Gateway) post(ctx context.Context, path string, body, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	return g.do(ctx, http.MethodPost, path, encoded, out, 1<<20)
}

func (g *Gateway) get(ctx context.Context, path string, out any) error {
	return g.do(ctx, http.MethodGet, path, nil, out, usageResponseLimit)
}

func (g *Gateway) do(ctx context.Context, method, path string, body []byte, out any, limit int64) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.AdminBaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+g.adminKey)

	client := g.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := readBoundedResponse(resp.Body, limit)
	if err != nil {
		return err
	}
	if !successfulGatewayStatus(resp.StatusCode) {
		return &gatewayError{Status: resp.StatusCode, Message: truncate(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func successfulGatewayStatus(code int) bool { return code >= 200 && code < 300 }

func readBoundedResponse(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return raw, nil
}
