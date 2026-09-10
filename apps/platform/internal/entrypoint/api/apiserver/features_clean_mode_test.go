package apiserver

import "testing"

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

func TestFeaturesCleanModeIgnoresTheProcessEnvironment(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")

	f := disclosureFeatures(Config{})
	if _, ok := f["clean_mode"]; ok {
		t.Fatal("disclosureFeatures(Config{}) read SKILLHUB_CLEAN_MODE from the environment; the flag must reach it through Config.CleanMode only")
	}
}

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
