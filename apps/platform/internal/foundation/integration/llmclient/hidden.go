package llmclient

import (
	"bytes"
	"unicode"
)

// Characters a person cannot see but a model reads, removed from every request
// that leaves for a model (04 丙-210, SEC-013/LLM01).
//
// # The channel
//
// Unicode has a whole family of code points that render as nothing: the Tags
// block (U+E0000–U+E007F) mirrors ASCII one-for-one, the bidirectional
// overrides reorder what a reader sees, and Sneaky Bits encodes arbitrary bits
// with two invisible math operators (U+2062, U+2064). A page can therefore
// read as harmless prose to a person and as a complete instruction set to a
// model. The creation loop is an ideal host for it: the fetch tool puts whole
// attacker-written pages into Messages, and every step sends the full Messages
// back, so a hidden instruction survives the entire session while the screen
// shows nothing unusual.
//
// # Why a category and not a list
//
// Naming the known tricks is a blacklist, and the author of Sneaky Bits makes
// exactly that point: stripping the Tags block alone is not enough, because
// the encoding characters are configurable. Every trick above is Unicode
// category Cf (Format), so one category test covers the ones that exist and
// the ones nobody has published yet.
//
// The two exceptions are orthography, not smuggling: ZWNJ and ZWJ are how
// Indic scripts and emoji sequences are spelled. Variation selectors are left
// alone too — they are Mn rather than Cf, and removing them would change how
// emoji present.
//
// Known cost, and it is the same one AWS accepts in its own guidance:
// subdivision flag emoji (🏴󠁧󠁢󠁳󠁣󠁴󠁿 and friends) are built from Tags characters and
// degrade to a plain black flag on the way to the model.
//
// # Where this does NOT run
//
// Only outbound, and only toward a model. The stored conversation keeps every
// byte the person or the page actually contained, because the screen has the
// opposite job: apps/web reveals these characters rather than removing them.
// A Skill body is what somebody approves and iron rule 4 makes it immutable —
// silently rewriting it before display would be asking a person to sign
// something they were not shown. Removing it before a model reads it, and
// showing it to the human who has to judge it, are the same policy applied to
// two readers with opposite needs.
//
// # Deliberately not normalizing
//
// The usual companion advice is NFKC, and here it would do damage: this
// product is Traditional Chinese end to end and NFKC folds full-width
// punctuation into ASCII. Normalization is also its own injection surface —
// U+FF07 becomes a real apostrophe under NFKC — so a value validated before
// normalization is not the value that comes out. If anything ever needs
// normalizing here it is NFC, after validation.
func hidden(r rune) bool {
	// Written as escapes on purpose: a literal here would be a character this
	// file's own reader cannot see, which is the problem, not the fix.
	if r == '\u200c' || r == '\u200d' { // ZWNJ, ZWJ
		return false
	}
	return unicode.Is(unicode.Cf, r)
}

// withoutHidden strips those characters from an already-marshalled request
// body.
//
// It runs on the JSON rather than on each field for one reason: JSON's own
// syntax is ASCII, so nothing removable can appear outside a string value, and
// the document stays valid. Doing it per field would mean listing them —
// Messages, Brief, SampleInput, DiagramUnderstanding, the draft body, every
// packaged file, and whatever the next field is — and that list is what
// eventually misses one. Go's encoder has already replaced any invalid UTF-8
// with U+FFFD by this point, so lone surrogates are not a case here.
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
