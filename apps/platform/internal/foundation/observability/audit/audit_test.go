package audit

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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
	if len(db.args) != 7 {
		t.Fatalf("insert args = %d, want 7", len(db.args))
	}
	if db.args[3] != ActionLogin || db.args[4] != ResourceSession {
		t.Fatalf("insert action/resource = %v/%v", db.args[3], db.args[4])
	}
	if got := string(db.args[6].([]byte)); got != `{"result":"ok"}` {
		t.Fatalf("metadata = %s", got)
	}
}

func TestTheActorKindFollowsWhichActorIsSet(t *testing.T) {
	someone := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	for _, tc := range []struct {
		name   string
		record Record
		want   ActorKind
	}{
		{name: "a person", record: Record{Actor: someone}, want: ActorPerson},
		{name: "an agent", record: Record{Agent: someone}, want: ActorAgent},
		{name: "neither is the platform", record: Record{}, want: ActorSystem},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.record.ActorKind(); got != tc.want {
				t.Errorf("ActorKind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLogRefusesWithoutDatabaseHandle(t *testing.T) {
	if err := Log(t.Context(), nil, Event{}); err == nil {
		t.Error("audit log succeeded without a database handle")
	}
}
