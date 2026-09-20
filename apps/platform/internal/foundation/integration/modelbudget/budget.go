package modelbudget

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// Margin is how far below the caller's own deadline a sent ceiling must stay:
// below it the caller stops waiting while the gateway call keeps running and
// keeps billing.
const Margin = 5 * time.Second

const (
	MinSeconds = 1
	maxReason  = 1000
)

var (
	ErrUnknownKind    = errors.New("modelbudget: no such model endpoint")
	ErrReasonRequired = errors.New("modelbudget: a reason is required")
	ErrOutOfRange     = errors.New("modelbudget: seconds outside what the platform allows")
	ErrNotSet         = errors.New("modelbudget: no operator value is set for this endpoint")
)

// An Endpoint is one model call the platform makes, named by the kind an
// operator sees and bounded by the deadline the calling code will wait for.
type Endpoint struct {
	Kind     string
	Deadline time.Duration
}

// Ceiling is the largest value an operator may set for this endpoint.
func (e Endpoint) Ceiling() int {
	return int((e.Deadline - Margin) / time.Second)
}

func (e Endpoint) valid(seconds int) bool {
	return seconds >= MinSeconds && seconds <= e.Ceiling()
}

// A Setting is one endpoint's ceiling as an operator left it.
type Setting struct {
	Kind    string
	Seconds int
	Reason  string
	SetAt   time.Time
}

// A Service reads and writes the operator-set ceilings. Endpoints is the
// roster the composition root supplies: modelbudget does not know which model
// calls exist, only what was stored against their names.
type Service struct {
	Pool      *pgxpool.Pool
	Endpoints []Endpoint
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

func (s *Service) endpoint(kind string) (Endpoint, bool) {
	for _, e := range s.Endpoints {
		if e.Kind == kind {
			return e, true
		}
	}
	return Endpoint{}, false
}

// Within is how long to let this endpoint's call run: the operator's value when
// one is set and valid, otherwise the longest that leaves Margin. A failed
// lookup falls back to that same default rather than failing the model call.
func (s *Service) Within(ctx context.Context, e Endpoint) time.Duration {
	fallback := time.Duration(e.Ceiling()) * time.Second
	if s == nil || s.Pool == nil {
		return fallback
	}
	set, err := s.Get(ctx, e.Kind)
	if err != nil {
		if !errors.Is(err, ErrNotSet) {
			slog.Warn("modelbudget: falling back to the compiled ceiling",
				"kind", e.Kind, "error", err)
		}
		return fallback
	}
	if !e.valid(set.Seconds) {
		return fallback
	}
	return time.Duration(set.Seconds) * time.Second
}

// Get is the operator's value for one endpoint, or ErrNotSet.
func (s *Service) Get(ctx context.Context, kind string) (Setting, error) {
	rows, err := s.queries().ListModelCallBudgets(ctx)
	if err != nil {
		return Setting{}, err
	}
	for _, row := range rows {
		if row.Kind == kind {
			return settingOf(row), nil
		}
	}
	return Setting{}, ErrNotSet
}

// List reports every endpoint on the roster, whether or not it has a value.
// Endpoints the roster does not name are left out: a row for a model call the
// platform no longer makes is not a setting anyone can act on.
func (s *Service) List(ctx context.Context) ([]Setting, []Endpoint, error) {
	rows, err := s.queries().ListModelCallBudgets(ctx)
	if err != nil {
		return nil, nil, err
	}
	stored := map[string]Setting{}
	for _, row := range rows {
		stored[row.Kind] = settingOf(row)
	}
	settings := make([]Setting, 0, len(s.Endpoints))
	for _, e := range s.Endpoints {
		settings = append(settings, stored[e.Kind])
	}
	return settings, s.Endpoints, nil
}

// Set records an operator's ceiling for one endpoint, with the reason and the
// audit event in the same transaction.
func (s *Service) Set(ctx context.Context, kind string, seconds int, reason string,
	actor pgtype.UUID,
) (Setting, error) {
	e, known := s.endpoint(kind)
	if !known {
		return Setting{}, ErrUnknownKind
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Setting{}, ErrReasonRequired
	}
	if len(reason) > maxReason {
		return Setting{}, fmt.Errorf("%w: at most %d bytes", ErrReasonRequired, maxReason)
	}
	if !e.valid(seconds) {
		return Setting{}, fmt.Errorf("%w: %s accepts %d to %d seconds",
			ErrOutOfRange, kind, MinSeconds, e.Ceiling())
	}

	before, err := s.Get(ctx, kind)
	if err != nil && !errors.Is(err, ErrNotSet) {
		return Setting{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Setting{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := s.queries().WithTx(tx).SetModelCallBudget(ctx, gen.SetModelCallBudgetParams{
		Kind:    kind,
		Seconds: int32(seconds),
		Reason:  reason,
		SetBy:   actor,
	})
	if err != nil {
		return Setting{}, err
	}
	if err := audit.Log(ctx, tx, changeEvent(actor, kind, reason, before.Seconds, seconds)); err != nil {
		return Setting{}, err
	}
	return settingOf(row), tx.Commit(ctx)
}

// Clear returns one endpoint to the compiled ceiling. Clearing an endpoint
// that has no value is still an operator action, so it is still audited.
func (s *Service) Clear(ctx context.Context, kind, reason string, actor pgtype.UUID) error {
	if _, known := s.endpoint(kind); !known {
		return ErrUnknownKind
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	removed, err := s.queries().WithTx(tx).ClearModelCallBudget(ctx, kind)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := audit.Log(ctx, tx, changeEvent(actor, kind, reason, int(removed.Seconds), 0)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func changeEvent(actor pgtype.UUID, kind, reason string, before, after int) audit.Event {
	return audit.Event{
		Actor:        actor,
		Action:       audit.ActionModelBudgetSet,
		ResourceType: audit.ResourceModelBudget,
		Metadata: map[string]any{
			"kind":   kind,
			"reason": reason,
			"before": secondsOrDefault(before),
			"after":  secondsOrDefault(after),
			"scope":  audit.ScopeOperator,
		},
	}
}

func secondsOrDefault(seconds int) any {
	if seconds <= 0 {
		return "default"
	}
	return seconds
}

func settingOf(row gen.ModelCallBudget) Setting {
	return Setting{
		Kind:    row.Kind,
		Seconds: int(row.Seconds),
		Reason:  row.Reason,
		SetAt:   row.SetAt.Time,
	}
}
