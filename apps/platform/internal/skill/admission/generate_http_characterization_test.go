package ingest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error body: %q", rec.Body.String())
	}
	return body.Error
}

func TestEachGenerateRefusalHasItsOwnStatusAndMessage(t *testing.T) {
	quota := fmt.Errorf("%w: 本月已用完", policy.ErrGenerateQuotaExceeded)
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantInMsg  string
	}{
		{"blank task", ErrGenerateBlank, http.StatusUnprocessableEntity, "預期產出是什麼。"},
		{"no input at all", ErrGenerateNoInput, http.StatusUnprocessableEntity, "兩者至少要有一項"},
		{"unusable diagram", ErrDiagramInvalid, http.StatusBadRequest, "diagram 不是可用的圖片"},
		{"too many references", ErrTooManyReferences, http.StatusUnprocessableEntity, "最多三個"},
		{"reference unavailable", ErrReferenceUnavailable, http.StatusUnprocessableEntity, "無法使用"},
		{"quota exceeded", quota, http.StatusUnprocessableEntity, quota.Error()},
		{"allowance unknown", policy.ErrAllowanceUnavailable, http.StatusServiceUnavailable, "算不出"},
		{"truncated generation", ErrGenerationTruncated, http.StatusUnprocessableEntity, "拆小一點"},
		{"task too long", ErrGenerateTooLong, http.StatusUnprocessableEntity, "其餘可以省略"},
		{"slot unreadable", ErrGenerateSlotUnavailable, http.StatusServiceUnavailable, "沒有花錢"},
		{"generation in flight", ErrGenerateInFlight, http.StatusConflict, "付兩次錢"},
		{"credit below threshold", ErrCreditThreshold, http.StatusUnprocessableEntity, "點數不足"},
		{"catalogue workspace", ErrGenerateNotForCatalogue, http.StatusUnprocessableEntity, "公開目錄不生成"},
		{"name collision", ErrGeneratedNameCollision, http.StatusUnprocessableEntity, "同名的 Skill"},
		{"any other failure", errors.New("gateway reset"), http.StatusBadGateway, "模型服務"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeGenerateOutcome(rec, GenerateResult{}, fmt.Errorf("generate: %w", tc.err))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if msg := errorBody(t, rec); !strings.Contains(msg, tc.wantInMsg) {
				t.Fatalf("message = %q, want it to contain %q", msg, tc.wantInMsg)
			}
		})
	}
}

func TestAGenerationWithoutErrorIsCreatedUnlessItsReportBlocks(t *testing.T) {
	rec := httptest.NewRecorder()
	writeGenerateOutcome(rec, GenerateResult{Result: Result{Report: skillpkg.Report{Blocked: true}}, Attempts: 2}, nil)
	var rejected GenerateRejected
	if err := json.Unmarshal(rec.Body.Bytes(), &rejected); err != nil || rec.Code != http.StatusUnprocessableEntity || rejected.Attempts != 2 {
		t.Fatalf("blocked: status=%d attempts=%d err=%v, want 422 carrying 2 attempts", rec.Code, rejected.Attempts, err)
	}

	rec = httptest.NewRecorder()
	writeGenerateOutcome(rec, GenerateResult{Attempts: 1, Model: "writer", PromptVersion: "v9"}, nil)
	var created GenerateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusCreated ||
		created.Attempts != 1 || created.Model != "writer" || created.PromptVersion != "v9" {
		t.Fatalf("created: status=%d body=%+v err=%v, want 201 with attempts, model and prompt version", rec.Code, created, err)
	}
}

func TestGenerateRequestDecoding(t *testing.T) {
	const refID = "0b7e7c1e-5d4f-4c1a-9a57-3a2b1c0d9e8f"
	for _, tc := range []struct {
		name      string
		body      string
		wantOK    bool
		wantInMsg string
	}{
		{"not JSON", "task", false, "body must be JSON"},
		{"diagram data that is not base64", `{"diagram":{"media_type":"image/png","data":"@@@"}}`, false, "diagram.data 不是合法的 base64"},
		{"diagram over the encoded ceiling", `{"diagram":{"media_type":"image/png","data":"` +
			strings.Repeat("A", base64.StdEncoding.EncodedLen(generateMaxDiagramBytes)+8) + `"}}`, false, "diagram 超過 4 MB 上限"},
		{"reference id that is not a UUID", `{"task_description":"整理表格","reference_skill_ids":["` + refID + `","nope"]}`, false, "reference_skill_ids"},
		{"task with one reference", `{"task_description":"整理表格","reference_skill_ids":["` + refID + `"]}`, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader(tc.body))
			in, ok := decodeGenerateInput(rec, req)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (response %d %q)", ok, tc.wantOK, rec.Code, rec.Body.String())
			}
			if !tc.wantOK {
				if rec.Code != http.StatusBadRequest || !strings.Contains(errorBody(t, rec), tc.wantInMsg) {
					t.Fatalf("status=%d body=%q, want 400 mentioning %q", rec.Code, rec.Body.String(), tc.wantInMsg)
				}
				return
			}
			if in.TaskDescription != "整理表格" || len(in.ReferenceSkillIDs) != 1 || !in.ReferenceSkillIDs[0].Valid {
				t.Fatalf("decoded input = %+v, want the task and one valid reference id", in)
			}
		})
	}
}
