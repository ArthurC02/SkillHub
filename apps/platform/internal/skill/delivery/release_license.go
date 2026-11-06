package packaging

import "fmt"

const (
	licenseSourceReleaseDeclaration = "release-declaration"
	releaseDeclarationDisclosure    = "license-from-release-declaration"
)

func withReleaseDeclaration(v VersionFacts, declared string) VersionFacts {
	if v.LicenseExpression != nil || declared == "" {
		return v
	}
	source := licenseSourceReleaseDeclaration
	v.LicenseExpression, v.LicenseSource = &declared, &source
	return v
}

func declaredOnRelease(v VersionFacts) (string, bool) {
	if v.LicenseSource == nil || *v.LicenseSource != licenseSourceReleaseDeclaration {
		return "", false
	}
	return *v.LicenseExpression, true
}

func releaseDeclarationDisclosures(v VersionFacts) []ManifestFinding {
	license, ok := declaredOnRelease(v)
	if !ok {
		return nil
	}
	return []ManifestFinding{{
		Code: releaseDeclarationDisclosure,
		Message: fmt.Sprintf("套件本身沒有授權宣告；發佈者在發佈這個版本時宣告授權是 %s。"+
			"這份宣告不在 SKILL.md 裡，也沒有經過人工確認。", license),
	}}
}

func releaseDeclarationInstallNote(v VersionFacts) string {
	license, ok := declaredOnRelease(v)
	if !ok {
		return ""
	}
	return fmt.Sprintf("\n## License\n\nThis Skill's own files declare no license. The publisher declared %s "+
		"when releasing this version; `skillhub-manifest.json` records it with the source tier "+
		"`release-declaration`. Skill Hub has not checked the declaration.\n", license)
}
