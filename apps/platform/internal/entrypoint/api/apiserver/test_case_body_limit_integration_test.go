package apiserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestATestCasePromptAtTheCharacterLimitInFourByteCharactersIsAccepted(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, freshName("wide-prompt"))
	skillID := importPackage(t, pool, a.packages, owner, freshName("wide-prompt-skill"), false)
	create := func(chars int) int {
		body, _ := json.Marshal(map[string]string{
			"skill_id": skillID, "name": freshName("wide"), "user_prompt": strings.Repeat("𠮷", chars),
		})
		code, _ := postJSON(t, owner, "/test-cases", string(body))
		return code
	}

	if code := create(testlab.MaxPromptChars + 1); code != http.StatusBadRequest {
		t.Errorf("a prompt one character past the limit: got %d, want 400", code)
	}
	if code := create(testlab.MaxPromptChars); code != http.StatusCreated {
		t.Errorf("a prompt at the character limit in four-byte characters: got %d, want 201", code)
	}
}
