package skillpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	packageKeyPrefix = "packages/"
	packageKeySuffix = ".zip"
)

func PackageObjectKey(data []byte) (key, digest string) {
	sum := sha256.Sum256(data)
	digest = hex.EncodeToString(sum[:])
	return packageKeyPrefix + digest + packageKeySuffix, digest
}

func PackageDigest(objectKey string) string {
	digest, ok := strings.CutPrefix(objectKey, packageKeyPrefix)
	if !ok {
		return ""
	}
	digest, ok = strings.CutSuffix(digest, packageKeySuffix)
	if !ok || len(digest) != sha256.Size*2 || strings.Trim(digest, "0123456789abcdef") != "" {
		return ""
	}
	return digest
}
