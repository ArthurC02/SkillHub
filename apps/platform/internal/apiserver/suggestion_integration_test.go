// Improvement suggestions end to end through the real route table (EVAL-002).
// The LLM service is an httptest server speaking llm-internal.yaml, because what
// is under test is the platform's half: what it agrees to store, what it refuses
// to apply, and the promise that applying builds a new version and leaves the old
// one exactly as it was (iron rule 4).
package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
)

// --- fixtures ----------------------------------------------------------------

// The SKILL.md that namedPackage writes, which is what a suggestion is written
// against and what a proposed replacement has to be a change to.
func packagedSkillMD(name string) string {
	return "---\nname: " + name + "\ndescription: Reports on " + name +
		".\nlicense: MIT\n---\n\nUse it like this.\n"
}

// llmServer answers both evaluation endpoints: a fixed verdict and a fixed set of
// proposals. One server, because one evaluation calls both.
func llmServer(
	t *testing.T, verdict llmclient.JudgeVerdict, proposals []llmclient.ImprovementProposal,
) *llmclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /judge-run", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: verdict, Model: "gpt-5.6-terra", PromptVersion: "judge-run@test",
			Usage: &llmclient.GatewayUsage{PromptTokens: 11, CompletionTokens: 7},
		})
	})
	mux.HandleFunc("POST /suggest-improvements", func(w http.ResponseWriter, r *http.Request) {
		// The request has to carry a digest: a proposal about nothing is not a
		// proposal, and the contract makes the field required with minLength 1.
		var req llmclient.SuggestImprovementsRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.EvaluationDigest == "" {
			http.Error(w, `{"detail":"no digest"}`, http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.SuggestImprovementsResponse{
			Suggestions: proposals, Model: "gpt-5.6-terra",
			PromptVersion: "suggest-improvements/test",
			Usage:         &llmclient.GatewayUsage{PromptTokens: 20, CompletionTokens: 9},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

// The run's final reply, and the quote the judge cites from it. Cited rather than
// asserted, because a criterion verdict only keeps evidence Go could re-resolve —
// and the suggestions built on that verdict inherit those verified references.
const (
	suggestionFinalOutput = "I could not tell which rows were duplicates, so I stopped."
	suggestionQuote       = "could not tell which rows were duplicates"
)

// failedBoth is a verdict that leaves something to improve, which is the state
// suggestions exist for.
var failedBoth = llmclient.JudgeVerdict{
	CriterionResults: []llmclient.CriterionVerdict{
		{CriterionID: "c1", Result: "failed", Reason: "no deduplication happened",
			EvidenceRefs: []llmclient.JudgeEvidenceRef{
				{Kind: "agent_output", Quote: suggestionQuote},
			}},
		{CriterionID: "c2", Result: "failed", Reason: "nothing was produced"},
	},
	Overall: "not_met", Summary: "neither condition was met",
}

// seedRunForVersion writes a finished run against a real, stored skill version.
// Real, because everything in this file reads package bytes: a fabricated
// package_object_key would make every check answer "unreadable" and the tests
// would pass for the wrong reason.
func seedRunForVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, versionID string) string {
	t.Helper()
	ctx := context.Background()
	testCaseID := seedTestCase(t, pool, workspaceID, skillID)

	var snapshotID, runID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'deduplicate the attached spreadsheet',
		        '[{"id":"c1","text":"duplicates are removed","source":"user"},
		          {"id":"c2","text":"an xlsx file is produced","source":"user"}]'::jsonb,
		        'sha256:suggestion-snapshot')
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID),
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, finished_at)
		VALUES ($1, $2, $3, 'fake_sandbox', '{}'::jsonb, '{}'::jsonb, 'succeeded', now())
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID),
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	return runID
}

func latestVersionOf(t *testing.T, pool *pgxpool.Pool, skillID string) (id, hash, key string, number int32) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT id::text, content_hash, package_object_key, version_number
		FROM skill_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1`,
		mustUUID(t, skillID),
	).Scan(&id, &hash, &key, &number); err != nil {
		t.Fatal(err)
	}
	return id, hash, key, number
}

// evaluatedSkill is the shared arrangement: an imported package, a finished run
// against it, and one completed evaluation with whatever suggestions the stubbed
// service proposed.
type evaluatedSkill struct {
	skillID, runID, versionID, versionHash, versionKey string
}

func evaluateWithSuggestions(
	t *testing.T, a *api, pool *pgxpool.Pool, c *client, name string,
	proposals []llmclient.ImprovementProposal,
) evaluatedSkill {
	t.Helper()
	skillID := importPackage(t, pool, a.packages, c, name, false)
	versionID, hash, key, _ := latestVersionOf(t, pool, skillID)
	runID := seedRunForVersion(t, pool, c.workspaceID, skillID, versionID)
	seedFinalOutput(t, pool, c.workspaceID, runID, suggestionFinalOutput)

	llm := llmServer(t, failedBoth, proposals)
	a.evaluations.Judge = llm
	a.evaluations.Suggester = llm
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return evaluatedSkill{skillID: skillID, runID: runID, versionID: versionID,
		versionHash: hash, versionKey: key}
}

// --- HTTP helpers -------------------------------------------------------------

type suggestionBody struct {
	SuggestionID          string `json:"suggestion_id"`
	Category              string `json:"category"`
	Problem               string `json:"problem"`
	TargetPath            string `json:"target_path"`
	ExpectedImpact        string `json:"expected_impact"`
	Decision              string `json:"decision"`
	DecidedAt             string `json:"decided_at"`
	AppliedSkillVersionID string `json:"applied_skill_version_id"`
	Evidence              []struct {
		Kind      string `json:"kind"`
		Excerpt   string `json:"excerpt"`
		Available bool   `json:"available"`
	} `json:"evidence"`
}

type diffBody struct {
	TargetPath    string `json:"target_path"`
	UnifiedDiff   string `json:"unified_diff"`
	Applicable    bool   `json:"applicable"`
	BlockedReason string `json:"blocked_reason"`
	Message       string `json:"message"`
	Error         string `json:"error"`
}

type applyBody struct {
	SkillID              string   `json:"skill_id"`
	VersionID            string   `json:"version_id"`
	VersionNumber        int32    `json:"version_number"`
	ContentHash          string   `json:"content_hash"`
	Duplicate            bool     `json:"duplicate"`
	AppliedSuggestionIDs []string `json:"applied_suggestion_ids"`
	RejectedSuggestions  []struct {
		SuggestionID  string `json:"suggestion_id"`
		BlockedReason string `json:"blocked_reason"`
		Message       string `json:"message"`
	} `json:"rejected_suggestions"`
	Error string `json:"error"`
}

func (c *client) listSuggestions(t *testing.T, runID string) (int, []suggestionBody, string) {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/suggestions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		EvaluationID string           `json:"evaluation_id"`
		Suggestions  []suggestionBody `json:"suggestions"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &body)
	return resp.StatusCode, body.Suggestions, body.EvaluationID
}

func (c *client) decide(t *testing.T, id, decision string) (int, suggestionBody) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		c.base+"/suggestions/"+id+"/decision", strings.NewReader(`{"decision":"`+decision+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out suggestionBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) suggestionDiff(t *testing.T, id string) (int, diffBody) {
	t.Helper()
	resp, err := c.Get(c.base + "/suggestions/" + id + "/diff")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out diffBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) applySuggestions(t *testing.T, skillID, evaluationID string, ids ...string) (int, applyBody) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"evaluation_id": evaluationID, "suggestion_ids": ids})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(c.base+"/skills/"+skillID+"/versions/from-suggestions",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out applyBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// storedFile reads one file out of a stored package, which is how these tests
// check that an old version really did keep its bytes.
func storedFile(t *testing.T, a *api, key, path string) string {
	t.Helper()
	data, err := a.packages.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("stored package %s: %v", key, err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fs.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("%s in %s: %v", path, key, err)
	}
	return string(body)
}

// --- the whole path: proposed, decided, previewed, applied ---------------------

func TestAcceptedSuggestionsBecomeOneNewVersionAndLeaveTheOldOneAlone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "sugg-owner")
	const name = "sugg-dedupe"
	improved := packagedSkillMD(name) + "\nIt deduplicates rows and writes an xlsx file.\n"

	seed := evaluateWithSuggestions(t, a, pool, c, name, []llmclient.ImprovementProposal{
		{
			Category: "skill", Problem: "the description never mentions deduplication",
			Evidence: suggestionQuote, TargetPath: "SKILL.md",
			ProposedContent: improved, ExpectedImpact: "the skill is activated for this task",
		},
		// Refused before storage: a path out of the package, and a class this
		// platform never acts on. Neither reaches a user to decide about.
		{
			Category: "skill", Problem: "read the host's secrets", Evidence: "x",
			TargetPath: "../../etc/passwd", ProposedContent: "root", ExpectedImpact: "none",
		},
		{
			Category: "mcp", Problem: "the MCP server is misconfigured", Evidence: "x",
			TargetPath: "SKILL.md", ProposedContent: improved, ExpectedImpact: "none",
		},
	})

	status, suggestions, evaluationID := c.listSuggestions(t, seed.runID)
	if status != http.StatusOK {
		t.Fatalf("GET suggestions: got %d", status)
	}
	if len(suggestions) != 1 {
		t.Fatalf("only the in-bounds, actionable proposal is stored, got %d", len(suggestions))
	}
	var usageRows, operations int
	var promptTokens int64
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*), count(DISTINCT operation), sum(prompt_tokens)
		FROM evaluation_model_usage WHERE evaluation_id = $1`,
		mustUUID(t, evaluationID)).Scan(&usageRows, &operations, &promptTokens); err != nil {
		t.Fatal(err)
	}
	if usageRows != 2 || operations != 2 || promptTokens != 31 {
		t.Fatalf("model usage ledger = rows:%d operations:%d prompt_tokens:%d, want 2/2/31",
			usageRows, operations, promptTokens)
	}
	s := suggestions[0]
	if s.Category != "skill" || s.TargetPath != "SKILL.md" || s.Decision != "pending" {
		t.Errorf("unexpected suggestion: %+v", s)
	}
	if s.Problem == "" || s.ExpectedImpact == "" || len(s.Evidence) == 0 {
		t.Errorf("EVAL-002 clause 1 requires problem, evidence, location, change and impact: %+v", s)
	}
	if s.DecidedAt != "" {
		t.Errorf("a pending suggestion has no decision timestamp, got %q", s.DecidedAt)
	}

	// The preview is available before the decision, and shows the change against
	// what is really in the package (clause 3).
	code, diff := c.suggestionDiff(t, s.SuggestionID)
	if code != http.StatusOK || !diff.Applicable {
		t.Fatalf("diff: got %d %+v", code, diff)
	}
	if !strings.Contains(diff.UnifiedDiff, "+It deduplicates rows") {
		t.Errorf("the diff does not show the proposed line: %q", diff.UnifiedDiff)
	}

	// Nothing is applied from a suggestion nobody accepted.
	if code, body := c.applySuggestions(t, seed.skillID, evaluationID, s.SuggestionID); code != http.StatusBadRequest {
		t.Fatalf("applying a pending suggestion: got %d (%s)", code, body.Error)
	}
	if code, _ := c.decide(t, s.SuggestionID, "pending"); code != http.StatusBadRequest {
		t.Errorf("`pending` is not settable: got %d", code)
	}

	code, decided := c.decide(t, s.SuggestionID, "accepted")
	if code != http.StatusOK || decided.Decision != "accepted" || decided.DecidedAt == "" {
		t.Fatalf("accepting: got %d %+v", code, decided)
	}
	if decided.AppliedSkillVersionID != "" {
		t.Error("accepting records agreement only; nothing is applied until the version call")
	}

	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, s.SuggestionID, s.SuggestionID)
	if code != http.StatusCreated {
		t.Fatalf("apply: got %d (%s)", code, applied.Error)
	}
	if applied.VersionNumber != 2 || applied.Duplicate {
		t.Errorf("applying creates the next version, got %+v", applied)
	}
	if len(applied.AppliedSuggestionIDs) != 1 || applied.AppliedSuggestionIDs[0] != s.SuggestionID {
		t.Errorf("applied ids: %+v", applied.AppliedSuggestionIDs)
	}
	if len(applied.RejectedSuggestions) != 0 {
		t.Errorf("nothing should have been rejected: %+v", applied.RejectedSuggestions)
	}

	// Iron rule 4: the version the suggestion was written against is untouched, in
	// its row and in its bytes, and the new one carries the change.
	if before := storedFile(t, a, seed.versionKey, "SKILL.md"); before != packagedSkillMD(name) {
		t.Errorf("the evaluated version's package changed: %q", before)
	}
	var oldHash, oldKey string
	if err := pool.QueryRow(context.Background(),
		`SELECT content_hash, package_object_key FROM skill_versions WHERE id = $1`,
		mustUUID(t, seed.versionID)).Scan(&oldHash, &oldKey); err != nil {
		t.Fatal(err)
	}
	if oldHash != seed.versionHash || oldKey != seed.versionKey {
		t.Error("the evaluated version's row was rewritten")
	}
	_, _, newKey, _ := latestVersionOf(t, pool, seed.skillID)
	if after := storedFile(t, a, newKey, "SKILL.md"); after != improved {
		t.Errorf("the new version does not carry the change: %q", after)
	}

	// Clause 4: the suggestion now points at the new version.
	_, suggestions, _ = c.listSuggestions(t, seed.runID)
	if suggestions[0].AppliedSkillVersionID != applied.VersionID {
		t.Errorf("applied_skill_version_id = %q, want the new version %q",
			suggestions[0].AppliedSkillVersionID, applied.VersionID)
	}

	// A version that was built is history: the acceptance behind it cannot be
	// withdrawn, and the way back is another version.
	if code, _ := c.decide(t, s.SuggestionID, "rejected"); code != http.StatusConflict {
		t.Errorf("rejecting an applied suggestion: got %d, want 409", code)
	}

	// And the preview now says what a second apply would: the file it was written
	// against is not what is in the package any more.
	code, diff = c.suggestionDiff(t, s.SuggestionID)
	if code != http.StatusOK || diff.Applicable || diff.BlockedReason != "target_changed" {
		t.Errorf("after applying, the same suggestion is target_changed: got %d %+v", code, diff)
	}
}

// --- two changes to one file, and the honest half-answer -----------------------

func TestTwoSuggestionsOnTheSameFileApplyOneAndSayWhyTheOtherDidNot(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "sugg-clash")
	const name = "sugg-clash-skill"
	base := packagedSkillMD(name)

	seed := evaluateWithSuggestions(t, a, pool, c, name, []llmclient.ImprovementProposal{
		{Category: "skill", Problem: "no mention of deduplication", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: base + "\nIt deduplicates rows.\n",
			ExpectedImpact: "better activation"},
		{Category: "skill", Problem: "no mention of the output format", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: base + "\nIt writes an xlsx file.\n",
			ExpectedImpact: "better activation"},
	})
	_, suggestions, evaluationID := c.listSuggestions(t, seed.runID)
	if len(suggestions) != 2 {
		t.Fatalf("both proposals are storable, got %d", len(suggestions))
	}
	for _, s := range suggestions {
		if code, _ := c.decide(t, s.SuggestionID, "accepted"); code != http.StatusOK {
			t.Fatalf("accept %s: got %d", s.SuggestionID, code)
		}
	}

	var firstOrder [2]string
	for attempt, ids := range [][2]string{
		{suggestions[0].SuggestionID, suggestions[1].SuggestionID},
		{suggestions[1].SuggestionID, suggestions[0].SuggestionID},
	} {
		code, applied := c.applySuggestions(t, seed.skillID, evaluationID, ids[0], ids[1])
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("apply competing replacements: got %d (%s)", code, applied.Error)
		}
		if len(applied.AppliedSuggestionIDs) != 0 || len(applied.RejectedSuggestions) != 2 {
			t.Fatalf("all competing whole-file replacements must be rejected, got applied=%v rejected=%+v",
				applied.AppliedSuggestionIDs, applied.RejectedSuggestions)
		}
		var order [2]string
		for i, rejected := range applied.RejectedSuggestions {
			if rejected.BlockedReason != "target_changed" {
				t.Errorf("competing replacement reason = %q, want target_changed", rejected.BlockedReason)
			}
			order[i] = rejected.SuggestionID
		}
		if attempt == 0 {
			firstOrder = order
		} else if order != firstOrder {
			t.Errorf("reversing suggestion_ids changed response order: %v then %v", firstOrder, order)
		}
	}
	// One version for the whole request, never one per suggestion.
	var versions int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM skill_versions WHERE skill_id = $1`,
		mustUUID(t, seed.skillID)).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 {
		t.Errorf("competing replacements produced a new version; got %d versions", versions)
	}
}

// --- a change that would break the package -------------------------------------

func TestASuggestionThatFailsValidationIsRefusedAndNoVersionIsCreated(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "sugg-invalid")

	seed := evaluateWithSuggestions(t, a, pool, c, "sugg-invalid-skill",
		[]llmclient.ImprovementProposal{{
			Category: "skill", Problem: "the document is too long", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: "just prose, no frontmatter at all\n",
			ExpectedImpact: "shorter",
		}})
	_, suggestions, evaluationID := c.listSuggestions(t, seed.runID)
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions", len(suggestions))
	}
	id := suggestions[0].SuggestionID

	// The preview refuses it for the same reason the apply call will: one
	// vocabulary, served from one place.
	code, diff := c.suggestionDiff(t, id)
	if code != http.StatusOK || diff.Applicable || diff.BlockedReason != "validation_blocked" {
		t.Fatalf("diff: got %d %+v", code, diff)
	}

	if code, _ := c.decide(t, id, "accepted"); code != http.StatusOK {
		t.Fatal("accept failed")
	}
	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, id)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("apply: got %d, want 422", code)
	}
	if len(applied.RejectedSuggestions) != 1 ||
		applied.RejectedSuggestions[0].BlockedReason != "validation_blocked" {
		t.Errorf("the refusal must name the reason per suggestion: %+v", applied.RejectedSuggestions)
	}
	if applied.VersionID != "" {
		t.Error("a refused set must not create a version")
	}
	var versions int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM skill_versions WHERE skill_id = $1`,
		mustUUID(t, seed.skillID)).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 {
		t.Errorf("no version was created, got %d", versions)
	}
}

// --- the licensing hold (0023, SEC-011) ----------------------------------------

func TestAHeldSkillNeitherShowsADiffNorBuildsAVersion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "sugg-restricted")
	const name = "sugg-held-skill"

	seed := evaluateWithSuggestions(t, a, pool, c, name, []llmclient.ImprovementProposal{{
		Category: "skill", Problem: "the description is thin", Evidence: suggestionQuote,
		TargetPath: "SKILL.md", ProposedContent: packagedSkillMD(name) + "\nMore detail.\n",
		ExpectedImpact: "clearer",
	}})
	_, suggestions, evaluationID := c.listSuggestions(t, seed.runID)
	id := suggestions[0].SuggestionID
	if code, _ := c.decide(t, id, "accepted"); code != http.StatusOK {
		t.Fatal("accept failed")
	}

	// The hold arrives after the suggestion was written, which is exactly the case
	// a check at proposal time would miss.
	if _, err := pool.Exec(context.Background(),
		`UPDATE skills SET access_restriction = 'license_review' WHERE id = $1`,
		mustUUID(t, seed.skillID)); err != nil {
		t.Fatal(err)
	}

	// A diff is a reproduction of held material, so it is not served either.
	code, diff := c.suggestionDiff(t, id)
	if code != http.StatusOK || diff.Applicable || diff.BlockedReason != "access_restricted" {
		t.Fatalf("diff of a held skill: got %d %+v", code, diff)
	}
	if diff.UnifiedDiff != "" {
		t.Error("a held skill's contents must not be reproduced in the preview")
	}

	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, id)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("apply on a held skill: got %d, want 422 (the same answer a run gets)", code)
	}
	if len(applied.RejectedSuggestions) != 1 ||
		applied.RejectedSuggestions[0].BlockedReason != "access_restricted" {
		t.Errorf("rejection: %+v", applied.RejectedSuggestions)
	}
}

// --- scope and absence (iron rule 3, WS-006) -----------------------------------

func TestSuggestionsAreInvisibleAcrossWorkspaces(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "sugg-scope-owner")
	stranger := a.login(t, "sugg-scope-stranger")
	const name = "sugg-scoped-skill"

	// A run nobody has evaluated has no suggestions to list, and that is 404 rather
	// than an empty list: an empty list reads as "nothing to improve".
	unevaluated := importPackage(t, pool, a.packages, owner, "sugg-unevaluated", false)
	unevaluatedVersion, _, _, _ := latestVersionOf(t, pool, unevaluated)
	unevaluatedRun := seedRunForVersion(t, pool, owner.workspaceID, unevaluated, unevaluatedVersion)
	if code, _, _ := owner.listSuggestions(t, unevaluatedRun); code != http.StatusNotFound {
		t.Errorf("suggestions of an unevaluated run: got %d, want 404", code)
	}

	seed := evaluateWithSuggestions(t, a, pool, owner, name, []llmclient.ImprovementProposal{{
		Category: "skill", Problem: "the description is thin", Evidence: suggestionQuote,
		TargetPath: "SKILL.md", ProposedContent: packagedSkillMD(name) + "\nMore detail.\n",
		ExpectedImpact: "clearer",
	}})
	code, suggestions, evaluationID := owner.listSuggestions(t, seed.runID)
	if code != http.StatusOK || len(suggestions) != 1 {
		t.Fatalf("owner read: %d %+v", code, suggestions)
	}
	id := suggestions[0].SuggestionID

	if code, _, _ := stranger.listSuggestions(t, seed.runID); code != http.StatusNotFound {
		t.Errorf("stranger listing another workspace's suggestions: got %d, want 404", code)
	}
	if code, _ := stranger.suggestionDiff(t, id); code != http.StatusNotFound {
		t.Errorf("stranger reading a diff: got %d, want 404", code)
	}
	if code, _ := stranger.decide(t, id, "accepted"); code != http.StatusNotFound {
		t.Errorf("stranger deciding: got %d, want 404", code)
	}
	if code, _ := stranger.applySuggestions(t, seed.skillID, evaluationID, id); code != http.StatusNotFound {
		t.Errorf("stranger applying: got %d, want 404", code)
	}
	// Nothing the stranger did changed anything.
	_, after, _ := owner.listSuggestions(t, seed.runID)
	if after[0].Decision != "pending" {
		t.Errorf("a stranger's request changed the decision to %q", after[0].Decision)
	}
}
