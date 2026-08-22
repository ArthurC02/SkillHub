package audit

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recordingDB struct {
	args []any
}

func (db *recordingDB) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	db.args = args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (*recordingDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	panic("unexpected Query")
}

func (*recordingDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	panic("unexpected QueryRow")
}

func TestLogBuildsPersistenceParamsInsideAudit(t *testing.T) {
	db := &recordingDB{}
	if err := Log(t.Context(), db, Event{
		Action: ActionLogin, ResourceType: ResourceSession,
		Metadata: map[string]any{"result": "ok"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(db.args) != 6 {
		t.Fatalf("insert args = %d, want 6", len(db.args))
	}
	if db.args[2] != ActionLogin || db.args[3] != ResourceSession {
		t.Fatalf("insert action/resource = %v/%v", db.args[2], db.args[3])
	}
	if got := string(db.args[5].([]byte)); got != `{"result":"ok"}` {
		t.Fatalf("metadata = %s", got)
	}
}

func TestLogRefusesWithoutDatabaseHandle(t *testing.T) {
	if err := Log(t.Context(), nil, Event{}); err == nil {
		t.Error("audit log succeeded without a database handle")
	}
}
