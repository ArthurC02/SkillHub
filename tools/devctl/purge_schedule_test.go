package main

import (
	"strings"
	"testing"
)

func TestEveryRealMaintenanceJobHasACronLine(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := purgeScheduleProblems(root); len(problems) > 0 {
		t.Fatalf("%d maintenance job(s) with no cron line:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

func TestTheMaintenanceSwitchScanStillFindsTheJobs(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	names, err := maintenanceSubcommands(root + "/" + maintenanceMain)
	if err != nil {
		t.Fatal(err)
	}

	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	if len(names) < purgeSchedFloor {
		t.Errorf("the switch scan found %d subcommands (%v); the floor is %d", len(names), names, purgeSchedFloor)
	}
	for _, n := range []string{"purge-accounts", "rotate-partitions", "collect-objects"} {
		if !have[n] {
			t.Errorf("the switch scan no longer finds %q; it found %v", n, names)
		}
	}
}

const fixtureMaintenance = `package main

func main() {
	switch os.Args[1] {
	case "purge-accounts":
		err = purgeAccounts(ctx, pool)
	case "purge-audit":
		err = purgeAudit(ctx, pool)
	case "purge-datasets":
		err = purgeDatasets(ctx, pool)
	case "purge-analytics":
		err = purgeAnalytics(ctx, pool)
	case "purge-run-artifacts":
		err = purgeRunArtifacts(ctx, pool)
	case "rotate-partitions":
		err = rotatePartitions(ctx, pool)
	case "collect-objects":
		err = collectObjects(ctx, pool)
	}
}
`

func writePurgeFixture(t *testing.T, deployment string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, maintenanceMain, fixtureMaintenance)
	writeAt(t, root, deploymentDoc,
		"# 檢查表\n\n## 1. 程式面\n\n每一個 `purge-*` 都已實作。\n\n## 2. 部署期\n\n"+
			deployment+
			"\n## 3. 負責人動作\n\n`maintenance purge-audit` 接上 cron（這一段不算數）。\n")
	return root
}

const allScheduled = "- [ ] `cmd/maintenance purge-accounts` 接上 cron\n" +
	"- [ ] `cmd/maintenance purge-audit` 接上 cron\n" +
	"- [ ] `cmd/maintenance purge-datasets` 接上 cron\n" +
	"- [ ] `cmd/maintenance purge-analytics` 接上 cron\n" +
	"- [ ] `cmd/maintenance purge-run-artifacts` 接上 cron\n" +
	"- [ ] `cmd/maintenance rotate-partitions` 接上每月 cron\n" +
	"- [ ] `cmd/maintenance collect-objects` 接上每日 cron\n"

func TestPurgeScheduleAcceptsATreeWhereEveryJobIsScheduled(t *testing.T) {
	t.Parallel()
	if problems := purgeScheduleProblems(writePurgeFixture(t, allScheduled)); len(problems) != 0 {
		t.Fatalf("a fully scheduled deployment section was rejected: %v", problems)
	}

	without := strings.Replace(allScheduled, "- [ ] `cmd/maintenance collect-objects` 接上每日 cron\n", "", 1)
	if problems := purgeScheduleProblems(writePurgeFixture(t, without)); len(problems) != 1 || !strings.Contains(problems[0], "collect-objects") {
		t.Fatalf("a job that is not a retention sweep still has to be scheduled, got %v", problems)
	}
}

func TestPurgeScheduleNamesTheJobNobodyScheduled(t *testing.T) {
	t.Parallel()
	without := strings.Replace(allScheduled,
		"- [ ] `cmd/maintenance purge-run-artifacts` 接上 cron\n", "", 1)
	problems := purgeScheduleProblems(writePurgeFixture(t, without))
	if len(problems) != 1 || !strings.Contains(problems[0], "`maintenance purge-run-artifacts` has no cron line") {
		t.Fatalf("want exactly the purge-run-artifacts problem, got %v", problems)
	}
}

func TestPurgeScheduleSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("a mention outside the deployment section does not count", func(t *testing.T) {
		t.Parallel()

		without := strings.Replace(allScheduled, "- [ ] `cmd/maintenance purge-audit` 接上 cron\n", "", 1)
		problems := purgeScheduleProblems(writePurgeFixture(t, without))
		if len(problems) != 1 || !strings.Contains(problems[0], "purge-audit") {
			t.Fatalf("a §3 mention satisfied the check: %v", problems)
		}
	})
	t.Run("the subcommand switch lost its jobs", func(t *testing.T) {
		t.Parallel()
		root := writePurgeFixture(t, allScheduled)
		writeAt(t, root, maintenanceMain,
			"package main\n\nfunc main() {\n\tswitch os.Args[1] {\n\tcase \"collect-objects\":\n\t\treturn\n\t}\n}\n")
		problems := purgeScheduleProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "the switch scan is broken rather than the jobs deleted") {
			t.Fatalf("a near-empty job list was accepted: %v", problems)
		}
	})
	t.Run("the deployment chapter is gone", func(t *testing.T) {
		t.Parallel()
		root := writePurgeFixture(t, allScheduled)
		writeAt(t, root, deploymentDoc, "# 檢查表\n\n## 1. 程式面\n\n"+allScheduled)
		problems := purgeScheduleProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "no `## 2.` deployment chapter") {
			t.Fatalf("a checklist with no deployment chapter was accepted: %v", problems)
		}
	})
	t.Run("a cron-less mention inside the section does not count", func(t *testing.T) {
		t.Parallel()
		swapped := strings.Replace(allScheduled,
			"- [ ] `cmd/maintenance purge-datasets` 接上 cron\n",
			"- [ ] `cmd/maintenance purge-datasets` 已實作，排程另議\n", 1)
		problems := purgeScheduleProblems(writePurgeFixture(t, swapped))
		if len(problems) != 1 || !strings.Contains(problems[0], "purge-datasets") {
			t.Fatalf("a mention without a schedule satisfied the check: %v", problems)
		}
	})
}
