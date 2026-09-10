package llmclient

import (
	"bytes"
	"unicode"
)

// hidden matches the whole Unicode Format (Cf) category rather than a list of
// known invisible-character tricks, so unpublished variants are caught too.
// ZWNJ and ZWJ are excluded because they are legitimate script/emoji joiners.
func hidden(r rune) bool {

	if r == '\u200c' || r == '\u200d' {
		return false
	}
	return unicode.Is(unicode.Cf, r)
}

// withoutHidden runs on the marshalled JSON rather than per field, since
// JSON's own syntax is ASCII and every removable character can only occur
// inside a string value.
func withoutHidden(body []byte) []byte {
	if !bytes.ContainsFunc(body, hidden) {
		return body
	}
	return bytes.Map(func(r rune) rune {
		if hidden(r) {
			return -1
		}
		return r
	}, body)
}
