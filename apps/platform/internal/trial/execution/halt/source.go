package halt

import (
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Source string

const (
	Incident Source = "p1_incident"

	OrphanThreshold Source = "orphan_threshold"
)

func AllSources() []Source {
	return []Source{Incident, OrphanThreshold}
}

var (
	ErrReasonRequired = errors.New("halt: halting or resuming dispatch needs a reason")
	ErrUnknownSource  = errors.New("halt: unknown dispatch halt source")
)

func (s Source) RecoversAutomatically() bool {
	return s == OrphanThreshold
}

func AutomaticallyRecoveringSources() []Source {
	var recovering []Source
	for _, source := range AllSources() {
		if source.RecoversAutomatically() {
			recovering = append(recovering, source)
		}
	}
	return recovering
}

func requireReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	return nil
}

type declaration struct {
	source Source
	reason string
	actor  pgtype.UUID
}

func newDeclaration(source Source, reason string, actor pgtype.UUID) (declaration, error) {
	if !slices.Contains(AllSources(), source) {
		return declaration{}, ErrUnknownSource
	}
	if err := requireReason(reason); err != nil {
		return declaration{}, err
	}
	return declaration{source: source, reason: reason, actor: actor}, nil
}

func (d declaration) over(active gen.DispatchHalt) gen.DispatchHalt {
	next := active
	next.ClearRounds = 0
	if Source(active.Source) == Incident && d.source != Incident {
		return next
	}
	next.Reason = d.reason
	if d.source == Incident {
		next.Source = string(d.source)
		next.DeclaredBy = d.actor
	}
	return next
}
