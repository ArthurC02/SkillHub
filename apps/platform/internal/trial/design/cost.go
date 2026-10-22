package testlab

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrCreditThreshold  = errors.New("點數不足，無法開始建議驗收條件，沒有呼叫模型。請聯絡管理者為這個帳號加點。")
	ErrModelCallsHalted = errors.New("平台暫停中，這次沒有呼叫模型、也沒有扣點。驗收條件可以自己手動輸入，或稍後再試。")
)

type SuggestBilling struct {
	ModelCallsHalted func(ctx context.Context) bool
	CanStart         func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)
	Charge           func(ctx context.Context, workspaceID pgtype.UUID, usage *ModelUsage) error
}

func (b SuggestBilling) admit(ctx context.Context, workspaceID pgtype.UUID) error {
	if b.ModelCallsHalted != nil && b.ModelCallsHalted(ctx) {
		return ErrModelCallsHalted
	}
	if b.CanStart == nil {
		return nil
	}
	ok, err := b.CanStart(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCreditThreshold
	}
	return nil
}

func (b SuggestBilling) charge(ctx context.Context, workspaceID pgtype.UUID, usage *ModelUsage) {
	if b.Charge == nil {
		return
	}
	if err := b.Charge(context.WithoutCancel(ctx), workspaceID, usage); err != nil {
		slog.Error("testlab: suggesting criteria not charged", "error", err)
	}
}
