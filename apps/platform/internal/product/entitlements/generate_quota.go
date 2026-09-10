package policy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrGenerateQuotaExceeded = errors.New("this workspace has used its free generation allowance")

const (
	generateDaily       = 10
	generateWindow      = 30
	generateFirstWindow = 20
	generateWindowDays  = 30
)

func DefaultGenerateQuotaLimits() QuotaLimits {
	return QuotaLimits{
		Daily:       generateDaily,
		Window:      generateWindow,
		FirstWindow: generateFirstWindow,
		WindowDays:  generateWindowDays,
	}
}

func EnforceGenerateQuota(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID,
) (string, error) {
	return enforce(ctx, reader, l, workspaceID, allowance{
		sentinel: ErrGenerateQuotaExceeded, noun: "generations", prefix: "generate_quota",
	})
}
