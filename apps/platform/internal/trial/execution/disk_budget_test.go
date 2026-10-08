package run

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestTheLargestInputsARunCanBeGivenFitInAQuarterOfItsDisk(t *testing.T) {
	skill := skillpkg.Limits()
	archive, staging, installed := skill.ZipBytes, skill.UnpackedBytes, skill.UnpackedBytes
	inputs := archive + staging + installed + int64(testlab.MaxTestCaseBytes)

	disk := DefaultResourceLimits().DiskBytes
	if inputs > disk/4 {
		t.Fatalf("the largest inputs a run can be given take %d bytes (package %d, unpacked twice %d, datasets %d) "+
			"but a run's inputs may use at most a quarter of its %d byte disk: raise the disk or lower an input ceiling, "+
			"or runs will fail on space the platform promised",
			inputs, archive, staging+installed, testlab.MaxTestCaseBytes, disk)
	}
}

func TestTheDefaultDiskIsNoLargerThanTheDefaultMemoryItIsBackedBy(t *testing.T) {
	limits := DefaultResourceLimits()
	if limits.DiskBytes > limits.MemoryBytes {
		t.Fatalf("default disk %d exceeds default memory %d, but scratch files are counted against memory",
			limits.DiskBytes, limits.MemoryBytes)
	}
}
