package registry

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var (
	ErrVersionDisabled        = errors.New("skill version is disabled")
	ErrVersionAlreadyDisabled = errors.New("skill version is already disabled")
	ErrDisableReasonRequired  = errors.New("a version disable needs a reason")
	ErrDisableReasonTooLong   = errors.New("a version disable reason exceeds 1000 bytes")
)

type OperatorVersionStatus struct {
	ID            pgtype.UUID
	VersionNumber int32
	Disabled      bool
}

func (s *Service) OperatorVersionStatus(ctx context.Context, versionID pgtype.UUID) (OperatorVersionStatus, error) {
	row, err := gen.New(s.Pool).GetOperatorVersionStatus(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OperatorVersionStatus{}, ErrNotFound
	}
	if err != nil {
		return OperatorVersionStatus{}, err
	}
	return OperatorVersionStatus{ID: row.ID, VersionNumber: row.VersionNumber, Disabled: row.Disabled}, nil
}

func (s *Service) VersionDisabled(ctx context.Context, workspaceID, versionID pgtype.UUID) (bool, error) {
	disabled, err := gen.New(s.Pool).GetWorkspaceVersionDisableStatus(ctx, gen.GetWorkspaceVersionDisableStatusParams{
		WorkspaceID: workspaceID, VersionID: versionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return disabled, err
}

func (s *Service) AdmitVersion(ctx context.Context, tx pgx.Tx, workspaceID, versionID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.LockWorkspaceVersionAdmission(ctx, gen.LockWorkspaceVersionAdmissionParams{
		WorkspaceID: workspaceID, VersionID: versionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	disabled, err := q.GetWorkspaceVersionDisableStatus(ctx, gen.GetWorkspaceVersionDisableStatusParams{
		WorkspaceID: workspaceID, VersionID: versionID,
	})
	if err != nil {
		return err
	}
	if disabled {
		return ErrVersionDisabled
	}
	return nil
}

func (s *Service) DisableVersion(ctx context.Context, versionID, actor pgtype.UUID, reason string) (OperatorVersionStatus, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return OperatorVersionStatus{}, ErrDisableReasonRequired
	}
	if len(reason) > 1000 {
		return OperatorVersionStatus{}, ErrDisableReasonTooLong
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return OperatorVersionStatus{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	locked, err := q.LockOperatorVersion(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OperatorVersionStatus{}, ErrNotFound
	}
	if err != nil {
		return OperatorVersionStatus{}, err
	}
	status, err := q.GetOperatorVersionStatus(ctx, versionID)
	if err != nil {
		return OperatorVersionStatus{}, err
	}
	if !status.Disabled {
		if err := q.InsertSkillVersionDisable(ctx, versionID); err != nil {
			return OperatorVersionStatus{}, err
		}
	}
	action := audit.ActionSkillVersionDisable
	if status.Disabled {
		action = audit.ActionSkillVersionDisableAttempt
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: locked.WorkspaceID, Action: action,
		ResourceType: audit.ResourceVersion, ResourceID: versionID,
		Metadata: map[string]any{
			"reason": reason, "scope": audit.ScopeOperator,
			"previous_value": status.Disabled, "new_value": true,
		},
	}); err != nil {
		return OperatorVersionStatus{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OperatorVersionStatus{}, err
	}
	if status.Disabled {
		return OperatorVersionStatus{}, ErrVersionAlreadyDisabled
	}
	return OperatorVersionStatus{ID: versionID, VersionNumber: locked.VersionNumber, Disabled: true}, nil
}
