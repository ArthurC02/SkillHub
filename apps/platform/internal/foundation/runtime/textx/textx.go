package textx

func CutRunes(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

func Ellipsize(s string, limit int) string {
	if cut, truncated := CutRunes(s, limit); truncated {
		return cut + "…"
	}
	return s
}
