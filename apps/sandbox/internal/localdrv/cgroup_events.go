package localdrv

import (
	"strconv"
	"strings"
)

func eventCount(raw, key string) (int64, bool) {
	for _, line := range strings.Split(raw, "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found || name != key {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func eventOccurred(raw, key string) bool {
	n, ok := eventCount(raw, key)
	return ok && n > 0
}
