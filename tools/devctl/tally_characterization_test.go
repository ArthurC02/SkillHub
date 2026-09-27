package main

import (
	"strings"
	"testing"
)

func TestTheFirstRowOfARepeatedBacklogIDDecidesWhetherItIsOpen(t *testing.T) {
	t.Parallel()
	root := writeBacklog(t,
		"| 丙 | **1** | 逐列重數 <!-- open: 1 --> |",
		"| **丙-1** | a | b | 等部署 |\n| **丙-1** | a | b | 已結案 |\n")
	if problems := backlogTallyProblems(root); len(problems) != 0 {
		t.Fatalf("a later mention of an open row closed it: %v", problems)
	}
}

func TestTheBaselineTotalMustAgreeOnTheBlockingAndWarningSplit(t *testing.T) {
	t.Parallel()
	owner := strings.Replace(baselineValidOwnerBody, "合計：3 項檢查（阻擋 2 項、告警 1 項）", "合計：3 項檢查（阻擋 1 項、告警 2 項）", 1)
	problems := strings.Join(baselineTallyProblems(writeBaseline(t, owner)), "\n")
	if !strings.Contains(problems, "says 合計 3 項（阻擋 1、告警 2） but its rows are 3（阻擋 2、告警 1）") {
		t.Fatalf("a total with the right sum and the wrong split was accepted: %v", problems)
	}
}
