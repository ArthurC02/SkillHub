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

// DDD-018 left this command without a wiring test on the grounds that each
// subcommand is a single struct literal next to its only caller, which a reader
// checks by looking at it. The account purge broke that: its literal now carries
// injected steps, one per context that owns rows in a workspace (ADR-034),
// and this is the process that actually runs the purge — the API only offers the
// button. Dropping one of them is a compliance hole, so it gets a test.
//
// Reflection rather than named assertions so that another context added to
// identity.Service is covered here the day it appears, not the day somebody
// remembers this file. No database and no object storage: a nil pool is never
// dialled because nothing here queries.
func TestPurgeServiceCarriesEveryContextsStep(t *testing.T) {
	svc := reflect.ValueOf(*purgeService(nil))
	for i := range svc.NumField() {
		field := svc.Field(i)
		if (field.Type() == reflect.TypeFor[identity.WorkspacePurge]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceObjectKeys]()) && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: purge-accounts would refuse to run",
				svc.Type().Field(i).Name)
		}
	}
}

// TestEveryPartitionedTableIsRotated ties the two constants rotate-partitions
// reaches for to the migrations that declare them. It catches the two ways this
// job silently stops covering everything: a typo in an owner's PartitionedTable
// (the job would fail on "relation does not exist", but only on the deployment
// that ran it), and a third partitioned table added later whose owner never
// wires it in — that one has no symptom at all until the month its default
// partition is the only thing left catching writes.
//
// Reads the migration files rather than a database, so it runs everywhere.
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
			// `CREATE TABLE x (` opens a definition; the `PARTITION BY RANGE`
			// that closes it can be many lines later.
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

// Fail-closed on the retention windows. Neither TRACE_RETENTION nor
// ANALYTICS_RETENTION has a default, because both numbers are PDM-006 proposals
// nobody has ratified and this job's mistake — a dropped month — does not come
// back. Unset must therefore refuse rather than pick something reasonable.
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

// The same fail-closed rule as above, asserted on the subcommand rather than on
// the helper, because the mistake this catches is not "positiveDuration accepts
// junk" -- it is a job that reads no window at all and sweeps with a compiled-in
// 30 days. PDM-006 6.1's 30 days is unratified and what this one deletes is the
// user's own content, so an unset variable has to stop it before any statement
// runs. A nil pool proves it did: reaching the database would panic.
func TestPurgeDeletedSkillsRefusesWithoutAGracePeriod(t *testing.T) {
	for _, unusable := range []string{"", "30", "0s", "-720h", "thirty days"} {
		t.Setenv("SKILL_DELETION_GRACE", unusable)
		err := purgeDeletedSkills(context.Background(), nil)
		if err == nil {
			t.Errorf("SKILL_DELETION_GRACE=%q started the purge", unusable)
			continue
		}
		// The operator's next action is to set it, so the variable has to be
		// named in what they see.
		if !strings.Contains(err.Error(), "SKILL_DELETION_GRACE") {
			t.Errorf("SKILL_DELETION_GRACE=%q: error does not name the variable: %v", unusable, err)
		}
	}
}
