package apiserver_test

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func (s packageStore) Remove(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

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

	code, _ = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		fmt.Sprintf(`{"user_prompt":%q}`, strings.Repeat("x", testlab.MaxPromptBytes+1)))
	if code != http.StatusRequestEntityTooLarge && code != http.StatusBadRequest {
		t.Errorf("patch with an over-long prompt: got %d, want 400", code)
	}

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

	if _, body := bob.doJSON(t, http.MethodGet, "/test-cases", ""); len(body["test_cases"].([]any)) != 0 {
		t.Errorf("another workspace's drafts leaked into the list: %v", body)
	}

	if _, body := alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", ""); len(body["datasets"].([]any)) != 1 {
		t.Errorf("owner lost the dataset: %v", body)
	}
}

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

	if code, _ := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"   "}`); code != http.StatusBadRequest {
		t.Errorf("blank criterion text: got %d, want 400", code)
	}

	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid, `{"confirmed":true}`)
	list = criteriaOf(t, body)
	if list[0]["confirmed_at"] == nil {
		t.Fatal("confirmation was not recorded")
	}

	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid,
		`{"text":"The summary names every column and its unit."}`)
	list = criteriaOf(t, body)
	if list[0]["confirmed_at"] != nil {
		t.Fatal("editing a confirmed criterion kept the old confirmation")
	}
	if list[0]["text"] != "The summary names every column and its unit." {
		t.Fatalf("edit did not apply: %v", list[0])
	}

	if code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/nope", `{"confirmed":true}`); code != http.StatusNotFound {
		t.Errorf("confirming an unknown criterion: got %d, want 404", code)
	}

	_, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/criteria/"+cid, "")
	if len(criteriaOf(t, body)) != 0 {
		t.Fatalf("criterion was not removed: %v", body)
	}
}

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

	code, _ := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		`{"rubric":{"version":"content-007/writing/v1","items":[{"id":"not-a-criterion","text":"x","evidence_required":true}]}}`)
	if code != http.StatusBadRequest {
		t.Errorf("rubric item with an unknown criterion id: got %d, want 400", code)
	}

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

	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"name":"rubric renamed"}`)
	if body["rubric"] == nil {
		t.Error("an omitted rubric field must keep the stored rubric")
	}

	_, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/criteria/"+cid, "")
	if body["rubric"] != nil {
		t.Errorf("rubric outlived the only criterion it addressed: %v", body["rubric"])
	}
}

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

	expires, err := time.Parse(time.RFC3339, body["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(expires); d < 89*24*time.Hour || d > 91*24*time.Hour {
		t.Errorf("expires_at is %v away, want ~90 days", d)
	}

	_, body = alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", "")
	if list, _ := body["datasets"].([]any); len(list) != 1 {
		t.Fatalf("dataset list: %v", body)
	}
	if total, _ := body["total_bytes"].(float64); int(total) != 2048 {
		t.Errorf("total_bytes = %v, want 2048", body["total_bytes"])
	}

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

func TestDatasetUploadEnforcesPerFileSizeLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-size-limit")
	_, id := newTestCase(t, pool, a, alice, "size")

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

	code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "overflow.csv", csvBytes(1024))
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload past the total budget: got %d, body %v", code, body)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "100 MB") {
		t.Errorf("the refusal does not say what the limit is: %q", msg)
	}
}

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

		if msg, _ := body["error"].(string); msg != "不支援這種檔案類型" {
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

func TestDatasetDeletionRemovesTheObject(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-dataset-delete")
	_, id := newTestCase(t, pool, a, alice, "delete")

	_, body := alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", csvBytes(512))
	datasetID := body["dataset_id"].(string)
	keyCount := len(a.packages)
	seedSighting(t, pool, "dataset", datasetID)

	code, body := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+datasetID, "")
	if code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("DELETE dataset: got %d, body %v", code, body)
	}
	if len(a.packages) != keyCount-1 {
		t.Error("the stored object survived the deletion")
	}
	if n := countRows(t, pool, "SELECT count(*) FROM object_reconcile_sightings WHERE resource_id = $1",
		mustUUID(t, datasetID)); n != 0 {
		t.Error("the deleted dataset left a stale missing-object sighting")
	}
	if _, list := alice.doJSON(t, http.MethodGet, "/test-cases/"+id+"/datasets", ""); len(list["datasets"].([]any)) != 0 {
		t.Errorf("deleted dataset still listed: %v", list)
	}

	if code, _ := alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/datasets/"+datasetID, ""); code != http.StatusNotFound {
		t.Errorf("second delete: got %d, want 404", code)
	}

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

	again := takeSnapshot(t, pool, wsID, tcID)
	if again.ContentHash != snap.ContentHash {
		t.Fatalf("hash is not stable over identical input: %s vs %s", again.ContentHash, snap.ContentHash)
	}

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

	if _, err := pool.Exec(ctx,
		"UPDATE test_case_snapshots SET user_prompt = 'tampered' WHERE id = $1", snap.ID); err == nil {
		t.Fatal("a snapshot row accepted an UPDATE")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM test_case_snapshots WHERE id = $1", snap.ID); err == nil {
		t.Fatal("a snapshot row accepted a DELETE")
	}

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
	if _, err := (&testlab.Service{Pool: pool}).CreateSnapshot(ctx, tx, bobWS, tcID); err == nil {
		t.Fatal("snapshotted another workspace's test case")
	}
}

func TestRunCannotStartFromADeletedTestCase(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-deleted-testcase")

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

func takeSnapshot(t *testing.T, pool *pgxpool.Pool, wsID, tcID pgtype.UUID) testlab.Snapshot {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	snap, err := (&testlab.Service{Pool: pool}).CreateSnapshot(ctx, tx, wsID, tcID)
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

func TestTestCaseListFiltersBySkillAndCarriesItsAggregates(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testcase-filter")

	skillA, first := newTestCase(t, pool, a, alice, "filter-a")

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

		if row["skill_name"] != "filter-a-skill" {
			t.Errorf("skill_name = %v, want the seeded skill's name", row["skill_name"])
		}
	}

	if onA[0]["name"] != "filter-a-second" {
		t.Errorf("filtered list is not newest first: %v", onA)
	}
	if onB := alice.listTestCases(t, skillB); len(onB) != 1 {
		t.Errorf("list for skill B = %d drafts, want 1", len(onB))
	}

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

func TestTestCaseListRefusesAnOutOfSchemaLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testcase-limit")
	skillID, _ := newTestCase(t, pool, a, alice, "limit")

	if code, body := alice.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(
		`{"skill_id":%q,"name":"limit-second","user_prompt":"Another prompt."}`, skillID),
	); code != http.StatusCreated {
		t.Fatalf("second draft: got %d, body %v", code, body)
	}

	for _, raw := range []string{"0", "102", "abc", "", "-1", "1.5"} {
		if code, body := alice.doJSON(t, http.MethodGet, "/test-cases?limit="+raw, ""); code != http.StatusBadRequest {
			t.Errorf("GET /test-cases?limit=%q: got %d, want 400 (body %v)", raw, code, body)
		}
	}

	for _, path := range []string{"?limit=1", "?limit=101", "", "?skill_id=" + skillID + "&limit=101"} {
		if code, body := alice.doJSON(t, http.MethodGet, "/test-cases"+path, ""); code != http.StatusOK {
			t.Errorf("GET /test-cases%s: got %d, want 200 (body %v)", path, code, body)
		}
	}

	if code, body := alice.doJSON(t, http.MethodGet, "/test-cases?limit=1", ""); code == http.StatusOK {
		if rows, _ := body["test_cases"].([]any); len(rows) != 1 {
			t.Errorf("limit=1 returned %d rows", len(rows))
		}
	}
}

func (c *client) testCasePage(t *testing.T, query string) []any {
	t.Helper()
	code, body := c.doJSON(t, http.MethodGet, "/test-cases"+query, "")
	if code != http.StatusOK {
		t.Fatalf("GET /test-cases%s: got %d, body %v", query, code, body)
	}
	rows, _ := body["test_cases"].([]any)
	return rows
}

func TestTestCaseListRefusesAnOutOfSchemaOffset(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-testcase-offset")
	skillID, first := newTestCase(t, pool, a, alice, "offset")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(
		`{"skill_id":%q,"name":"offset-second","user_prompt":"Another prompt."}`, skillID))
	if code != http.StatusCreated {
		t.Fatalf("second draft: got %d, body %v", code, body)
	}

	for _, raw := range []string{"-1", "abc", "", "1.5", "2147483648", "0x10"} {
		if code, body := alice.doJSON(t, http.MethodGet, "/test-cases?offset="+raw, ""); code != http.StatusBadRequest {
			t.Errorf("GET /test-cases?offset=%q: got %d, want 400 (body %v)", raw, code, body)
		}
	}

	for _, query := range []string{"", "?offset=0", "?offset=2147483647", "?limit=101&offset=0"} {
		if code, body := alice.doJSON(t, http.MethodGet, "/test-cases"+query, ""); code != http.StatusOK {
			t.Errorf("GET /test-cases%s: got %d, want 200 (body %v)", query, code, body)
		}
	}

	page := alice.testCasePage(t, "?offset=0")
	if len(page) != 2 {
		t.Fatalf("offset=0 returned %d drafts, want 2", len(page))
	}
	skipped := alice.testCasePage(t, "?offset=1")
	if len(skipped) != 1 {
		t.Fatalf("offset=1 returned %d drafts, want 1", len(skipped))
	}
	if idOf(t, skipped[0]) == idOf(t, page[0]) {
		t.Errorf("offset=1 led with the same draft offset=0 did (%s), so it was ignored", idOf(t, page[0]))
	}
	if idOf(t, skipped[0]) != first {
		t.Errorf("offset=1 returned draft %s, want the older one %s", idOf(t, skipped[0]), first)
	}
	if rows := alice.testCasePage(t, "?offset=2"); len(rows) != 0 {
		t.Errorf("offset past the end returned %d drafts, want 0", len(rows))
	}
}

func idOf(t *testing.T, row any) string {
	t.Helper()
	m, ok := row.(map[string]any)
	if !ok {
		t.Fatalf("list row is not an object: %v", row)
	}
	id, _ := m["test_case_id"].(string)
	return id
}

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

	if rows := bob.listTestCases(t, "not-a-uuid"); len(rows) != 0 {
		t.Errorf("an unparseable skill_id fell back to the unfiltered list: %v", rows)
	}

	if rows := alice.listTestCases(t, aliceSkill); len(rows) != 1 {
		t.Errorf("owner lost her filtered list: %v", rows)
	}
}
