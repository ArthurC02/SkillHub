package apiserver

import "testing"

// PORT-003: disclosureFeatures()'s clean_mode key must behave the same way generate_skill
// already does — absent, not false, when the deployment has not declared
// itself. A client (web's useCleanMode, or /me's own doc comment) that has to
// tell "off" apart from "this build predates the flag" is a client that will
// get one of them wrong.
//
// Set on the Config, not on the process environment. features() used to read
// SKILLHUB_CLEAN_MODE itself, which is why this file used to need t.Setenv: the
// one axis that decides whether a user sees the disclosure was the one axis a
// test could not set the way it sets every other deployment input.
func TestFeaturesCleanModeAbsentByDefault(t *testing.T) {
	f := disclosureFeatures(Config{})
	if _, ok := f["clean_mode"]; ok {
		t.Fatal(`disclosureFeatures(Config{}) has a "clean_mode" key with CleanMode unset; want the key absent`)
	}
}

func TestFeaturesCleanModeOnWhenDeclared(t *testing.T) {
	f := disclosureFeatures(Config{CleanMode: true})
	if f["clean_mode"] != true {
		t.Fatalf(`disclosureFeatures(Config{CleanMode: true})["clean_mode"] = %v; want true`, f["clean_mode"])
	}
}

// The environment must no longer reach features(): a deployment that sets the
// variable but does not wire the field is a deployment whose API half of PORT-003
// is silent, and that is exactly the split the field exists to close.
func TestFeaturesCleanModeIgnoresTheProcessEnvironment(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")

	f := disclosureFeatures(Config{})
	if _, ok := f["clean_mode"]; ok {
		t.Fatal("disclosureFeatures(Config{}) read SKILLHUB_CLEAN_MODE from the environment; the flag must reach it through Config.CleanMode only")
	}
}

// The other half of the split (ADR-060 / ADR-052): clean_mode is a disclosure and
// generate_skill is an entry point, and they must not come out of one map. /me
// gates entry points on the BETA-001 invite list, so a shared map meant an
// uninvited visitor on a clean-mode deployment was told nothing about the
// deployment they were standing in.
func TestTheTwoFeatureMapsDoNotOverlap(t *testing.T) {
	cfg := Config{CleanMode: true, GenerateExposed: true}
	entry, disclosure := entryPointFeatures(cfg), disclosureFeatures(cfg)
	if entry["generate_skill"] != true {
		t.Error("generate_skill is not an entry point; /me would stop gating it per caller")
	}
	if _, ok := entry["clean_mode"]; ok {
		t.Error("clean_mode is in the entry-point map, so /me gates a disclosure on the invite list")
	}
	if disclosure["clean_mode"] != true {
		t.Error("clean_mode is not a disclosure; a deployment fact would be gated")
	}
	if _, ok := disclosure["generate_skill"]; ok {
		t.Error("generate_skill is in the disclosure map, so an uninvited caller is shown an entry point they cannot use")
	}
}
