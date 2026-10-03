package dockerdrv

import "testing"

func TestTheWorkSpaceHoldsAtLeastTheQuarterOfDiskThePlatformBudgetsForInputs(t *testing.T) {
	for _, disk := range []int64{4, 64 << 20, 1 << 30, 8 << 30} {
		if got := workShare(disk); got < disk/4 || got > disk {
			t.Errorf("workShare(%d) = %d, want between a quarter of the disk (%d) and all of it: "+
				"the platform lets a run's inputs, which land in /work, take a quarter", disk, got, disk/4)
		}
	}
	if got := workShare(8 << 30); got != 6<<30 {
		t.Errorf("workShare(8 GiB) = %d, want 6 GiB", got)
	}
}
