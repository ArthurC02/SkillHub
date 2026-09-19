package testlab

import (
	"context"
	"log/slog"
)

func (s *Service) recordSuggestCost(ctx context.Context, u *ModelUsage) {
	if s.RecordSpend == nil {
		return
	}
	if err := s.RecordSpend(ctx, u); err != nil {
		slog.Warn("testlab: suggesting criteria cost not recorded", "error", err)
	}
}
