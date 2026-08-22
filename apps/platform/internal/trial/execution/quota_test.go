package run

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRequireQuotaRequiresTransaction(t *testing.T) {
	err := (&Service{}).requireQuota(t.Context(), nil, pgtype.UUID{})
	if !errors.Is(err, errQuotaTransactionRequired) {
		t.Fatalf("requireQuota() error = %v, want %v", err, errQuotaTransactionRequired)
	}
}
