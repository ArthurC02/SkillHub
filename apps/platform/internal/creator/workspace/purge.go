package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type ObjectRemover interface {
	Remove(ctx context.Context, key string) error
}

type WorkspacePurge func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error

type WorkspaceObjectKeys func(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) ([]string, error)

type WorkspaceQuiescence func(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error)

var errAccountPurgeDeferred = errors.New("identity: account purge deferred while workspace work is active")

type purgeStep struct {
	context string
	purge   WorkspacePurge
}

func (s *Service) purgeSteps() []purgeStep {
	return []purgeStep{
		{"analytics", s.PurgeAnalytics},
		{"testlab", s.PurgeTestData},
		{"run", s.PurgeRunArtifacts},
		{"packaging", s.PurgeDownloads},
		{"creation", s.PurgeCreation},

		// registry before ingest: registry deletes the skill_versions rows,
		// and ingest only removes import sources no remaining version points
		// at. Reversed, every source still backs a live version and none is removed.
		{"registry", s.PurgeSkills},
		{"ingest", s.PurgeImportSources},
	}
}

type objectKeyStep struct {
	context string
	list    WorkspaceObjectKeys
}

func (s *Service) objectKeySteps() []objectKeyStep {
	return []objectKeyStep{
		{"testlab", s.DatasetObjectKeys},
		{"run", s.RunArtifactObjectKeys},
		{"packaging", s.DownloadArtifactObjectKeys},
	}
}

func (s *Service) requirePurgeSteps() error {
	var missing []string
	for _, step := range s.purgeSteps() {
		if step.purge == nil {
			missing = append(missing, step.context)
		}
	}

	for _, step := range s.objectKeySteps() {
		if step.list == nil {
			missing = append(missing, step.context+" (object keys)")
		}
	}
	if s.WorkspaceQuiescent == nil {
		missing = append(missing, "run (purge readiness)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("identity: account purge steps not injected for %s; refusing to purge",
			strings.Join(missing, ", "))
	}
	return nil
}

func (s *Service) PurgeExpiredAccounts(ctx context.Context, store ObjectRemover, grace time.Duration, limit int32) (purged int, err error) {
	if err := s.requirePurgeSteps(); err != nil {
		return 0, err
	}
	q := s.queries()
	ids, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{
		Cutoff: pgconv.Timestamptz(time.Now().Add(-grace)),
		Limit:  limit,
	})
	if err != nil {
		return 0, err
	}
	var failures []error
	for _, id := range ids {
		if err := s.purgeAccount(ctx, store, id); err != nil {
			if errors.Is(err, errAccountPurgeDeferred) {
				continue
			}

			slog.Error("account purge failed", "user_id", uuidText(id), "error", err)
			failures = append(failures, fmt.Errorf("purge account %s: %w", uuidText(id), err))
			continue
		}
		purged++
	}

	return purged, errors.Join(failures...)
}

func (s *Service) purgeAccount(ctx context.Context, store ObjectRemover, userID pgtype.UUID) error {
	q := s.queries()
	workspaces, err := q.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		return err
	}
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	locked := make([]pgtype.UUID, 0, len(workspaces))
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for i := len(locked) - 1; i >= 0; i-- {
			if _, err := gen.New(conn).UnlockAccountWorkspaceObjects(unlockCtx, locked[i]); err != nil {
				slog.Error("account purge workspace lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		conn.Release()
	}()
	for _, ws := range workspaces {
		if err := gen.New(conn).LockAccountWorkspaceObjects(ctx, ws.ID); err != nil {
			return err
		}
		locked = append(locked, ws.ID)
	}
	for _, ws := range workspaces {
		ready, err := s.WorkspaceQuiescent(ctx, conn, ws.ID)
		if err != nil {
			return fmt.Errorf("check run cleanup readiness: %w", err)
		}
		if !ready {
			return errAccountPurgeDeferred
		}
	}
	started, err := gen.New(conn).MarkAccountPurgeStarted(ctx, userID)
	if err != nil {
		return err
	}
	if started == 0 {
		return errAccountPurgeDeferred
	}

	// Objects removed before the transaction opens: object storage has no
	// rollback, so a later failure leaves rows pointing at missing files
	// (recoverable by the next sweep) rather than an orphaned file no row names.
	for _, ws := range workspaces {
		keys := map[string]struct{}{}
		for _, step := range s.objectKeySteps() {
			owned, err := step.list(ctx, conn, ws.ID)
			if err != nil {
				return fmt.Errorf("list %s object keys: %w", step.context, err)
			}
			for _, key := range owned {
				keys[key] = struct{}{}
			}
		}
		for key := range keys {
			if err := store.Remove(ctx, key); err != nil {
				return err
			}
		}
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return err
	}
	q = gen.New(tx)

	for _, ws := range workspaces {
		for _, step := range s.purgeSteps() {
			if err := step.purge(ctx, tx, ws.ID); err != nil {
				return fmt.Errorf("purge %s: %w", step.context, err)
			}
		}
	}
	if _, err := q.DeleteUserIdentities(ctx, userID); err != nil {
		return err
	}
	if _, err := q.DeleteUserSessions(ctx, userID); err != nil {
		return err
	}
	if _, err := q.AnonymizeWorkspacesByOwner(ctx, userID); err != nil {
		return err
	}
	if _, err := q.AnonymizeUser(ctx, userID); err != nil {
		return err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        userID,
		Action:       audit.ActionAccountPurge,
		ResourceType: audit.ResourceAccount,
		ResourceID:   userID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func uuidText(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
