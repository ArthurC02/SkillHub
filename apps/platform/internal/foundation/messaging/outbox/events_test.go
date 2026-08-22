package outbox

// The vocabulary is declared in three places and enforced in two of them, which
// is one more place than any of them can see. These tests are the seam: they fail
// when Go, the DB CHECK and the catalogue stop agreeing, which is the only way a
// "closed set" that lives in three files stays closed.
//
// No database. Reading the .sql text rather than querying pg_constraint is the
// point: this must fail in `go test ./...` on a laptop with no Postgres, not only
// in the integration lane, or the drift it exists to catch ships first.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	migrationPath = "../../../../../../db/migrations/0035_outbox_hardening.sql"
	cataloguePath = "../../../../../../contracts/events/domain-events.md"
)

func TestEventTypesMatchTheMigrationCheck(t *testing.T) {
	assertSameSet(t, "the DB CHECK in 0035", EventTypes, migrationCheckValues(t))
}

func TestEventTypesMatchTheCatalogue(t *testing.T) {
	assertSameSet(t, "contracts/events/domain-events.md §3", EventTypes, catalogueEventTypes(t))
}

// Every run status must produce an event, and every event it produces must be in
// the set. Together with the two tests above, this is what makes adding a status
// to the enum impossible to do quietly: the new status has no mapping, so this
// fails; give it one, and the migration and the catalogue must gain it too.
func TestEveryRunStatusMapsIntoTheClosedSet(t *testing.T) {
	statuses := []gen.RunStatus{
		gen.RunStatusQueued, gen.RunStatusProvisioning, gen.RunStatusPreparing,
		gen.RunStatusRunning, gen.RunStatusEvaluating, gen.RunStatusSucceeded,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	}
	for _, status := range statuses {
		event, err := StatusEvent(string(status))
		if err != nil {
			t.Errorf("run status %q has no domain event: %v", status, err)
			continue
		}
		if !slices.Contains(EventTypes, event) {
			t.Errorf("run status %q maps to %q, which is not in the closed set", status, event)
		}
	}
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusCleaned, gen.RunCleanupStatusFailed} {
		event, err := CleanupEvent(string(status))
		if err != nil {
			t.Errorf("cleanup status %q has no domain event: %v", status, err)
			continue
		}
		if !slices.Contains(EventTypes, event) {
			t.Errorf("cleanup status %q maps to %q, which is not in the closed set", status, event)
		}
	}
}

// The refusal is the feature: an unmapped status must stop the transaction, not
// invent an event type nobody catalogued.
func TestUnmappedStatusesAreRefused(t *testing.T) {
	if event, err := StatusEvent("teleporting"); err == nil {
		t.Errorf("an unknown run status produced %q, want an error", event)
	}
	// The two cleanup statuses that are not outcomes. Announcing "cleanup is still
	// running" as a completed fact is the bug this refusal prevents.
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusPending, gen.RunCleanupStatusCleaningUp} {
		if event, err := CleanupEvent(string(status)); err == nil {
			t.Errorf("cleanup status %q produced %q, want an error", status, event)
		}
	}
}

func TestInsertRefusesWithoutTransaction(t *testing.T) {
	if err := Insert(t.Context(), nil, NewEvent{}); err == nil {
		t.Error("outbox insert succeeded without the caller transaction")
	}
}

// --- reading the other two declarations --------------------------------------

// migrationCheckValues reads the quoted values out of 0035's CHECK body. Scoped
// to the constraint rather than the whole file so a comment that happens to
// mention an event type cannot pass for an enforced value.
func migrationCheckValues(t *testing.T) []string {
	t.Helper()
	sql := readFile(t, migrationPath)
	const open = "CHECK (event_type IN ("
	start := strings.Index(sql, open)
	if start < 0 {
		t.Fatalf("%s no longer declares the event_type CHECK", migrationPath)
	}
	body := sql[start+len(open):]
	end := strings.Index(body, "))")
	if end < 0 {
		t.Fatalf("%s: the event_type CHECK is not closed", migrationPath)
	}
	var values []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(body[:end], -1) {
		values = append(values, m[1])
	}
	return values
}

// catalogueEventTypes reads §3's backtick-quoted tokens. Only whole tokens count:
// the section also carries `run.<status>` and `run.succeeded|failed|timed_out` as
// prose shorthand, and neither is an event type.
func catalogueEventTypes(t *testing.T) []string {
	t.Helper()
	doc := readFile(t, cataloguePath)
	start := strings.Index(doc, "\n## 3.")
	if start < 0 {
		t.Fatalf("%s no longer has a §3", cataloguePath)
	}
	section := doc[start:]
	if end := strings.Index(section, "\n## 4."); end >= 0 {
		section = section[:end]
	}
	seen := map[string]bool{}
	var types []string
	for _, m := range regexp.MustCompile("`(run\\.[a-z_]+)`").FindAllStringSubmatch(section, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			types = append(types, m[1])
		}
	}
	return types
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertSameSet compares as sets, not sequences: the catalogue groups four event
// types into one table cell and the migration lists them one per line, so
// requiring the same order would only ever produce a false failure.
func assertSameSet(t *testing.T, other string, ours, theirs []string) {
	t.Helper()
	a, b := slices.Sorted(slices.Values(ours)), slices.Sorted(slices.Values(theirs))
	if slices.Equal(a, b) {
		return
	}
	for _, v := range a {
		if !slices.Contains(b, v) {
			t.Errorf("%q is in outbox.EventTypes but missing from %s", v, other)
		}
	}
	for _, v := range b {
		if !slices.Contains(a, v) {
			t.Errorf("%q is in %s but missing from outbox.EventTypes", v, other)
		}
	}
}
