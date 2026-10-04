package skillpkg

import "testing"

const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestAPackageIsStoredUnderItsOwnDigest(t *testing.T) {
	key, digest := PackageObjectKey([]byte("hello"))
	if key != "packages/"+helloSHA256+".zip" || digest != helloSHA256 {
		t.Fatalf("PackageObjectKey(hello) = %q, %q", key, digest)
	}
	if got := PackageDigest(key); got != helloSHA256 {
		t.Fatalf("PackageDigest(%q) = %q, want %q: the sandbox checks the bytes it fetched against this", key, got, helloSHA256)
	}
}

func TestAKeyThatDoesNotNameADigestYieldsNone(t *testing.T) {
	for _, key := range []string{
		"",
		"packages/whole-plugin.zip",
		helloSHA256 + ".zip",
		"packages/" + helloSHA256,
		"packages/" + helloSHA256[:63] + ".zip",
		"packages/" + helloSHA256 + "0.zip",
		"packages/2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824.zip",
		"packages/" + helloSHA256[:63] + "g.zip",
		"datasets/" + helloSHA256 + ".zip",
	} {
		if got := PackageDigest(key); got != "" {
			t.Errorf("PackageDigest(%q) = %q, want none: a digest read from the wrong key would refuse a good package", key, got)
		}
	}
}
