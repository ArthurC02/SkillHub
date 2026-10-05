package sandbox

import (
	"errors"
	"testing"
)

const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestEveryInputIsCheckedAgainstTheDigestItsRequestNames(t *testing.T) {
	req := RunRequest{
		SkillVersion: PackageRef{PackageSHA256: helloSHA256},
		TestCase: TestCaseSnapshotRef{DatasetRefs: []DatasetRef{
			{ObjectKey: "datasets/a.csv", ContentHash: helloSHA256},
			{ObjectKey: "datasets/unhashed.csv"},
		}},
	}
	for _, tc := range []struct {
		name     string
		grant    ObjectGrant
		body     string
		mismatch bool
	}{
		{"the skill package as named", ObjectGrant{Purpose: "skill_package", ObjectKey: "packages/x.zip"}, "hello", false},
		{"a skill package with other bytes", ObjectGrant{Purpose: "skill_package", ObjectKey: "packages/x.zip"}, "hellO", true},
		{"a dataset as named", ObjectGrant{Purpose: "dataset", ObjectKey: "datasets/a.csv"}, "hello", false},
		{"a dataset with other bytes", ObjectGrant{Purpose: "dataset", ObjectKey: "datasets/a.csv"}, "", true},
		{"a dataset the request names no digest for", ObjectGrant{Purpose: "dataset", ObjectKey: "datasets/unhashed.csv"}, "anything", false},
		{"a dataset the request does not list", ObjectGrant{Purpose: "dataset", ObjectKey: "datasets/other.csv"}, "anything", false},
		{"an output grant", ObjectGrant{Purpose: "artifact_upload", ObjectKey: "packages/x.zip"}, "anything", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := req.VerifyInput(tc.grant, []byte(tc.body))
			if got := errors.Is(err, ErrInputDigestMismatch); got != tc.mismatch {
				t.Fatalf("VerifyInput = %v, want mismatch %v", err, tc.mismatch)
			}
		})
	}
}

func TestAPackageWithNoDigestIsNotChecked(t *testing.T) {
	if err := (RunRequest{}).VerifyInput(ObjectGrant{Purpose: "skill_package"}, []byte("hello")); err != nil {
		t.Fatalf("VerifyInput with no package_sha256 = %v, want nil: a platform that predates the field sends none", err)
	}
}
