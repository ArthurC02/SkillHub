package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func TestASaveIntoADeletedCandidateSaysItIsGoneNotThatTheNameIsTaken(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"deleted candidate", fmt.Errorf("materialize: %w", ingest.ErrGeneratedSkillGone),
			"這次創作先前建立的私人候選版本已被刪除，草稿不能再存成它的新版本；請開始新的創作。"},
		{"name collision", ingest.ErrGeneratedNameCollision,
			"你的工作區已經有一個同名的小工具。請先刪除或改名它，或在對話中請 Agent 把草稿改名後再保存。"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			(&creationHandler{Svc: &creation.Service{}}).creationError(w, c.err)
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != 422 || body.Error != c.want {
				t.Errorf("got %d %q, want 422 %q", w.Code, body.Error, c.want)
			}
		})
	}
}
