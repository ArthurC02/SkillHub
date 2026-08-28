package apiserver

import "testing"

// PORT-003: features()'s clean_mode key must behave the same way generate_skill
// already does — absent, not false, when the deployment has not declared
// itself. A client (web's useCleanMode, or /me's own doc comment) that has to
// tell "off" apart from "this build predates the flag" is a client that will
// get one of them wrong.
func TestFeaturesCleanModeAbsentByDefault(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	f := features(Config{})
	if _, ok := f["clean_mode"]; ok {
		t.Fatal(`features(Config{}) has a "clean_mode" key with SKILLHUB_CLEAN_MODE unset; want the key absent`)
	}
}

func TestFeaturesCleanModeOnWhenDeclared(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")

	f := features(Config{})
	if f["clean_mode"] != true {
		t.Fatalf(`features(Config{})["clean_mode"] = %v with SKILLHUB_CLEAN_MODE=1; want true`, f["clean_mode"])
	}
}
