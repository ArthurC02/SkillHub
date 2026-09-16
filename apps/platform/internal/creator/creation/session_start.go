package creation

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Create(ctx context.Context, ws identity.Workspace, id pgtype.UUID, message string, budget float64) (View, error) {
	if err := s.admitStart(ctx, ws, id, message, budget); err != nil {
		return View{}, err
	}
	key := digest(struct {
		Message string
		Budget  float64
	}{message, budget})
	if r, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID}); err == nil {
		return resumeStart(r, key)
	}
	e, state := s.openingEnvelope(ctx, ws, message, budget, key)
	return s.insertSession(ctx, ws, id, key, e, state)
}

func (s *Service) admitStart(ctx context.Context, ws identity.Workspace, id pgtype.UUID, message string, budget float64) error {
	if s.Billing != nil {
		ok, err := s.Billing.CanStart(ctx, ws.ID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCreditThreshold
		}
	}
	if !s.Limits.Valid() {
		return ErrUnavailable
	}
	if !id.Valid || !finite(budget) || utf8.RuneCountInString(message) > maxPersonMessageRunes {
		return ErrInvalidCommand
	}
	if budget < s.Limits.MaxCallCostUSD || budget > s.Limits.MaxCostUSD {
		return ErrBudgetOutOfBand
	}
	return nil
}

func resumeStart(r gen.CreationSession, key string) (View, error) {
	e, err := decode(r)
	if err != nil {
		return View{}, err
	}
	if e.StartHash != key {
		return View{}, ErrReplayMismatch
	}
	if !live(r) {
		return View{}, ErrNotFound
	}
	return view(r)
}

func (s *Service) openingEnvelope(ctx context.Context, ws identity.Workspace, message string, budget float64, key string) (envelope, State) {
	zero := 0.0
	e := envelope{Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, References: []Reference{}, BudgetUSD: budget, SpentUSD: &zero}, Limits: s.Limits, StartHash: key, Deadline: time.Now().Add(s.Limits.SessionTimeout)}
	if strings.TrimSpace(message) == "" {
		return e, StateWaitingInput
	}
	e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(message)})
	if s.CatalogCheck == nil {
		return e, StateQueued
	}
	refs, cost, err := s.CatalogCheck(ctx, ws, message)
	if err != nil {
		slog.Warn("creation: catalogue check failed, continuing without it", "error", err)
	}
	e.Snapshot.CatalogChecked = err == nil
	addSpend(&e.Snapshot, cost)
	if len(refs) == 0 {
		return e, StateQueued
	}
	e.Snapshot.References = shortlist(refs)
	e.Snapshot.PendingAction = PendingReferenceChoice
	return e, StateWaitingConfirmation
}

func (s *Service) insertSession(ctx context.Context, ws identity.Workspace, id pgtype.UUID, key string, e envelope, state State) (View, error) {
	b, _ := json.Marshal(e)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.CreateCreationSession(ctx, gen.CreateCreationSessionParams{ID: id, WorkspaceID: ws.ID, State: string(state), Snapshot: b, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.Limits.Retention), Valid: true}})
	if err != nil {
		_ = tx.Rollback(ctx)
		return s.startedConcurrently(ctx, ws, id, key)
	}
	if err = q.AppendCreationEvent(ctx, gen.AppendCreationEventParams{SessionID: id, WorkspaceID: ws.ID, Revision: 1, EventType: "created", Snapshot: b}); err != nil {
		return View{}, err
	}
	if state == StateQueued {
		if _, err = s.enqueue(ctx, tx, row, &e, false); err != nil {
			return View{}, err
		}
		if row, err = s.advance(ctx, tx, row, state, "started", e); err != nil {
			return View{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return view(row)
}

func (s *Service) startedConcurrently(ctx context.Context, ws identity.Workspace, id pgtype.UUID, key string) (View, error) {
	r, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID})
	if err != nil {
		return View{}, ErrConflict
	}
	return resumeStart(r, key)
}
