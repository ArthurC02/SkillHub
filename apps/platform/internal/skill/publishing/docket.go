package publishing

import (
	"context"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type DocketEntry struct {
	State    ExposureState
	Snapshot *SearchSnapshot
	Findings skillpkg.CategorizedFindings
}

func (s *Service) ExposureDocket(ctx context.Context) ([]DocketEntry, error) {
	if err := s.requireSnapshotRead(); err != nil {
		return nil, err
	}
	queue, err := s.ExposureQueue(ctx)
	if err != nil {
		return nil, err
	}
	q := gen.New(s.Pool)
	docket := make([]DocketEntry, 0, len(queue))
	for _, state := range queue {
		entry := DocketEntry{State: state}
		snapshot, found, err := s.ReadSearchSnapshot(ctx, state.SkillID)
		if err != nil {
			return nil, err
		}
		if found {
			entry.Snapshot = &snapshot
		}
		releases, err := releasesOf(ctx, q, state.PublicationID)
		if err != nil {
			return nil, err
		}
		if len(releases) > 0 {
			entry.Findings = releases[0].Findings
		}
		docket = append(docket, entry)
	}
	return docket, nil
}
