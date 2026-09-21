package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const (
	defaultKeyBudgetUSD = 0.50
	defaultKeyTPMLimit  = 200_000
)

type ModelGateway interface {
	Issue(ctx context.Context, runID, runAttemptID string, ttl time.Duration, maxBudgetUSD float64) (*ModelGatewayGrant, error)
	Revoke(ctx context.Context, runAttemptID string) error
	Usage(ctx context.Context, runAttemptID string, since time.Time) (AttemptUsage, error)
	BudgetCeilingUSD() float64
}

// An absent gateway must reach the run as a nil interface: a nil *Gateway
// inside a non-nil interface passes every `!= nil` guard and panics on use.
func GatewayOrNone(g *Gateway) ModelGateway {
	if g == nil {
		return nil
	}
	return g
}

const modelGatewayPurpose = "model_gateway"

var ErrNoModelGateway = errors.New("this deployment has no model gateway, so a run has no way to reach a model")

func (s *Service) requireModelGateway() error {
	if s.Gateway == nil {
		return ErrNoModelGateway
	}
	return nil
}

type Gateway struct {
	adminBaseURL   string
	adminKey       string
	sandboxBaseURL string
	model          string
	maxBudgetUSD   float64
	tpmLimit       int
	client         *http.Client
}

type GatewayConfig struct {
	AdminBaseURL, AdminKey, SandboxBaseURL, Model string
	MaxBudgetUSD                                  float64
	TPMLimit                                      int
	HTTP                                          *http.Client
}

type Deployment struct {
	Model, GatewayURL string
	BudgetUSD         float64
	MinimumIsolation  IsolationStrength
	CleanMode         bool
	CleanModeReleases string
}

func (d Deployment) Budget() float64 {
	if d.BudgetUSD > 0 {
		return d.BudgetUSD
	}
	return defaultKeyBudgetUSD
}

func (d Deployment) RequiredIsolation() IsolationStrength {
	if d.MinimumIsolation != "" {
		return d.MinimumIsolation
	}
	return strongIsolation
}

func NewGateway(c GatewayConfig) *Gateway {
	if c.SandboxBaseURL == "" || c.AdminKey == "" {
		return nil
	}
	if c.AdminBaseURL == "" {
		c.AdminBaseURL = c.SandboxBaseURL
	}
	if c.MaxBudgetUSD <= 0 {
		c.MaxBudgetUSD = defaultKeyBudgetUSD
	}
	if c.TPMLimit <= 0 {
		c.TPMLimit = defaultKeyTPMLimit
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	return &Gateway{adminBaseURL: c.AdminBaseURL, adminKey: c.AdminKey, sandboxBaseURL: c.SandboxBaseURL, model: c.Model, maxBudgetUSD: c.MaxBudgetUSD, tpmLimit: c.TPMLimit, client: c.HTTP}
}

func keyAlias(runAttemptID string) string { return "skillhub-attempt-" + runAttemptID }

func (g *Gateway) Issue(
	ctx context.Context, runID, runAttemptID string, ttl time.Duration, maxBudgetUSD float64,
) (*ModelGatewayGrant, error) {
	return g.issue(ctx, runAttemptID, ttl, maxBudgetUSD, g.model,
		map[string]string{"run_id": runID, "run_attempt_id": runAttemptID})
}

func (g *Gateway) IssueCreation(ctx context.Context, sessionID, attemptID string, ttl time.Duration) (*ModelGatewayGrant, error) {
	return g.issue(ctx, attemptID, ttl, 0, g.model, map[string]string{"creation_session_id": sessionID, "creation_attempt_id": attemptID})
}

func (g *Gateway) IssueCreationForModel(ctx context.Context, sessionID, attemptID string, ttl time.Duration, budget float64, model string) (*ModelGatewayGrant, error) {
	return g.issue(ctx, attemptID, ttl, budget, model, map[string]string{"creation_session_id": sessionID, "creation_attempt_id": attemptID})
}

type keyGenerationRequest struct {
	KeyAlias  string            `json:"key_alias"`
	Duration  string            `json:"duration"`
	MaxBudget float64           `json:"max_budget"`
	TPMLimit  int               `json:"tpm_limit"`
	Metadata  map[string]string `json:"metadata"`
	Models    []string          `json:"models,omitempty"`
}

type keyDeletionRequest struct {
	KeyAliases []string `json:"key_aliases"`
}

func (g *Gateway) issue(
	ctx context.Context, runAttemptID string, ttl time.Duration, maxBudgetUSD float64, model string, metadata map[string]string,
) (*ModelGatewayGrant, error) {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if maxBudgetUSD <= 0 {
		maxBudgetUSD = g.maxBudgetUSD
	}
	body := keyGenerationRequest{
		KeyAlias:  keyAlias(runAttemptID),
		Duration:  strconv.Itoa(int(ttl.Seconds())) + "s",
		MaxBudget: maxBudgetUSD,
		TPMLimit:  g.tpmLimit,
		Metadata:  metadata,
	}
	if model != "" {
		body.Models = []string{model}
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
		BaseURL:      g.sandboxBaseURL,
		VirtualKey:   out.Key,
		MaxBudgetUSD: maxBudgetUSD,
		TPMLimit:     g.tpmLimit,
		ExpiresAt:    time.Now().UTC().Add(ttl),
	}, nil
}

func (g *Gateway) Revoke(ctx context.Context, runAttemptID string) error {
	err := g.post(ctx, "/key/delete", keyDeletionRequest{KeyAliases: []string{keyAlias(runAttemptID)}}, nil)
	if ge, ok := errors.AsType[*gatewayError](err); ok && ge.notFound() {
		return nil
	}
	return err
}

func (g *Gateway) BudgetCeilingUSD() float64 { return g.maxBudgetUSD }

type AttemptUsage struct {
	InputTokens  int
	OutputTokens int

	ModelCostUSD float64

	CostReported bool
}

const (
	usagePageSize = 200

	maxUsagePages = 5

	usageResponseLimit = 8 << 20

	usageDateFormat = "2006-01-02 15:04:05"
)

func (g *Gateway) Usage(ctx context.Context, runAttemptID string, since time.Time) (AttemptUsage, error) {
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
				total.ModelCostUSD += *row.Spend
				total.CostReported = true
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
	status, raw, err := (httpx.Transport{
		Client:        g.client,
		Token:         g.adminKey,
		ResponseLimit: limit,
	}).Do(ctx, method, g.adminBaseURL+path, body)
	if err != nil {
		return err
	}
	if !successfulGatewayStatus(status) {
		return &gatewayError{Status: status, Message: truncate(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func successfulGatewayStatus(code int) bool { return code >= 200 && code < 300 }
