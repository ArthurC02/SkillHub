package literalgate

import (
	"bytes"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type requiredLiteral struct {
	text           string
	asciiFoldsOnly bool
}

var required sync.Map

func CanMatch(re *regexp.Regexp, s string) bool {
	return canMatch(literalsOf(re), s, strings.Contains, isASCIIString)
}

func CanMatchBytes(re *regexp.Regexp, b []byte) bool {
	return canMatch(literalsOf(re), b, func(b []byte, lit string) bool { return bytes.Contains(b, []byte(lit)) }, isASCIIBytes)
}

func canMatch[T string | []byte](literals []requiredLiteral, text T, contains func(T, string) bool, ascii func(T) bool) bool {
	checkedASCII, isASCII := false, false
	for _, lit := range literals {
		if !lit.asciiFoldsOnly {
			if !contains(text, lit.text) {
				return false
			}
			continue
		}
		if !checkedASCII {
			checkedASCII, isASCII = true, ascii(text)
		}
		if isASCII && indexASCIIFold(text, lit.text) < 0 {
			return false
		}
	}
	return true
}

func literalsOf(re *regexp.Regexp) []requiredLiteral {
	if found, ok := required.Load(re); ok {
		return found.([]requiredLiteral)
	}
	found, _ := required.LoadOrStore(re, literalsEveryMatchContains(re))
	return found.([]requiredLiteral)
}

// literalsEveryMatchContains walks the top-level concatenation (and its groups)
// for literals no match can avoid. A case-insensitive ASCII literal gates only
// ASCII text: its other folds (Kelvin sign, long s) need non-ASCII bytes.
func literalsEveryMatchContains(re *regexp.Regexp) []requiredLiteral {
	parsed, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return nil
	}
	var out []requiredLiteral
	var collect func(*syntax.Regexp)
	collect = func(node *syntax.Regexp) {
		switch node.Op {
		case syntax.OpLiteral:
			if lit, ok := literalOf(node); ok {
				out = append(out, lit)
			}
		case syntax.OpConcat:
			for _, sub := range node.Sub {
				collect(sub)
			}
		case syntax.OpCapture:
			collect(node.Sub[0])
		}
	}
	collect(parsed)
	return out
}

func literalOf(node *syntax.Regexp) (requiredLiteral, bool) {
	text := string(node.Rune)
	if node.Flags&syntax.FoldCase == 0 || !anyRuneFolds(node.Rune) {
		return requiredLiteral{text: text}, true
	}
	if !isASCIIString(text) {
		return requiredLiteral{}, false
	}
	return requiredLiteral{text: strings.ToLower(text), asciiFoldsOnly: true}, true
}

func anyRuneFolds(runes []rune) bool {
	for _, r := range runes {
		if unicode.SimpleFold(r) != r {
			return true
		}
	}
	return false
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func isASCIIBytes(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func indexASCIIFold[T string | []byte](text T, lower string) int {
	for i := 0; i+len(lower) <= len(text); i++ {
		if asciiLower(text[i]) != lower[0] {
			continue
		}
		matched := true
		for j := 1; j < len(lower); j++ {
			if asciiLower(text[i+j]) != lower[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func asciiLower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
