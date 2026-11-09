package run

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func databaseEnum(t *testing.T, typeName string) []string {
	t.Helper()
	const migration = "db/migrations/0004_test_lab_and_runs.sql"
	raw, err := os.ReadFile(filepath.Join(sandboxModuleRoot, migration))
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}
	src := string(raw)
	start := strings.Index(src, "CREATE TYPE "+typeName+" AS ENUM (")
	if start < 0 {
		t.Fatalf("%s no longer declares %s", migration, typeName)
	}
	end := strings.Index(src[start:], ");")
	var values []string
	for _, m := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(src[start:start+end], -1) {
		values = append(values, m[1])
	}
	if len(values) < 4 {
		t.Fatalf("read %d values of %s, want the whole enum", len(values), typeName)
	}
	return values
}

func TestEveryRunStatusTheDatabaseAllowsMapsIntoTheClosedSet(t *testing.T) {
	for _, status := range databaseEnum(t, "run_status") {
		event := StatusChanged{RunStatusChanged: outbox.RunStatusChanged{ToStatus: status}}.eventType()
		if !slices.Contains(outbox.EventTypes, event) {
			t.Errorf("run status %q maps to %q, which is not in the closed set", status, event)
		}
	}
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusCleaned, gen.RunCleanupStatusFailed} {
		if event, ok := cleanupEvents[status]; !ok || !slices.Contains(outbox.EventTypes, event) {
			t.Errorf("cleanup status %q maps to %q, which is not in the closed set", status, event)
		}
	}
}

func TestEveryCleanupStatusTheDatabaseAllowsHasItsOwnWords(t *testing.T) {
	for _, status := range databaseEnum(t, "run_cleanup_status") {
		if word := cleanupWord(gen.RunCleanupStatus(status)); word.Label == status {
			t.Errorf("cleanup status %q falls back to its raw value: %+v", status, word)
		}
	}
}

func TestOnlySettledCleanupStatusesHaveAnEvent(t *testing.T) {
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusPending, gen.RunCleanupStatusCleaningUp} {
		if event, ok := cleanupEvents[status]; ok {
			t.Errorf("cleanup status %q produced %q, want no event", status, event)
		}
	}
}
