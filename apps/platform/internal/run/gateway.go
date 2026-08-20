package run

// SBX-008 / SEC-005: the per-Run model credential.
//
// Every model call goes through the LiteLLM gateway and the provider API key
// exists only there (iron rule 8, ADR-017). What a sandbox gets instead is a
// Virtual Key minted for one attempt, carrying a spend brake and a rate brake,
// and revoked when the run is cleaned up.
//
// Go talks to the gateway's *management* API only - key lifecycle - and never
// makes an inference call (ADR-017 boundary rule 1). The admin key stays in this
// process; it is never written to a RunRequest, a log, a trace or the database.
//
// Anti-corruption layer, same rule as provider.go: LiteLLM's request and response
// shapes stop at this file. What leaves it is a ModelGatewayGrant — the domain's
// own word for "this attempt may reach the gateway until then" — and nothing in
// the aggregate or the database is expressed in the gateway's vocabulary.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Two brakes, not one (PDM-003 v5 / PDM-005 5.2a-5). max_budget stops a run that
// burns money; tpm_limit stops one that burns it faster than the spend flush can
// notice. Neither is the token ceiling - that is the worker counting reported
// input_tokens, because prompt caching decoupled spend from token count by 7-8x.
//
// The key's TTL is the caller's: it is the same grantSlack-extended wall clock
// the object grants get, so the gateway expiring a key mid-run - which would
// look to the workload like a model outage - cannot happen before the platform's
// own deadline does.
const (
	defaultKeyBudgetUSD = 0.50
	defaultKeyTPMLimit  = 200_000
)

// Gateway is the LiteLLM management client.
type Gateway struct {
	// AdminBaseURL is where this process reaches the gateway. Not necessarily
	// the address a sandbox uses - the execution plane sits on a different
	// network (SBX-007), which is why SandboxBaseURL is separate.
	AdminBaseURL string
	// AdminKey is the gateway master key. Secret, and the reason this type is
	// never marshalled.
	adminKey string
	// SandboxBaseURL is the gateway address that goes into the grant and into
	// the egress allow list the user confirms.
	SandboxBaseURL string
	// Model is the logical name the gateway resolves; the key is scoped to it,
	// so a workload cannot spend this budget on a bigger tier.
	Model        string
	MaxBudgetUSD float64
	TPMLimit     int
	HTTP         *http.Client
}

// GatewayFromEnv builds the gateway client from deployment configuration, or
// returns nil when none is configured. Nil is a valid deployment: no grant is
// minted, the egress allow list stays empty, and a run gets a sandbox with no
// route out - honest, and safer than a half-configured gateway.
//
//	SKILLHUB_MODEL_GATEWAY_URL        the address a sandbox reaches (allow list)
//	SKILLHUB_MODEL_GATEWAY_ADMIN_URL  the address this process reaches (defaults
//	                                  to the above)
//	SKILLHUB_MODEL_GATEWAY_KEY        master key; without it nothing is minted
//	SKILLHUB_RUN_MODEL                logical model, default tier is mini
//	SKILLHUB_RUN_MAX_BUDGET_USD       per-run spend brake
//	SKILLHUB_RUN_TPM_LIMIT            per-run rate brake
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

// RunModel is the logical model name a run asks the gateway for. Empty means the
// gateway's own default; the deployed default is the mini tier (PDM-003 v5).
func RunModel() string { return os.Getenv("SKILLHUB_RUN_MODEL") }

// GatewayURL is the sandbox-facing gateway address, or empty when no gateway is
// configured. Read by defaultPolicy so the egress allow list, the permission
// summary and the minted grant all name the same destination.
func GatewayURL() string {
	if os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY") == "" {
		return ""
	}
	return strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/")
}

// keyAlias is the gateway-side name of one attempt's key. Derived from the
// permanent attempt id rather than stored, which is what makes revocation
// possible after a crash that lost every in-memory handle - and what keeps the
// key itself out of the database (SEC-005: secrets live in the gateway).
func keyAlias(runAttemptID string) string { return "skillhub-attempt-" + runAttemptID }

// Issue mints one attempt's Virtual Key. The caller must treat a failure as
// fail-closed: a run dispatched without a grant would reach a sandbox that can
// see the gateway network and has no credential for it, which is a run that
// fails later and less clearly.
func (g *Gateway) Issue(ctx context.Context, runID, runAttemptID string, ttl time.Duration) (*ModelGatewayGrant, error) {
	if ttl <= 0 {
		ttl = time.Hour
	}
	body := map[string]any{
		"key_alias":  keyAlias(runAttemptID),
		"duration":   strconv.Itoa(int(ttl.Seconds())) + "s",
		"max_budget": g.MaxBudgetUSD,
		"tpm_limit":  g.TPMLimit,
		// Attribution for the spend the gateway records against this key. The
		// platform run_id is the correlation everywhere (iron rule 10).
		"metadata": map[string]string{"run_id": runID, "run_attempt_id": runAttemptID},
	}
	if g.Model != "" {
		// Scope the key to the tier this run was priced for. A key that can call
		// anything is a budget that means nothing.
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

// Revoke deletes the attempt's key. Idempotent (iron rule 9): a key that is
// already gone, or that was never minted, is the state the caller asked for, so
// the gateway saying "no such key" is success. Addressed by alias, so cleanup
// after a restart needs nothing that was held in memory.
func (g *Gateway) Revoke(ctx context.Context, runAttemptID string) error {
	err := g.post(ctx, "/key/delete", map[string]any{
		"key_aliases": []string{keyAlias(runAttemptID)},
	}, nil)
	var ge *gatewayError
	if errors.As(err, &ge) && ge.notFound() {
		return nil
	}
	return err
}

type gatewayError struct {
	Status  int
	Message string
}

func (e *gatewayError) Error() string {
	return fmt.Sprintf("gateway returned %d: %s", e.Status, e.Message)
}

// notFound covers the shapes LiteLLM uses for "that key is not here". It answers
// 400 rather than 404 for an unknown alias, so the status alone is not enough.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.AdminBaseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
	// Bounded: the gateway is infrastructure, not a trusted narrator.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return &gatewayError{Status: resp.StatusCode, Message: truncate(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
