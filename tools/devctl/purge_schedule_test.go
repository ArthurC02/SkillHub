package main

import (
	"strings"
	"testing"
)

// Pointed at the tree. RED until the missing cron lines are written, and the
// failure names which sweeps have no schedule rather than just failing.
func TestEveryRealMaintenanceSweepHasACronLine(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := purgeScheduleProblems(root); len(problems) > 0 {
		t.Fatalf("%d retention sweep(s) with no cron line:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// The subcommand scan has to keep finding subcommands. If it stopped, the check
// above would pass by having nothing to schedule.
func TestTheMaintenanceSwitchScanStillFindsTheSweeps(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	names, err := maintenanceSubcommands(root + "/" + maintenanceMain)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately not a list of every sweep: the names move (purge-analytics
	// became purge-feedback on 2026-08-29) and a test that pins them turns a
	// rename into a failure of the wrong thing. What must not move is that the
	// scan finds a switch full of sweeps, and the two that have been there
	// since M4.
	sweeps := 0
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
		if strings.HasPrefix(n, purgeSchedPrefix) || n == rotateSubcommand {
			sweeps++
		}
	}
	if sweeps < purgeSchedFloor {
		t.Errorf("the switch scan found %d sweeps (%v); the floor is %d", sweeps, names, purgeSchedFloor)
	}
	for _, n := range []string{"purge-accounts", rotateSubcommand} {
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
	"- [ ] `cmd/maintenance rotate-partitions` 接上每月 cron\n"

func TestPurgeScheduleAcceptsATreeWhereEverySweepIsScheduled(t *testing.T) {
	t.Parallel()
	if problems := purgeScheduleProblems(writePurgeFixture(t, allScheduled)); len(problems) != 0 {
		t.Fatalf("a fully scheduled deployment section was rejected: %v", problems)
	}
	// `collect-objects` is not a retention sweep and must not be demanded.
	if strings.Contains(strings.Join(purgeScheduleProblems(writePurgeFixture(t, allScheduled)), ""), "collect-objects") {
		t.Fatal("collect-objects is not a purge- or rotate-partitions sweep and must not be required")
	}
}

func TestPurgeScheduleNamesTheSweepNobodyScheduled(t *testing.T) {
	t.Parallel()
	without := strings.Replace(allScheduled,
		"- [ ] `cmd/maintenance purge-run-artifacts` 接上 cron\n", "", 1)
	problems := purgeScheduleProblems(writePurgeFixture(t, without))
	if len(problems) != 1 || !strings.Contains(problems[0], "`maintenance purge-run-artifacts` is a retention sweep with no cron line") {
		t.Fatalf("want exactly the purge-run-artifacts problem, got %v", problems)
	}
}

// The two ways this passes while proving nothing.
func TestPurgeScheduleSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("a mention outside the deployment section does not count", func(t *testing.T) {
		t.Parallel()
		// §1 and §3 both name purge-audit; only §2 schedules anything.
		without := strings.Replace(allScheduled, "- [ ] `cmd/maintenance purge-audit` 接上 cron\n", "", 1)
		problems := purgeScheduleProblems(writePurgeFixture(t, without))
		if len(problems) != 1 || !strings.Contains(problems[0], "purge-audit") {
			t.Fatalf("a §3 mention satisfied the check: %v", problems)
		}
	})
	t.Run("the subcommand switch stopped having sweeps", func(t *testing.T) {
		t.Parallel()
		root := writePurgeFixture(t, allScheduled)
		writeAt(t, root, maintenanceMain,
			"package main\n\nfunc main() {\n\tswitch os.Args[1] {\n\tcase \"collect-objects\":\n\t\treturn\n\t}\n}\n")
		problems := purgeScheduleProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "the switch scan is broken rather than the sweeps deleted") {
			t.Fatalf("an empty sweep list was accepted: %v", problems)
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
