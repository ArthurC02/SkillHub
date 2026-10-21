package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExpiredFeedbackRefusesWithoutAPoolOrAPositiveRetention(t *testing.T) {
	cases := []struct {
		name      string
		svc       *Service
		retention time.Duration
		count     string
		purge     string
	}{
		{"nil service", nil, time.Hour,
			"feedback count requires a database pool", "feedback purge requires a database pool"},
		{"nil pool", &Service{}, time.Hour,
			"feedback count requires a database pool", "feedback purge requires a database pool"},
		{"zero retention", &Service{Pool: &pgxpool.Pool{}}, 0,
			"feedback count requires a positive retention period", "feedback purge requires a positive retention period"},
		{"negative retention", &Service{Pool: &pgxpool.Pool{}}, -time.Second,
			"feedback count requires a positive retention period", "feedback purge requires a positive retention period"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.svc.CountExpiredFeedback(context.Background(), c.retention); err == nil || err.Error() != c.count {
				t.Errorf("count error = %v, want %q", err, c.count)
			}
			if _, err := c.svc.PurgeExpiredFeedback(context.Background(), c.retention); err == nil || err.Error() != c.purge {
				t.Errorf("purge error = %v, want %q", err, c.purge)
			}
		})
	}
}
