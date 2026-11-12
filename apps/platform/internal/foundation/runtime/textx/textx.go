package textx

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func CutRunes(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

func CutBytes(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	return strings.ToValidUTF8(s[:limit], ""), true
}

func Ellipsize(s string, limit int) string {
	if cut, truncated := CutRunes(s, limit); truncated {
		return cut + "…"
	}
	return s
}

func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
