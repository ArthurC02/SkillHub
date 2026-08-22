package identity

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestReadWorkspaceCreatedAtRequiresTransaction(t *testing.T) {
	_, err := ReadWorkspaceCreatedAt(t.Context(), nil, pgtype.UUID{})
	if !errors.Is(err, errWorkspaceCreatedAtTransactionRequired) {
		t.Fatalf("ReadWorkspaceCreatedAt() error = %v, want %v", err, errWorkspaceCreatedAtTransactionRequired)
	}
}
