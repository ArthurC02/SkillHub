package creation

import "testing"

func TestCreationActivityClassificationCoversEveryLifecycleState(t *testing.T) {
	wants := map[State]string{
		StateQueued: "in_progress", StateWorking: "in_progress",
		StateWaitingInput: "needs_attention", StateWaitingConfirmation: "needs_attention",
		StateDraftReady: "needs_attention", StateCandidateReady: "needs_attention",
		StateFailed: "needs_attention", StateNeedsReupload: "needs_attention",
		StateSaved: "recent", StateCancelled: "recent",
		State("future"): "needs_attention",
	}
	for state, want := range wants {
		got, label := classifyCreationActivity(state)
		if got != want || label == "" {
			t.Errorf("state %q: classification = %q, label = %q", state, got, label)
		}
	}
}

func TestCreationPendingActionOwnsTheSummary(t *testing.T) {
	for action, want := range map[PendingAction]string{
		PendingBriefConfirmation:        "確認 Skill 需求",
		PendingDiagramDescription:       "補充圖像說明",
		PendingDiagramAnswers:           "回答圖像問題",
		PendingDiagramInterpretation:    "確認圖像理解",
		PendingReferenceChoice:          "確認參考 Skill",
		PendingDuplicateAcknowledgement: "確認重複 Skill",
		PendingFetchPermission:          "確認外部資料存取",
		NothingPending:                  "Skill 創作進度",
	} {
		if got := creationActivitySummary(action); got != want {
			t.Errorf("action %q: summary = %q, want %q", action, got, want)
		}
	}
}
