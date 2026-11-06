package apiserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAnAcquiredGeneratedSkillCarriesTheLicenseItsPublisherDeclared(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("declared-alice"))
	bob := a.login(t, freshName("declared-bob"))
	registerPublisher(t, alice, freshName("declared"))
	name := freshName("declared-skill")
	code, reply := postSource(t, alice, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: " + name + "\ndescription: Turn one thing into another.\n---\n# " + name + "\n\nDo it.\n",
	}))
	if code != http.StatusCreated {
		t.Fatalf("upload: %d %v", code, reply)
	}
	skillID, _ := skillsOf(t, reply)[0]["skill_id"].(string)
	setSkill(t, pool, skillID, "redistribution = 'generated'")
	code, published := publish(t, alice, skillID, `{"rights_attested":true,"declared_license":"MIT"}`)
	if code != http.StatusOK {
		t.Fatalf("publishing with MIT declared: %d %v", code, published)
	}

	code, body := acquire(t, bob, published["address"].(string))
	if code != http.StatusCreated {
		t.Fatalf("bob acquiring: %d %v", code, body)
	}
	got, install, skillMD := acquiredLicense(t, a, body["content_hash"].(string))
	if got.Expression == nil || *got.Expression != "MIT" || got.SourceTier == nil || *got.SourceTier != "release-declaration" {
		t.Errorf("manifest license = %+v, want MIT from release-declaration", got)
	}
	if len(got.Disclosures) != 1 || got.Disclosures[0].Code != "license-from-release-declaration" {
		t.Errorf("manifest disclosures = %+v, want exactly license-from-release-declaration", got.Disclosures)
	}
	if !strings.Contains(install, "The publisher declared MIT when releasing this version") {
		t.Errorf("INSTALL.md does not name the declared license:\n%s", install)
	}
	if strings.Contains(skillMD, "license:") {
		t.Errorf("the declaration was written into SKILL.md:\n%s", skillMD)
	}
}

type manifestLicense struct {
	Expression  *string `json:"expression"`
	SourceTier  *string `json:"source_tier"`
	Disclosures []struct {
		Code string `json:"code"`
	} `json:"disclosures"`
}

func acquiredLicense(t *testing.T, a *api, contentHash string) (license manifestLicense, install, skillMD string) {
	t.Helper()
	var manifest struct {
		License manifestLicense `json:"license"`
	}
	for path, content := range zipEntries(t, a, contentHash) {
		switch {
		case strings.HasSuffix(path, "skillhub-manifest.json"):
			if err := json.Unmarshal(content, &manifest); err != nil {
				t.Fatal(err)
			}
		case strings.HasSuffix(path, "INSTALL.md"):
			install = string(content)
		case strings.HasSuffix(path, "SKILL.md"):
			skillMD = string(content)
		}
	}
	return manifest.License, install, skillMD
}
