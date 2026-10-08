package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type ProposalStatus string

const (
	ProposalProposed  ProposalStatus = "proposed"
	ProposalApproved  ProposalStatus = "approved"
	ProposalRejected  ProposalStatus = "rejected"
	ProposalExpired   ProposalStatus = "expired"
	ProposalRunning   ProposalStatus = "running"
	ProposalSucceeded ProposalStatus = "succeeded"
	ProposalFailed    ProposalStatus = "failed"
)

const proposalLifetime = 24 * time.Hour

var liveProposalStatuses = []string{string(ProposalProposed), string(ProposalApproved), string(ProposalRunning)}

var (
	ErrUnknownProposal = errors.New("operations: no proposal has that id")
	ErrProposalClosed  = errors.New("operations: the proposal is no longer waiting for a decision")
)

type ProposalRequest struct {
	Action string   `json:"action"`
	Reason string   `json:"reason"`
	Cites  []string `json:"cites"`
}

type preparedProposal struct {
	ProposalRequest
	tier    ActionTier
	preview Preview
}

func prepareProposals(ctx context.Context, def Definition, actions []Action, requests []ProposalRequest) ([]preparedProposal, error) {
	prepared := make([]preparedProposal, 0, len(requests))
	for _, request := range requests {
		if !slices.Contains(def.Actions, request.Action) {
			return nil, fmt.Errorf("operations: the agent proposed %q, which it is not allowed to propose", request.Action)
		}
		i := slices.IndexFunc(actions, func(a Action) bool { return a.Name == request.Action })
		if i < 0 {
			return nil, fmt.Errorf("operations: no action named %q is registered", request.Action)
		}
		preview, err := actions[i].Preview(ctx)
		if err != nil {
			return nil, fmt.Errorf("operations: previewing %q: %w", request.Action, err)
		}
		if preview.changesNothing() {
			continue
		}
		prepared = append(prepared, preparedProposal{ProposalRequest: request, tier: actions[i].Tier, preview: preview})
	}
	return prepared, nil
}

func (s *Service) recordProposals(ctx context.Context, tx pgx.Tx, run pgtype.UUID, proposals []preparedProposal, now time.Time) error {
	if len(proposals) == 0 {
		return nil
	}
	q := gen.New(tx)
	agentID, err := q.GetPlatformAgentRunAgent(ctx, run)
	if err != nil {
		return err
	}
	for _, p := range proposals {
		live, err := q.LiveProposalExists(ctx, gen.LiveProposalExistsParams{Action: p.Action, Live: liveProposalStatuses})
		if err != nil {
			return err
		}
		if live {
			continue
		}
		preview, err := json.Marshal(p.preview)
		if err != nil {
			return err
		}
		id, err := q.ProposeAction(ctx, gen.ProposeActionParams{
			AgentID: agentID, RunID: run, Action: p.Action, Tier: string(p.tier), Reason: p.Reason,
			Cites: p.Cites, Preview: preview,
			ProposedAt: pgconv.Timestamptz(now), ExpiresAt: pgconv.Timestamptz(now.Add(proposalLifetime)),
		})
		if err != nil {
			return err
		}
		if err := audit.Log(ctx, tx, audit.Event{
			Agent: agentID, Action: audit.ActionProposalPropose, ResourceType: audit.ResourcePlatformAgentProposal,
			ResourceID: id, Metadata: map[string]any{"run": pgconv.UUIDString(run), "action": p.Action},
		}); err != nil {
			return err
		}
	}
	return nil
}

type decision struct {
	status ProposalStatus
	audit  string
	closes bool
}

var (
	approve = decision{status: ProposalApproved, audit: audit.ActionProposalApprove}
	reject  = decision{status: ProposalRejected, audit: audit.ActionProposalReject, closes: true}
)

func (s *Service) DecideProposal(ctx context.Context, id pgtype.UUID, d decision, operator pgtype.UUID, note string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var finished pgtype.Timestamptz
		if d.closes {
			finished = pgconv.Timestamptz(time.Now())
		}
		if _, err := q.DecideProposal(ctx, gen.DecideProposalParams{
			Status: string(d.status), DecidedBy: operator, Note: &note, FinishedAt: finished, ID: id,
		}); errors.Is(err, pgx.ErrNoRows) {
			return closedOrUnknown(ctx, q, id)
		} else if err != nil {
			return err
		}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: d.audit, ResourceType: audit.ResourcePlatformAgentProposal, ResourceID: id,
			Metadata: map[string]any{auditNote: note},
		})
	})
}

func closedOrUnknown(ctx context.Context, q *gen.Queries, id pgtype.UUID) error {
	if _, err := q.GetProposal(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return ErrUnknownProposal
	} else if err != nil {
		return err
	}
	return ErrProposalClosed
}

func (s *Service) ExpireProposals(ctx context.Context) (int, error) {
	var expired int
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		ids, err := gen.New(tx).ExpireProposals(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := systemAudit(ctx, tx, id, audit.ActionProposalExpire, nil); err != nil {
				return err
			}
		}
		expired = len(ids)
		return nil
	})
	return expired, err
}

type ClaimedProposal struct {
	ID     pgtype.UUID
	Action string
}

func (s *Service) ClaimApprovedProposal(ctx context.Context) (ClaimedProposal, bool, error) {
	row, err := gen.New(s.Pool).ClaimApprovedProposal(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaimedProposal{}, false, nil
	}
	if err != nil {
		return ClaimedProposal{}, false, err
	}
	return ClaimedProposal{ID: row.ID, Action: row.Action}, true, nil
}

func (s *Service) FinishProposal(ctx context.Context, id pgtype.UUID, failure error) error {
	status, action, metadata := ProposalSucceeded, audit.ActionProposalSucceed, map[string]any(nil)
	var outcome *string
	if failure != nil {
		message := failure.Error()
		if strings.TrimSpace(message) == "" {
			message = outcomeFailedSilently
		}
		status, action, outcome = ProposalFailed, audit.ActionProposalFail, &message
		metadata = map[string]any{auditError: message}
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		finished, err := gen.New(tx).FinishProposal(ctx, gen.FinishProposalParams{Status: string(status), Outcome: outcome, ID: id})
		if err != nil {
			return err
		}
		if finished == 0 {
			return ErrProposalClosed
		}
		return systemAudit(ctx, tx, id, action, metadata)
	})
}

const (
	auditError            = "error"
	proposalRunLease      = 2 * time.Hour
	outcomeFailedSilently = "the job failed without saying why"
	outcomeAbandoned      = "the maintenance process stopped before it reported an outcome"
)

func (s *Service) AbandonStaleProposals(ctx context.Context) (int, error) {
	var abandoned int
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		ids, err := gen.New(tx).AbandonStaleProposals(ctx, gen.AbandonStaleProposalsParams{
			Outcome: new(outcomeAbandoned), StartedBefore: pgconv.Timestamptz(time.Now().Add(-proposalRunLease)),
		})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := systemAudit(ctx, tx, id, audit.ActionProposalFail, map[string]any{auditError: outcomeAbandoned}); err != nil {
				return err
			}
		}
		abandoned = len(ids)
		return nil
	})
	return abandoned, err
}

func systemAudit(ctx context.Context, tx pgx.Tx, id pgtype.UUID, action string, metadata map[string]any) error {
	return audit.Log(ctx, tx, audit.Event{
		Action: action, ResourceType: audit.ResourcePlatformAgentProposal, ResourceID: id, Metadata: metadata,
	})
}
