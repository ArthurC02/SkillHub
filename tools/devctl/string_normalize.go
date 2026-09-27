package main

import "strings"

func trimQuoted(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func parseKeyValue(line, delimiter string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, delimiter)
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), trimQuoted(value), true
}
