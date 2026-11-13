package skillpkg

import (
	"io/fs"
	"testing"
)

type inaccessibleLicenseEvidenceFS struct {
	fs.FS
	denied string
}

func (f inaccessibleLicenseEvidenceFS) Stat(name string) (fs.FileInfo, error) {
	if name == f.denied {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrPermission}
	}
	return fs.Stat(f.FS, name)
}

func TestCarriedLicenseNeedsBothRegularEvidenceFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove string
		mode   fs.FileMode
		denied string
	}{
		{name: "provenance missing", remove: CarriedProvenanceFile},
		{name: "license missing", remove: CarriedLicenseFile},
		{name: "provenance directory", remove: CarriedProvenanceFile, mode: fs.ModeDir},
		{name: "license directory", remove: CarriedLicenseFile, mode: fs.ModeDir},
		{name: "provenance symlink", remove: CarriedProvenanceFile, mode: fs.ModeSymlink},
		{name: "license symlink", remove: CarriedLicenseFile, mode: fs.ModeSymlink},
		{name: "provenance inaccessible", denied: CarriedProvenanceFile},
		{name: "license inaccessible", denied: CarriedLicenseFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := pkg("---\nname: x\ndescription: d\n---\n", map[string]string{
				CarriedLicenseFile: mitText, CarriedProvenanceFile: repoProvenanceText,
			})
			if tc.mode != 0 {
				files[tc.remove].Mode = tc.mode
			} else if tc.remove != "" {
				delete(files, tc.remove)
			}
			var archive fs.FS = files
			if tc.denied != "" {
				archive = inaccessibleLicenseEvidenceFS{FS: files, denied: tc.denied}
			}
			report := Validate(archive)
			if report.LicenseExpression != "" || report.LicenseSource != "" {
				t.Fatalf("incomplete evidence became %q/%q", report.LicenseExpression, report.LicenseSource)
			}
			if codes(report)["license-unknown"] != SeverityWarning {
				t.Fatalf("incomplete evidence must remain unknown: %+v", report.Findings)
			}
			if _, trusted := codes(report)["license-from-repo-file"]; trusted {
				t.Fatalf("incomplete evidence was disclosed as a repo license: %+v", report.Findings)
			}
		})
	}
}

func TestCarriedLicenseDoesNotReplaceStrongerDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		md     string
		files  map[string]string
		source LicenseSource
	}{
		{"manifest", goodMD, map[string]string{CarriedLicenseFile: apacheText}, LicenseSourceManifest},
		{"package file", "---\nname: x\ndescription: d\n---\n", map[string]string{"LICENSE": mitText, CarriedLicenseFile: apacheText}, LicenseSourcePackageFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := Validate(pkg(tc.md, tc.files))
			if report.LicenseExpression != "MIT" || report.LicenseSource != tc.source {
				t.Fatalf("got %q/%q, want MIT/%s", report.LicenseExpression, report.LicenseSource, tc.source)
			}
		})
	}
}

func TestAReferenceToCarriedLicenseAlsoNeedsProvenance(t *testing.T) {
	const pointer = "Complete terms in LICENSE.repo"
	for _, tc := range []struct {
		name       string
		provenance bool
		want       string
		source     LicenseSource
	}{
		{"complete pair", true, "MIT", LicenseSourceManifestRef},
		{"orphan license", false, pointer, LicenseSourceManifest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{CarriedLicenseFile: mitText}
			if tc.provenance {
				files[CarriedProvenanceFile] = repoProvenanceText
			}
			report := Validate(pkg("---\nname: x\ndescription: d\nlicense: "+pointer+"\n---\n", files))
			if report.LicenseExpression != tc.want || report.LicenseSource != tc.source {
				t.Fatalf("got %q/%q, want %q/%q", report.LicenseExpression, report.LicenseSource, tc.want, tc.source)
			}
		})
	}
}
