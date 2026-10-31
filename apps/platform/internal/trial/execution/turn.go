package run

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const turnWindow = 500

type waitingRun struct {
	run           gen.Run
	sandboxesHeld int
}

func (a waitingRun) turnOrder(b waitingRun) int {
	return cmp.Or(
		cmp.Compare(a.sandboxesHeld, b.sandboxesHeld),
		a.run.CreatedAt.Time.Compare(b.run.CreatedAt.Time),
		bytes.Compare(a.run.ID.Bytes[:], b.run.ID.Bytes[:]),
	)
}

type runLine struct {
	waiting []gen.Run
	held    map[[16]byte]int
}

func lineOf(unfinished []gen.Run) runLine {
	line := runLine{held: map[[16]byte]int{}}
	for _, r := range unfinished {
		switch {
		case r.Status != gen.RunStatusQueued:
			line.held[r.WorkspaceID.Bytes]++
		case !r.CancelRequestedAt.Valid:
			line.waiting = append(line.waiting, r)
		}
	}
	return line
}

func (l runLine) standing(r gen.Run) waitingRun {
	return waitingRun{run: r, sandboxesHeld: l.held[r.WorkspaceID.Bytes]}
}

func (l runLine) ahead(of gen.Run) []gen.Run {
	me := l.standing(of)
	var ahead []gen.Run
	for _, other := range l.waiting {
		if l.standing(other).turnOrder(me) < 0 {
			ahead = append(ahead, other)
		}
	}
	return ahead
}

const messageQueuedBehind = "前面還有 %d 個試跑在排隊；輪到時會自動開始，不必重新送出。"

func explainQueuePosition(ctx context.Context, q *gen.Queries, runID pgtype.UUID, status, reason string) string {
	if status != string(gen.RunStatusQueued) || reason != "" {
		return reason
	}
	ahead, counted := runsAhead(ctx, q, runID)
	if !counted || ahead == 0 {
		return reason
	}
	return fmt.Sprintf(messageQueuedBehind, ahead)
}

func runsAhead(ctx context.Context, q *gen.Queries, runID pgtype.UUID) (int, bool) {
	unfinished, err := q.ListUnfinishedRuns(ctx, turnWindow)
	if err != nil {
		slog.Warn("could not read the queue; the run page says nothing about its place in it",
			"run_id", pgconv.UUIDString(runID), "error", err)
		return 0, false
	}
	return queuePosition(unfinished, runID)
}

func queuePosition(unfinished []gen.Run, runID pgtype.UUID) (int, bool) {
	if len(unfinished) >= turnWindow {
		return 0, false
	}
	line := lineOf(unfinished)
	i := slices.IndexFunc(line.waiting, func(r gen.Run) bool { return r.ID == runID })
	if i < 0 {
		return 0, false
	}
	return len(line.ahead(line.waiting[i])), true
}

func (s *Service) turnBelongsToAnother(ctx context.Context, current gen.Run, placements []Placement, setAside map[string]SetAsideProvider) (bool, error) {
	unfinished, err := s.queries().ListUnfinishedRuns(ctx, turnWindow)
	if err != nil {
		return false, err
	}
	free := 0
	for _, p := range placements {
		free += p.freeSlots()
	}
	ahead := lineOf(unfinished).ahead(current)
	policies, err := s.policiesOf(ctx, ahead)
	if err != nil {
		return false, err
	}
	contenders := 0
	for _, other := range ahead {
		if s.providers().sharesPlacement(ctx, policies[other.ID.Bytes], placements, setAside) {
			contenders++
		}
	}
	return contenders >= free, nil
}

func (r *Registry) sharesPlacement(ctx context.Context, policy []byte, placements []Placement, setAside map[string]SetAsideProvider) bool {
	req, _, err := requirementsFor(policy)
	if err != nil {
		return false
	}
	theirs, err := r.Place(ctx, req, setAside)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(theirs, func(t Placement) bool {
		return slices.ContainsFunc(placements, func(p Placement) bool { return p.Provider.Name() == t.Provider.Name() })
	})
}
