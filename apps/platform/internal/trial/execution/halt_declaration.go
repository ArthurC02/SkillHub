package run

import (
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var (
	ErrHaltReasonRequired = errors.New("run: halting or resuming dispatch needs a reason")
	ErrUnknownHaltSource  = errors.New("run: unknown dispatch halt source")
)

func (s HaltSource) RecoversAutomatically() bool {
	return s == HaltSourceOrphanThreshold
}

func automaticallyRecoveringSources() []HaltSource {
	var recovering []HaltSource
	for _, source := range AllHaltSources() {
		if source.RecoversAutomatically() {
			recovering = append(recovering, source)
		}
	}
	return recovering
}

func requireHaltReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrHaltReasonRequired
	}
	return nil
}

type haltDeclaration struct {
	source HaltSource
	reason string
	actor  pgtype.UUID
}

func newHaltDeclaration(source HaltSource, reason string, actor pgtype.UUID) (haltDeclaration, error) {
	if !slices.Contains(AllHaltSources(), source) {
		return haltDeclaration{}, ErrUnknownHaltSource
	}
	if err := requireHaltReason(reason); err != nil {
		return haltDeclaration{}, err
	}
	return haltDeclaration{source: source, reason: reason, actor: actor}, nil
}

func (d haltDeclaration) over(active gen.DispatchHalt) gen.DispatchHalt {
	next := active
	next.ClearRounds = 0
	if HaltSource(active.Source) == HaltSourceIncident && d.source != HaltSourceIncident {
		return next
	}
	next.Reason = d.reason
	if d.source == HaltSourceIncident {
		next.Source = string(d.source)
		next.DeclaredBy = d.actor
	}
	return next
}
