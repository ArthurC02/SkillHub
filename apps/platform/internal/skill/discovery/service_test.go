package catalog

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func TestAnAbsentModelDoesNotReachSearchLookingPresent(t *testing.T) {
	var unconfigured *llmclient.Client

	if model := ModelOrNone(unconfigured); model != nil {
		t.Error("an unconfigured client arrived as a non-nil Model; every `LLM == nil` guard in search " +
			"now passes and the first embedding call panics")
	}
	if model := ModelOrNone(&llmclient.Client{}); model == nil {
		t.Error("a configured client did not reach the service; search would silently stop embedding")
	}
}
