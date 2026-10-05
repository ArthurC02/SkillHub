package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var ErrInputDigestMismatch = errors.New("the delivered bytes are not the ones the request names")

func (r RunRequest) VerifyInput(g ObjectGrant, body []byte) error {
	want := r.inputDigest(g)
	if want == "" {
		return nil
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("%s %s: %w", g.Purpose, g.ObjectKey, ErrInputDigestMismatch)
	}
	return nil
}

func (r RunRequest) inputDigest(g ObjectGrant) string {
	switch g.Purpose {
	case "skill_package":
		return r.SkillVersion.PackageSHA256
	case "dataset":
		for _, d := range r.TestCase.DatasetRefs {
			if d.ObjectKey == g.ObjectKey {
				return d.ContentHash
			}
		}
	}
	return ""
}
