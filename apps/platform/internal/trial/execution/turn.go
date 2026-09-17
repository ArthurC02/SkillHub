package run

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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

func (s *Service) turnBelongsToAnother(ctx context.Context, current gen.Run, placements []Placement, halted map[string]gen.DispatchHalt) (bool, error) {
	unfinished, err := s.queries().ListUnfinishedRuns(ctx, turnWindow)
	if err != nil {
		return false, err
	}
	free := 0
	for _, p := range placements {
		free += p.freeSlots()
	}
	contenders := 0
	for _, other := range lineOf(unfinished).ahead(current) {
		if s.providers().sharesPlacement(ctx, other, placements, halted) {
			contenders++
		}
	}
	return contenders >= free, nil
}

func (r *Registry) sharesPlacement(ctx context.Context, other gen.Run, placements []Placement, halted map[string]gen.DispatchHalt) bool {
	req, _, err := requirementsFor(other)
	if err != nil {
		return false
	}
	theirs, err := r.Place(ctx, req, halted)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(theirs, func(t Placement) bool {
		return slices.ContainsFunc(placements, func(p Placement) bool { return p.Provider.Name == t.Provider.Name })
	})
}
