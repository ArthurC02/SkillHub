// Test Lab integration tests: TEST-001 (prompt), TEST-003 (acceptance criteria),
// TEST-004 (dataset upload, limits, association, deletion) and TEST-010 (the
// immutable snapshot a run executes).
//
// They live in this package for the reason authz_integration_test.go gives: the
// wired HTTP surface is here, so these exercise apiserver.NewRouter's real table
// rather than a copy. They need SKILLHUB_TEST_DATABASE_URL and skip without it.
package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"archive/zip"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
)

// Remove completes testlab.ObjectStore for the in-memory store. Deleting a key
// that is not there succeeds, the same as the real object store, which is what
// makes dataset deletion safe to re-run.
func (s packageStore) Remove(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

// --- helpers ---------------------------------------------------------------

func (c *client) doJSON(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// upload posts one multipart file part, which is the whole TEST-004 request.
func (c *client) upload(t *testing.T, path, fileName string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(c.base+path, mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// newTestCase creates a skill and a draft against it, returning both ids.
func newTestCase(t *testing.T, pool *pgxpool.Pool, a *api, c *client, name string) (skillID, testCaseID string) {
	t.Helper()
	skillID = seedSkill(t, pool, c.workspaceID, name+"-skill")
	code, body := c.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(
		`{"skill_id":%q,"name":%q,"user_prompt":"Summarise the attached rows."}`, skillID, name))
	if code != http.StatusCreated {
		t.Fatalf("POST /test-cases: got %d, body %v", code, body)
	}
	id, _ := body["test_case_id"].(string)
	if id == "" {
		t.Fatalf("created test case has no id: %v", body)
	}
	return skillID, id
}

func criteriaOf(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["acceptance_criteria"].([]any)
	if !ok {
		t.Fatalf("response has no acceptance_criteria: %v", body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("acceptance_criteria element is not an object: %v", item)
		}
		out = append(out, m)
	}
	return out
}

func csvBytes(n int) []byte {
	var b bytes.Buffer
	b.WriteString("id,value\n")
	for b.Len() < n {
		b.WriteString("1,filler-row-of-plain-text\n")
	}
	return b.Bytes()[:n]
}

// --- TEST-001 --------------------------------------------------------------

func TestTestCaseCRUDAndPromptValidation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testlab-crud")
	skillID, id := newTestCase(t, pool, a, alice, "crud")

	code, body := alice.doJSON(t, http.MethodGet, "/test-cases/"+id, "")
	if code != http.StatusOK || body["user_prompt"] != "Summarise the attached rows." {
		t.Fatalf("GET /test-cases/{id}: got %d, body %v", code, body)
	}
	if body["skill_id"] != skillID {
		t.Fatalf("draft is bound to the wrong skill: %v", body["skill_id"])
	}

	// TEST-001: a blank prompt is refused, on create and on edit alike.
	for _, prompt := range []string{"", "   ", "\n\t "} {
		code, _ := alice.doJSON(t, http.MethodPost, "/test-cases",
			fmt.Sprintf(`{"skill_id":%q,"name":"blank","user_prompt":%q}`, skillID, prompt))
		if code != http.StatusBadRequest {
			t.Errorf("create with prompt %q: got %d, want 400", prompt, code)
		}
		code, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
			fmt.Sprintf(`{"user_prompt":%q}`, prompt))
		if code != http.StatusBadRequest {
			t.Errorf("patch with prompt %q: got %d, want 400", prompt, code)
		}
	}
	// And so is one past the length cap.
	code, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		fmt.Sprintf(`{"user_prompt":%q}`, strings.Repeat("x", testlab.MaxPromptBytes+1)))
	if code != http.StatusRequestEntityTooLarge && code != http.StatusBadRequest {
		t.Errorf("patch with an over-long prompt: got %d, want 400", code)
	}

	// A partial edit leaves the untouched field alone.
	code, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"user_prompt":"Extract the totals."}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH /test-cases/{id}: got %d, body %v", code, body)
	}
	if body["name"] != "crud" || body["user_prompt"] != "Extract the totals." {
		t.Fatalf("partial edit changed the wrong fields: %v", body)
	}

	code, body = alice.doJSON(t, http.MethodGet, "/test-cases", "")
	if code != http.StatusOK {
		t.Fatalf("GET /test-cases: got %d", code)
	}
	if list, _ := body["test_cases"].([]any); len(list) != 1 {
		t.Fatalf("list returned %d drafts, want 1", len(list))
	}

	code, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id, "")
	if code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("DELETE /test-cases/{id}: got %d, body %v", code, body)
	}
	if note, _ := body["note"].(string); note == "" {
		t.Error("WS-002 requires the deletion to state its scope; note was empty")
	}
	if code, _ := alice.doJSON(t, http.MethodGet, "/test-cases/"+id, ""); code != http.StatusNotFound {
		t.Errorf("deleted draft still readable: got %d", code)
	}
}

// A draft may only reference a skill in the caller's own workspace: skill_id
// arrives from the client and the foreign key alone would accept anyone's
// (iron rule 3).
func TestTestCaseRejectsForeignSkill(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testlab-foreign")
	bob := a.login(t, "bob-testlab-foreign")
	bobSkill := seedSkill(t, pool, bob.workspaceID, "bob-private")

	code, _ := alice.doJSON(t, http.MethodPost, "/test-cases",
		fmt.Sprintf(`{"skill_id":%q,"name":"stolen","user_prompt":"Do a thing."}`, bobSkill))
	if code != http.StatusNotFound {
		t.Fatalf("draft against another workspace's skill: got %d, want 404", code)
	}
}

// WS-006: every test-lab route answers 404 for a draft in another workspace —
// the same answer as one that does not exist.
func TestTestCaseScopeIsWorkspaceBound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testlab-scope")
	bob := a.login(t, "bob-testlab-scope")
	_, id := newTestCase(t, pool, a, alice, "scoped")

	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", csvBytes(64))
	if code != http.StatusCreated {
		t.Fatalf("seed upload: got %d, body %v", code, body)
	}
	datasetID, _ := body["dataset_id"].(string)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/test-cases/" + id, ""},
		{http.MethodPatch, "/test-cases/" + id, `{"name":"hijacked"}`},
		{http.MethodDelete, "/test-cases/" + id, ""},
		{http.MethodPost, "/test-cases/" + id + "/criteria", `{"text":"anything"}`},
		{http.MethodGet, "/test-cases/" + id + "/datasets", ""},
		{http.MethodDelete, "/test-cases/" + id + "/datasets/" + datasetID, ""},
	} {
		if code, _ := bob.doJSON(t, tc.method, tc.path, tc.body); code != http.StatusNotFound {
			t.Errorf("%s %s as another user: got %d, want 404", tc.method, tc.path, code)
		}
	}
	// Bob's list never contains Alice's draft either.
	if _, body := bob.doJSON(t, http.MethodGet, "/test-cases", ""); len(body["test_cases"].([]any)) != 0 {
		t.Errorf("another workspace's drafts leaked into the list: %v", body)
	}
	// And Alice's file is still there, unaffected by Bob's attempts.
	if _, body := alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", ""); len(body["datasets"].([]any)) != 1 {
		t.Errorf("owner lost the dataset: %v", body)
	}
}

// Anonymous callers get 401 on every test-lab route, never a 404 that would
// mean the route fell out of the table (CORE-006).
func TestTestLabRoutesRejectAnonymousCallers(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	anon := &client{Client: &http.Client{}, base: a.URL}
	const id = "00000000-0000-0000-0000-000000000001"

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/test-cases"},
		{http.MethodPost, "/test-cases"},
		{http.MethodGet, "/test-cases/limits"},
		{http.MethodGet, "/test-cases/" + id},
		{http.MethodPatch, "/test-cases/" + id},
		{http.MethodDelete, "/test-cases/" + id},
		{http.MethodPost, "/test-cases/" + id + "/criteria"},
		{http.MethodPost, "/test-cases/" + id + "/criteria/suggest"},
		{http.MethodPatch, "/test-cases/" + id + "/criteria/abc"},
		{http.MethodDelete, "/test-cases/" + id + "/criteria/abc"},
		{http.MethodPost, "/test-cases/" + id + "/datasets"},
		{http.MethodGet, "/test-cases/" + id + "/datasets"},
		{http.MethodDelete, "/test-cases/" + id + "/datasets/" + id},
	} {
		if got := anon.status(t, tc.method, tc.path); got != http.StatusUnauthorized {
			t.Errorf("%s %s anonymously: got %d, want 401", tc.method, tc.path, got)
		}
	}
}

// --- TEST-003 --------------------------------------------------------------

func TestAcceptanceCriteriaLifecycle(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-criteria")
	_, id := newTestCase(t, pool, a, alice, "criteria")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria",
		`{"text":"The summary names every column."}`)
	if code != http.StatusCreated {
		t.Fatalf("POST criteria: got %d, body %v", code, body)
	}
	list := criteriaOf(t, body)
	if len(list) != 1 || list[0]["confirmed_at"] != nil {
		t.Fatalf("new criterion should start unconfirmed: %v", list)
	}
	if list[0]["source"] != "user" {
		t.Errorf("manually added criterion has source %v, want \"user\"", list[0]["source"])
	}
	cid, _ := list[0]["id"].(string)

	// Blank text is refused, so an empty condition cannot be created.
	if code, _ := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"   "}`); code != http.StatusBadRequest {
		t.Errorf("blank criterion text: got %d, want 400", code)
	}

	// Confirm (TEST-003 確認).
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid, `{"confirmed":true}`)
	list = criteriaOf(t, body)
	if list[0]["confirmed_at"] == nil {
		t.Fatal("confirmation was not recorded")
	}

	// Editing the text withdraws the confirmation: the agreement was to the old
	// wording, and carrying it over would let a criterion nobody agreed to reach
	// a run wearing a confirmation.
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid,
		`{"text":"The summary names every column and its unit."}`)
	list = criteriaOf(t, body)
	if list[0]["confirmed_at"] != nil {
		t.Fatal("editing a confirmed criterion kept the old confirmation")
	}
	if list[0]["text"] != "The summary names every column and its unit." {
		t.Fatalf("edit did not apply: %v", list[0])
	}

	// An unknown criterion is a 404, not a silent no-op.
	if code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/nope", `{"confirmed":true}`); code != http.StatusNotFound {
		t.Errorf("confirming an unknown criterion: got %d, want 404", code)
	}

	_, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/criteria/"+cid, "")
	if len(criteriaOf(t, body)) != 0 {
		t.Fatalf("criterion was not removed: %v", body)
	}
}

// --- CONTENT-007: the editable rubric ---------------------------------------

// The rubric is edited through the draft, and an item's id is the criterion it
// strengthens. That is not a naming convention: /judge-run answers one verdict
// per criterion id and Go drops any id it did not send, so an item pointing
// elsewhere could never produce a stored verdict (writing-rubrics.md §2.1).
func TestRubricIsEditableAndBoundToTheCriteriaItStrengthens(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-rubric")
	_, id := newTestCase(t, pool, a, alice, "rubric")

	_, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria",
		`{"text":"The rewrite keeps every claim of the draft."}`)
	cid := criteriaOf(t, body)[0]["id"].(string)
	if body["rubric"] != nil {
		t.Fatalf("a new test case has no rubric, got %v", body["rubric"])
	}

	// An item naming no criterion of this test case is refused at the boundary
	// rather than accepted and quietly ignored later.
	code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		`{"rubric":{"version":"content-007/writing/v1","items":[{"id":"not-a-criterion","text":"x","evidence_required":true}]}}`)
	if code != http.StatusBadRequest {
		t.Errorf("rubric item with an unknown criterion id: got %d, want 400", code)
	}
	// So is a rubric with no items: clearing one is done with null, which reads
	// differently from "there are items and they all went missing".
	code, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		`{"rubric":{"version":"v1","items":[]}}`)
	if code != http.StatusBadRequest {
		t.Errorf("empty rubric: got %d, want 400", code)
	}
	code, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		fmt.Sprintf(`{"rubric":{"version":"  ","items":[{"id":%q,"text":"x","evidence_required":true}]}}`, cid))
	if code != http.StatusBadRequest {
		t.Errorf("blank rubric version: got %d, want 400", code)
	}

	code, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, fmt.Sprintf(
		`{"rubric":{"version":"content-007/writing/v1","items":[
		   {"id":%q,"text":"Quote the sentence carrying the claim.","weight":3,"evidence_required":true}]}}`, cid))
	if code != http.StatusOK {
		t.Fatalf("PATCH rubric: got %d, body %v", code, body)
	}
	rubric, ok := body["rubric"].(map[string]any)
	if !ok {
		t.Fatalf("rubric was not returned: %v", body)
	}
	if rubric["version"] != "content-007/writing/v1" {
		t.Errorf("rubric version not stored: %v", rubric["version"])
	}
	items, _ := rubric["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("rubric items: %v", rubric["items"])
	}
	item := items[0].(map[string]any)
	if item["id"] != cid || item["evidence_required"] != true || item["weight"] != float64(3) {
		t.Errorf("rubric item round-trip: %v", item)
	}

	// A rubric edit is an edit of the draft alone, so it does not touch the name
	// or the prompt, and an omitted rubric on a later PATCH keeps it.
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"name":"rubric renamed"}`)
	if body["rubric"] == nil {
		t.Error("an omitted rubric field must keep the stored rubric")
	}

	// Deleting the criterion takes its rubric item with it: an item nothing will
	// ever answer is not something the user can see or edit.
	_, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/criteria/"+cid, "")
	if body["rubric"] != nil {
		t.Errorf("rubric outlived the only criterion it addressed: %v", body["rubric"])
	}
}

// --- TEST-004 --------------------------------------------------------------

func TestDatasetUploadStoresAndAssociates(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-dataset")
	_, id := newTestCase(t, pool, a, alice, "dataset")

	before := len(a.packages)
	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", csvBytes(2048))
	if code != http.StatusCreated {
		t.Fatalf("upload: got %d, body %v", code, body)
	}
	if body["content_type"] != "text/plain" {
		t.Errorf("stored content type %v, want the sniffed text type", body["content_type"])
	}
	if len(a.packages) != before+1 {
		t.Error("upload did not put an object in storage")
	}

	// PDM-005 §5.1 / PDM-006 §6: 90 day retention, written at creation so the UI
	// can show it before the run.
	expires, err := time.Parse(time.RFC3339, body["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(expires); d < 89*24*time.Hour || d > 91*24*time.Hour {
		t.Errorf("expires_at is %v away, want ~90 days", d)
	}

	// The file is associated with this test case and shows up under it.
	_, body = alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", "")
	if list, _ := body["datasets"].([]any); len(list) != 1 {
		t.Fatalf("dataset list: %v", body)
	}
	if total, _ := body["total_bytes"].(float64); int(total) != 2048 {
		t.Errorf("total_bytes = %v, want 2048", body["total_bytes"])
	}

	// 02:TEST-002 wants the limits shown before an upload, not after a refusal.
	code, limits := alice.doJSON(t, http.MethodGet, "/test-cases/limits", "")
	if code != http.StatusOK {
		t.Fatalf("GET /test-cases/limits: got %d", code)
	}
	if int64(limits["max_file_bytes"].(float64)) != testlab.MaxFileBytes ||
		int(limits["max_files_per_test_case"].(float64)) != testlab.MaxFilesPerTestCase ||
		int(limits["retention_days"].(float64)) != 90 {
		t.Errorf("published limits disagree with the enforced ones: %v", limits)
	}
}

// Each PDM-005 §5.1 limit, one assertion at a time.
func TestDatasetUploadEnforcesPerFileSizeLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-size-limit")
	_, id := newTestCase(t, pool, a, alice, "size")

	// Exactly at the cap is allowed; one byte past it is not.
	if code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "big.csv", csvBytes(testlab.MaxFileBytes)); code != http.StatusCreated {
		t.Fatalf("a file exactly at the cap was refused: got %d, body %v", code, body)
	}
	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "toobig.csv", csvBytes(testlab.MaxFileBytes+1))
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a file over the cap: got %d, body %v", code, body)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "25 MB") {
		t.Errorf("the refusal does not say what the limit is: %q", msg)
	}
}

func TestDatasetUploadEnforcesFileCountLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-count-limit")
	_, id := newTestCase(t, pool, a, alice, "count")

	for i := range testlab.MaxFilesPerTestCase {
		code, body := alice.upload(t, "/test-cases/"+id+"/datasets", fmt.Sprintf("f%d.csv", i), csvBytes(64))
		if code != http.StatusCreated {
			t.Fatalf("upload %d of %d: got %d, body %v", i+1, testlab.MaxFilesPerTestCase, code, body)
		}
	}
	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "one-too-many.csv", csvBytes(64))
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("file %d: got %d, body %v", testlab.MaxFilesPerTestCase+1, code, body)
	}

	// Deleting one frees a slot: the cap is on live files, not on files ever
	// uploaded.
	_, list := alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", "")
	first := list["datasets"].([]any)[0].(map[string]any)["dataset_id"].(string)
	if code, _ := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+first, ""); code != http.StatusOK {
		t.Fatalf("delete: got %d", code)
	}
	if code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "refill.csv", csvBytes(64)); code != http.StatusCreated {
		t.Fatalf("upload after freeing a slot: got %d, body %v", code, body)
	}
}

func TestDatasetUploadEnforcesTotalSizeLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("moves 100 MB through the upload path")
	}
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-total-limit")
	_, id := newTestCase(t, pool, a, alice, "total")

	chunk := csvBytes(testlab.MaxFileBytes)
	for i := range testlab.MaxTestCaseBytes / testlab.MaxFileBytes {
		if code, body := alice.upload(t, "/test-cases/"+id+"/datasets", fmt.Sprintf("c%d.csv", i), chunk); code != http.StatusCreated {
			t.Fatalf("chunk %d: got %d, body %v", i, code, body)
		}
	}
	// The test case now holds exactly 100 MB in 4 files: under the file count
	// cap, so only the byte budget can refuse the next one.
	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "overflow.csv", csvBytes(1024))
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload past the total budget: got %d, body %v", code, body)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "100 MB") {
		t.Errorf("the refusal does not say what the limit is: %q", msg)
	}
}

// The type is judged by content. A PE binary called rows.csv is refused, and a
// PNG called notes.txt is accepted for what it is.
func TestDatasetUploadJudgesTypeByContentNotExtension(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-magic")
	_, id := newTestCase(t, pool, a, alice, "magic")

	pad := bytes.Repeat([]byte{0}, 256)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"rows.csv", append([]byte{'M', 'Z', 0x90}, pad...)},
		{"notes.txt", append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, pad...)},
		{"data.json", append([]byte{0x1f, 0x8b, 0x08}, pad...)},
		{"report.pdf", append([]byte{0xcf, 0xfa, 0xed, 0xfe}, pad...)},
	} {
		code, body := alice.upload(t, "/test-cases/"+id+"/datasets", tc.name, tc.data)
		if code != http.StatusUnsupportedMediaType {
			t.Errorf("upload of executable content named %q: got %d, body %v", tc.name, code, body)
		}
		// 02:TEST-002: understandable, and silent about the system behind it.
		if msg, _ := body["error"].(string); msg != "file type is not supported" {
			t.Errorf("refusal message for %q leaks detail or is unclear: %q", tc.name, msg)
		}
	}

	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, pad...)
	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "notes.txt", png)
	if code != http.StatusCreated {
		t.Fatalf("PNG content named .txt: got %d, body %v", code, body)
	}
	if body["content_type"] != "image/png" {
		t.Errorf("recorded type %v, want the sniffed image/png rather than the name's", body["content_type"])
	}

	// A zip that escapes its own directory is refused whatever it is called.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../../etc/cron.d/evil")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("* * * * * root sh\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "data.zip", buf.Bytes()); code != http.StatusUnsupportedMediaType {
		t.Errorf("zip with a traversal entry: got %d, body %v", code, body)
	}
}

// TEST-004 deletion: the row goes, the object goes, and the test case's budget
// is freed. Deleting a test case takes its files with it.
func TestDatasetDeletionRemovesTheObject(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-dataset-delete")
	_, id := newTestCase(t, pool, a, alice, "delete")

	_, body := alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", csvBytes(512))
	datasetID := body["dataset_id"].(string)
	keyCount := len(a.packages)

	code, body := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+datasetID, "")
	if code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("DELETE dataset: got %d, body %v", code, body)
	}
	if len(a.packages) != keyCount-1 {
		t.Error("the stored object survived the deletion")
	}
	if _, list := alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", ""); len(list["datasets"].([]any)) != 0 {
		t.Errorf("deleted dataset still listed: %v", list)
	}
	// Idempotent: a second delete is a 404, not a 500 or a double removal.
	if code, _ := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+datasetID, ""); code != http.StatusNotFound {
		t.Errorf("second delete: got %d, want 404", code)
	}

	// Deleting the test case removes the remaining files and says so.
	_, _ = alice.upload(t, "/test-cases/"+id+"/datasets", "a.csv", csvBytes(64))
	_, _ = alice.upload(t, "/test-cases/"+id+"/datasets", "b.csv", csvBytes(64))
	keyCount = len(a.packages)
	code, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id, "")
	if code != http.StatusOK {
		t.Fatalf("DELETE test case: got %d, body %v", code, body)
	}
	if n, _ := body["datasets_deleted"].(float64); int(n) != 2 {
		t.Errorf("datasets_deleted = %v, want 2", body["datasets_deleted"])
	}
	if len(a.packages) != keyCount-2 {
		t.Error("test case deletion left its objects behind")
	}
}

// --- TEST-010 --------------------------------------------------------------

// CreateSnapshot is called by the run domain inside the transaction that creates
// the run. These assertions are about the snapshot itself: what it captures,
// that the hash covers all of it, and that nothing can change it afterwards.
func TestSnapshotFreezesTheTestCase(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-snapshot")
	_, id := newTestCase(t, pool, a, alice, "snapshot")

	_, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"Totals are correct."}`)
	cid := criteriaOf(t, body)[0]["id"].(string)
	_, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid, `{"confirmed":true}`)
	_, body = alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", csvBytes(256))
	datasetID := body["dataset_id"].(string)
	fileHash := body["content_hash"].(string)

	ctx := context.Background()
	var wsID, tcID pgtype.UUID
	if err := wsID.Scan(alice.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := tcID.Scan(id); err != nil {
		t.Fatal(err)
	}

	snap := takeSnapshot(t, pool, wsID, tcID)
	if snap.UserPrompt != "Summarise the attached rows." {
		t.Fatalf("snapshot prompt: %q", snap.UserPrompt)
	}
	criteria, err := testlab.DecodeCriteria(snap.AcceptanceCriteria)
	if err != nil {
		t.Fatal(err)
	}
	if len(criteria) != 1 || criteria[0].ConfirmedAt == nil {
		t.Fatalf("snapshot lost the confirmed criterion: %+v", criteria)
	}
	refs, err := testlab.DecodeDatasetRefs(snap.DatasetRefs)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].DatasetID != datasetID || refs[0].ContentHash != fileHash {
		t.Fatalf("snapshot dataset refs: %+v", refs)
	}
	if snap.ContentHash == "" {
		t.Fatal("snapshot has no content hash")
	}

	// Same input, same hash: two runs that hash alike executed the same thing.
	again := takeSnapshot(t, pool, wsID, tcID)
	if again.ContentHash != snap.ContentHash {
		t.Fatalf("hash is not stable over identical input: %s vs %s", again.ContentHash, snap.ContentHash)
	}

	// Editing the draft changes the next snapshot and leaves the old ones alone
	// (ADR-003, iron rule 4).
	if code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"user_prompt":"Something else."}`); code != http.StatusOK {
		t.Fatal("edit failed")
	}
	edited := takeSnapshot(t, pool, wsID, tcID)
	if edited.ContentHash == snap.ContentHash {
		t.Fatal("editing the prompt did not change the snapshot hash")
	}
	reread := readSnapshot(t, pool, snap.ID, wsID)
	if reread.UserPrompt != snap.UserPrompt {
		t.Fatal("editing the draft rewrote an existing snapshot")
	}

	// 0005's trigger, not application code, is what makes that true.
	if _, err := pool.Exec(ctx,
		"UPDATE test_case_snapshots SET user_prompt = 'tampered' WHERE id = $1", snap.ID); err == nil {
		t.Fatal("a snapshot row accepted an UPDATE")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM test_case_snapshots WHERE id = $1", snap.ID); err == nil {
		t.Fatal("a snapshot row accepted a DELETE")
	}

	// Deleting the file does not take the snapshot's record of it: the run is no
	// longer reproducible but is still traceable (ADR-003 刪除與可追溯性).
	if code, _ := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+datasetID, ""); code != http.StatusOK {
		t.Fatal("dataset delete failed")
	}
	refs, err = testlab.DecodeDatasetRefs(readSnapshot(t, pool, snap.ID, wsID).DatasetRefs)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ContentHash != fileHash || refs[0].FileName != "rows.csv" {
		t.Fatalf("snapshot lost the deleted file's identity: %+v", refs)
	}
}

// The rubric is frozen with the criteria it strengthens (CONTENT-007, iron rule
// 4). Editing it afterwards is a standard for the *next* run; the one that
// already happened keeps the standard it was judged against.
func TestSnapshotFreezesTheRubric(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-snapshot-rubric")
	_, id := newTestCase(t, pool, a, alice, "snapshot-rubric")

	_, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"Every claim is kept."}`)
	cid := criteriaOf(t, body)[0]["id"].(string)

	var wsID, tcID pgtype.UUID
	if err := wsID.Scan(alice.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := tcID.Scan(id); err != nil {
		t.Fatal(err)
	}

	// Before any rubric exists the snapshot hash is what it always was: adding a
	// nullable column must not make every rubric-less test case look different.
	noRubric := takeSnapshot(t, pool, wsID, tcID)
	if noRubric.Rubric != nil {
		t.Fatalf("a test case with no rubric freezes none, got %s", noRubric.Rubric)
	}

	if code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, fmt.Sprintf(
		`{"rubric":{"version":"content-007/writing/v1","items":[
		   {"id":%q,"text":"Quote the sentence carrying the claim.","weight":3,"evidence_required":true}]}}`,
		cid)); code != http.StatusOK {
		t.Fatal("setting the rubric failed")
	}

	snap := takeSnapshot(t, pool, wsID, tcID)
	frozen, err := testlab.DecodeRubric(snap.Rubric)
	if err != nil {
		t.Fatal(err)
	}
	if frozen == nil || frozen.Version != "content-007/writing/v1" || len(frozen.Items) != 1 {
		t.Fatalf("snapshot did not freeze the rubric: %+v", frozen)
	}
	if frozen.Items[0].ID != cid || !frozen.Items[0].EvidenceRequired {
		t.Fatalf("frozen rubric item: %+v", frozen.Items[0])
	}
	if snap.ContentHash == noRubric.ContentHash {
		t.Fatal("two runs judged against different rubrics did not execute the same input")
	}

	// Editing the rubric leaves the frozen copy alone.
	if code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"rubric":null}`); code != http.StatusOK {
		t.Fatal("clearing the rubric failed")
	}
	reread, err := testlab.DecodeRubric(readSnapshot(t, pool, snap.ID, wsID).Rubric)
	if err != nil {
		t.Fatal(err)
	}
	if reread == nil || len(reread.Items) != 1 {
		t.Fatalf("clearing the draft's rubric rewrote a frozen one: %+v", reread)
	}
	if after := takeSnapshot(t, pool, wsID, tcID); after.ContentHash != noRubric.ContentHash {
		t.Error("removing the rubric returns the snapshot to the shape it had without one")
	}
}

// A snapshot cannot be taken for a test case in another workspace, so a run
// cannot be started against one by guessing an id (iron rule 3).
func TestSnapshotIsWorkspaceScoped(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-snapshot-scope")
	bob := a.login(t, "bob-snapshot-scope")
	_, id := newTestCase(t, pool, a, alice, "snapshot-scope")

	var bobWS, tcID pgtype.UUID
	if err := bobWS.Scan(bob.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := tcID.Scan(id); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := testlab.CreateSnapshot(ctx, gen.New(tx), bobWS, tcID); err == nil {
		t.Fatal("snapshotted another workspace's test case")
	}
}

// A deleted draft is not runnable. The guard is testlab's scoped read — the same
// one every other test-lab route uses — rather than a check bolted onto the run
// path, so it holds for any future caller of CreateSnapshot too.
func TestRunCannotStartFromADeletedTestCase(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-deleted-testcase")

	// A run started while the draft was live is unaffected by the deletion: it
	// already holds its own frozen snapshot (ADR-003).
	before := f.start(t)

	if code, _ := f.doJSON(t, http.MethodDelete, "/test-cases/"+f.testCaseID, ""); code != http.StatusOK {
		t.Fatalf("DELETE test case: got %d", code)
	}
	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+`"}`)
	if code != http.StatusNotFound {
		t.Fatalf("run from a deleted test case: got %d, want 404", code)
	}
	if view.Error == "" {
		t.Error("refusal carried no reason")
	}
	if code, _ := f.getRun(t, before.RunID); code != http.StatusOK {
		t.Errorf("deleting the draft broke an existing run: got %d", code)
	}
}

// takeSnapshot calls CreateSnapshot the way the run domain must: inside a
// transaction, which here commits on its own because there is no run row to
// commit with it.
func takeSnapshot(t *testing.T, pool *pgxpool.Pool, wsID, tcID pgtype.UUID) gen.TestCaseSnapshot {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	snap, err := testlab.CreateSnapshot(ctx, gen.New(tx), wsID, tcID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return snap
}

func readSnapshot(t *testing.T, pool *pgxpool.Pool, id, wsID pgtype.UUID) gen.TestCaseSnapshot {
	t.Helper()
	snap, err := gen.New(pool).GetTestCaseSnapshot(context.Background(),
		gen.GetTestCaseSnapshotParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// --- GET /test-cases: the skill filter and the list-row aggregates -----------

// listTestCases reads the list, optionally narrowed to one skill.
func (c *client) listTestCases(t *testing.T, skillID string) []map[string]any {
	t.Helper()
	path := "/test-cases"
	if skillID != "" {
		path += "?skill_id=" + skillID
	}
	code, body := c.doJSON(t, http.MethodGet, path, "")
	if code != http.StatusOK {
		t.Fatalf("GET %s: got %d, body %v", path, code, body)
	}
	raw, ok := body["test_cases"].([]any)
	if !ok {
		t.Fatalf("GET %s has no test_cases: %v", path, body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("test_cases element is not an object: %v", item)
		}
		out = append(out, m)
	}
	return out
}

// "這個 Skill 我寫過哪些 Test Case" had no route before this: the query existed
// (the packager used it) and the parameter did not, so the Skill detail page had
// nothing to call.
func TestTestCaseListFiltersBySkillAndCarriesItsAggregates(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testcase-filter")

	skillA, first := newTestCase(t, pool, a, alice, "filter-a")
	// A second draft on the same skill, and one on a different skill, so the
	// filter has something to include and something to leave out.
	code, body := alice.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(
		`{"skill_id":%q,"name":"filter-a-second","user_prompt":"Another prompt."}`, skillA))
	if code != http.StatusCreated {
		t.Fatalf("second draft on the same skill: got %d, body %v", code, body)
	}
	skillB, _ := newTestCase(t, pool, a, alice, "filter-b")

	if all := alice.listTestCases(t, ""); len(all) != 3 {
		t.Fatalf("unfiltered list = %d drafts, want 3", len(all))
	}
	onA := alice.listTestCases(t, skillA)
	if len(onA) != 2 {
		t.Fatalf("list for skill A = %d drafts, want 2: %v", len(onA), onA)
	}
	for _, row := range onA {
		if row["skill_id"] != skillA {
			t.Errorf("the skill filter returned a draft of another skill: %v", row)
		}
		// 免裸 UUID: the row names its skill so a list is readable as it stands.
		if row["skill_name"] != "filter-a-skill" {
			t.Errorf("skill_name = %v, want the seeded skill's name", row["skill_name"])
		}
	}
	// Newest first, the same order the unfiltered list answers in.
	if onA[0]["name"] != "filter-a-second" {
		t.Errorf("filtered list is not newest first: %v", onA)
	}
	if onB := alice.listTestCases(t, skillB); len(onB) != 1 {
		t.Errorf("list for skill B = %d drafts, want 1", len(onB))
	}

	// The aggregates the browsing user reads before opening a draft.
	row := onA[1]
	if row["criteria_total"] != float64(0) || row["criteria_confirmed"] != float64(0) ||
		row["has_rubric"] != false {
		t.Fatalf("a fresh draft's aggregates are not zeroed: %v", row)
	}
	for _, text := range []string{"first condition", "second condition"} {
		if code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+first+"/criteria",
			fmt.Sprintf(`{"text":%q}`, text)); code != http.StatusCreated {
			t.Fatalf("add criterion: got %d, body %v", code, body)
		}
	}
	_, body = alice.doJSON(t, http.MethodGet, "/test-cases/"+first, "")
	criteria := criteriaOf(t, body)
	cid := criteria[0]["id"].(string)
	if code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+first+"/criteria/"+cid,
		`{"confirmed":true}`); code != http.StatusOK {
		t.Fatalf("confirm criterion: got %d, body %v", code, body)
	}
	if code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+first, fmt.Sprintf(
		`{"rubric":{"version":"v1","items":[{"id":%q,"text":"be specific"}]}}`, cid),
	); code != http.StatusOK {
		t.Fatalf("set rubric: got %d, body %v", code, body)
	}

	for _, row := range alice.listTestCases(t, skillA) {
		if row["test_case_id"] != first {
			continue
		}
		if row["criteria_total"] != float64(2) || row["criteria_confirmed"] != float64(1) {
			t.Errorf("criteria aggregates = %v/%v, want 1/2",
				row["criteria_confirmed"], row["criteria_total"])
		}
		if row["has_rubric"] != true {
			t.Errorf("has_rubric = %v after a rubric was set", row["has_rubric"])
		}
	}
}

// WS-006 / iron rule 3: the filter is a narrowing of the caller's own workspace,
// never a way to read into someone else's. Bob naming Alice's skill gets the same
// empty answer as Bob naming an id that does not exist.
func TestTestCaseSkillFilterDoesNotReachAnotherWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-filter-scope")
	bob := a.login(t, "bob-filter-scope")

	aliceSkill, _ := newTestCase(t, pool, a, alice, "scope-alice")
	if rows := bob.listTestCases(t, aliceSkill); len(rows) != 0 {
		t.Errorf("another workspace's test cases leaked through skill_id: %v", rows)
	}
	if rows := bob.listTestCases(t, "00000000-0000-0000-0000-000000000001"); len(rows) != 0 {
		t.Errorf("an unknown skill_id returned rows: %v", rows)
	}
	// A filter the server cannot parse answers empty, not the whole list: failing
	// open here would show a caller every draft they own when they asked for one
	// skill's.
	if rows := bob.listTestCases(t, "not-a-uuid"); len(rows) != 0 {
		t.Errorf("an unparseable skill_id fell back to the unfiltered list: %v", rows)
	}
	// Alice still sees her own.
	if rows := alice.listTestCases(t, aliceSkill); len(rows) != 1 {
		t.Errorf("owner lost her filtered list: %v", rows)
	}
}
