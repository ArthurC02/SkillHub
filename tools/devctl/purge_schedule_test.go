package main

import (
	"strings"
	"testing"
)

func TestEveryRealMaintenanceJobIsScheduled(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := purgeScheduleProblems(root); len(problems) > 0 {
		t.Fatalf("%d maintenance schedule problem(s):\n%s", len(problems), strings.Join(problems, "\n"))
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

const allScheduled = "daily purge-accounts\n" +
	"weekly purge-audit\n" +
	"daily purge-datasets\n" +
	"daily purge-analytics\n" +
	"daily purge-run-artifacts\n" +
	"monthly rotate-partitions\n" +
	"\n" +
	"daily collect-objects\n"

func writePurgeFixture(t *testing.T, schedule string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, maintenanceMain, fixtureMaintenance)
	writeAt(t, root, maintenanceSchedule, schedule)
	for _, period := range []string{"daily", "weekly", "monthly"} {
		writeAt(t, root, maintenanceTimers+"/skillhub-"+period+"@.timer", "[Timer]\n")
	}
	return root
}

func onlyProblem(t *testing.T, problems []string, fragment string) {
	t.Helper()
	if len(problems) != 1 || !strings.Contains(problems[0], fragment) {
		t.Fatalf("want exactly one problem containing %q, got %v", fragment, problems)
	}
}

func TestPurgeScheduleAcceptsATreeWhereEveryJobIsScheduled(t *testing.T) {
	t.Parallel()
	if problems := purgeScheduleProblems(writePurgeFixture(t, allScheduled)); len(problems) != 0 {
		t.Fatalf("a fully scheduled tree was rejected: %v", problems)
	}
}

func TestPurgeScheduleNamesTheJobNobodyScheduled(t *testing.T) {
	t.Parallel()
	without := strings.Replace(allScheduled, "daily collect-objects\n", "", 1)
	onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, without)), "`maintenance collect-objects` has no line in")
}

func TestPurgeScheduleRefusesALineThatCannotRun(t *testing.T) {
	t.Parallel()
	t.Run("a period with no timer", func(t *testing.T) {
		t.Parallel()
		schedule := strings.Replace(allScheduled, "weekly purge-audit", "hourly purge-audit", 1)
		onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, schedule)), `period "hourly" has no`)
	})
	t.Run("a job that is not a subcommand", func(t *testing.T) {
		t.Parallel()
		onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, allScheduled+"daily purge-everything\n")),
			"`maintenance purge-everything` is not a subcommand")
	})
	t.Run("a job scheduled twice", func(t *testing.T) {
		t.Parallel()
		onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, allScheduled+"weekly purge-accounts\n")),
			"`maintenance purge-accounts` is scheduled twice")
	})
	t.Run("a line that is not a period and a job", func(t *testing.T) {
		t.Parallel()
		onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, allScheduled+"daily\n")), "want `<period> <subcommand>`")
	})
	t.Run("a line with an argument the timer would drop", func(t *testing.T) {
		t.Parallel()
		onlyProblem(t, purgeScheduleProblems(writePurgeFixture(t, allScheduled+"weekly purge-audit --dry-run\n")),
			"want `<period> <subcommand>`")
	})
}

func TestPurgeScheduleSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("the subcommand switch lost its jobs", func(t *testing.T) {
		t.Parallel()
		root := writePurgeFixture(t, allScheduled)
		writeAt(t, root, maintenanceMain,
			"package main\n\nfunc main() {\n\tswitch os.Args[1] {\n\tcase \"collect-objects\":\n\t\treturn\n\t}\n}\n")
		onlyProblem(t, purgeScheduleProblems(root), "the switch scan is broken rather than the jobs deleted")
	})
	t.Run("the schedule file is gone", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeAt(t, root, maintenanceMain, fixtureMaintenance)
		onlyProblem(t, purgeScheduleProblems(root), "maintenance-schedule")
	})
}
