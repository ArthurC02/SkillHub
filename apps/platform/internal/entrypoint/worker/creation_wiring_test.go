package worker

import "testing"

func TestTheCreationKeyFollowsTheModelSettingAndDefaultsToTheCreationRole(t *testing.T) {
	t.Setenv("CREATION_MODEL", "")
	if got := creationModel(); got != "skillhub-creation" {
		t.Fatalf("unset CREATION_MODEL: got %q, want the creation role", got)
	}
	t.Setenv("CREATION_MODEL", "gpt-5.4-mini")
	if got := creationModel(); got != "gpt-5.4-mini" {
		t.Fatalf("set CREATION_MODEL: got %q, want it verbatim", got)
	}
}
