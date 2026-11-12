package localdrv

import "github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"

func eventOccurred(raw, key string) bool {
	n, ok := sandbox.CgroupEventCount(raw, key)
	return ok && n > 0
}
