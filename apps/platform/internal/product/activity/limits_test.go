package activity

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestTheServiceRefusesALimitOutsideOneToAHundred(t *testing.T) {
	for _, c := range []struct {
		limit  int
		wantOK bool
	}{{0, false}, {1, true}, {100, true}, {101, false}} {
		_, err := completeService().List(context.Background(), pgtype.UUID{}, "", c.limit)
		if (err == nil) != c.wantOK {
			t.Errorf("limit %d: err = %v, want accepted = %v", c.limit, err, c.wantOK)
		}
	}
}

func cursorOf(json string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(json))
}

func TestAWellFormedCursorWithAnImpossibleFieldIsRefused(t *testing.T) {
	cases := []struct {
		name, cursor string
		wantOK       bool
	}{
		{"the lowest rank", `{"rank":3,"activity_at":"2026-01-01T00:00:00Z","kind":"run","source_id":"x"}`, true},
		{"a rank past the lowest", `{"rank":4,"activity_at":"2026-01-01T00:00:00Z","kind":"run","source_id":"x"}`, false},
		{"no source id", `{"rank":0,"activity_at":"2026-01-01T00:00:00Z","kind":"run","source_id":""}`, false},
		{"no kind", `{"rank":0,"activity_at":"2026-01-01T00:00:00Z","kind":"","source_id":"x"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := completeService().List(context.Background(), pgtype.UUID{}, cursorOf(c.cursor), 50)
			if c.wantOK && err != nil {
				t.Fatalf("err = %v, want the cursor accepted", err)
			}
			if !c.wantOK && !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("err = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestAnUnclassifiedItemSortsAfterARecentOne(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	unknown := Item{Kind: "run", SourceID: "a", Classification: "something-new", ActivityAt: at.Add(time.Hour)}
	recent := Item{Kind: "run", SourceID: "b", Classification: Recent, ActivityAt: at}
	if compare(unknown, recent) <= 0 {
		t.Fatal("an unclassified item sorted before a recent one; it must rank lowest")
	}
}
