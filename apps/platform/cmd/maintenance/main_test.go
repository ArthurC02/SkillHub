package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func TestPurgeServiceCarriesEveryContextsStep(t *testing.T) {
	svc := reflect.ValueOf(*purgeService(nil))
	for i := range svc.NumField() {
		field := svc.Field(i)
		if (field.Type() == reflect.TypeFor[identity.WorkspacePurge]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceObjectKeys]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceQuiescence]()) && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: purge-accounts would refuse to run",
				svc.Type().Field(i).Name)
		}
	}
}

func TestEveryPartitionedTableIsRotated(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	partitioned := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var table string
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "--") {
				continue
			}

			if fields := strings.Fields(line); len(fields) >= 3 &&
				fields[0] == "CREATE" && fields[1] == "TABLE" && !strings.Contains(line, "PARTITION OF") {
				table = fields[2]
			}
			if strings.Contains(line, "PARTITION BY RANGE") && table != "" {
				partitioned[table] = true
			}
		}
	}

	rotated := map[string]bool{
		trace.PartitionedTable:     true,
		analytics.PartitionedTable: true,
	}
	for table := range partitioned {
		if !rotated[table] {
			t.Errorf("db/migrations declares %s PARTITION BY RANGE but rotate-partitions never touches it", table)
		}
	}
	for table := range rotated {
		if !partitioned[table] {
			t.Errorf("rotate-partitions names %s but no migration declares it PARTITION BY RANGE", table)
		}
	}
}

func TestRetentionWindowsHaveNoDefault(t *testing.T) {
	for _, unusable := range []string{"", "90", "0s", "-24h", "ninety days"} {
		t.Setenv("TRACE_RETENTION", unusable)
		if _, err := positiveDuration("TRACE_RETENTION"); err == nil {
			t.Errorf("TRACE_RETENTION=%q accepted", unusable)
		}
	}
	t.Setenv("TRACE_RETENTION", "2160h")
	if d, err := positiveDuration("TRACE_RETENTION"); err != nil || d != 2160*time.Hour {
		t.Errorf("positiveDuration = %s, %v", d, err)
	}
}

func TestPurgeDeletedSkillsRefusesWithoutAGracePeriod(t *testing.T) {
	for _, unusable := range []string{"", "30", "0s", "-720h", "thirty days"} {
		t.Setenv("SKILL_DELETION_GRACE", unusable)
		err := purgeDeletedSkills(context.Background(), nil)
		if err == nil {
			t.Errorf("SKILL_DELETION_GRACE=%q started the purge", unusable)
			continue
		}

		if !strings.Contains(err.Error(), "SKILL_DELETION_GRACE") {
			t.Errorf("SKILL_DELETION_GRACE=%q: error does not name the variable: %v", unusable, err)
		}
	}
}

func TestPurgeDatabaseURLFallsBackToTheAPIRole(t *testing.T) {
	t.Setenv("SKILLHUB_PURGE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://api-role@db/skillhub")
	if got := purgeDatabaseURL(); got != "postgres://api-role@db/skillhub" {
		t.Errorf("purgeDatabaseURL() = %q, want DATABASE_URL's value", got)
	}

	t.Setenv("SKILLHUB_PURGE_DATABASE_URL", "postgres://skillhub_purge@db/skillhub")
	if got := purgeDatabaseURL(); got != "postgres://skillhub_purge@db/skillhub" {
		t.Errorf("purgeDatabaseURL() = %q, want SKILLHUB_PURGE_DATABASE_URL's value", got)
	}
}

func TestPurgeFeedbackRefusesWithoutARetentionWindow(t *testing.T) {
	for _, unusable := range []string{"", "180", "0s", "-4320h", "six months"} {
		t.Setenv("FEEDBACK_RETENTION", unusable)
		err := purgeFeedback(context.Background(), nil)
		if err == nil {
			t.Errorf("FEEDBACK_RETENTION=%q started the purge", unusable)
			continue
		}
		if !strings.Contains(err.Error(), "FEEDBACK_RETENTION") {
			t.Errorf("FEEDBACK_RETENTION=%q: error does not name the variable: %v", unusable, err)
		}
	}
}
