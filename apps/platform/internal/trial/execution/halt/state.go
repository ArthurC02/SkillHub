package halt

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const Pool = ""

type Halt struct {
	ID          pgtype.UUID
	Provider    string
	Source      Source
	Reason      string
	DeclaredAt  *time.Time
	ClearRounds int32
}

type State struct {
	byTarget map[string]gen.DispatchHalt
}

func StateOf(rows ...gen.DispatchHalt) State {
	state := State{byTarget: make(map[string]gen.DispatchHalt, len(rows))}
	for _, row := range rows {
		state.byTarget[row.Provider] = row
	}
	return state
}

func failedClosed() State {
	return StateOf(gen.DispatchHalt{Provider: Pool, Source: string(Incident), Reason: "halt state unreadable"})
}

func (h State) active(target string) (gen.DispatchHalt, bool) {
	halt, ok := h.byTarget[target]
	return halt, ok
}

func (h State) Halts() []Halt {
	halts := make([]Halt, 0, len(h.byTarget))
	for _, row := range h.byTarget {
		halts = append(halts, haltOf(row))
	}
	return halts
}

func (h State) IncidentHeld(provider string) bool {
	if halt, ok := h.active(Pool); ok && Source(halt.Source) == Incident {
		return true
	}
	if provider == Pool {
		return false
	}
	halt, ok := h.active(provider)
	return ok && Source(halt.Source) == Incident
}

func (h State) DispatchPaused(providers []string) bool {
	if _, ok := h.active(Pool); ok {
		return true
	}
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		if _, ok := h.active(provider); !ok {
			return false
		}
	}
	return true
}

func (h State) IncidentPaused(providers []string) bool {
	if h.IncidentHeld(Pool) {
		return true
	}
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		if !h.IncidentHeld(provider) {
			return false
		}
	}
	return true
}

func Target(provider string) string {
	if provider == Pool {
		return "pool"
	}
	return provider
}

func haltOf(row gen.DispatchHalt) Halt {
	var declaredAt *time.Time
	if row.DeclaredAt.Valid {
		at := row.DeclaredAt.Time
		declaredAt = &at
	}
	return Halt{
		ID:          row.ID,
		Provider:    row.Provider,
		Source:      Source(row.Source),
		Reason:      row.Reason,
		DeclaredAt:  declaredAt,
		ClearRounds: row.ClearRounds,
	}
}
