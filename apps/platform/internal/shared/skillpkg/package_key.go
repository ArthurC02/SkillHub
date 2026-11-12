package skillpkg

import (
	"crypto/sha256"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/textx"
)

const (
	packageKeyPrefix = "packages/"
	packageKeySuffix = ".zip"
)

func PackageObjectKey(data []byte) (key, digest string) {
	digest = textx.SHA256Hex(data)
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
