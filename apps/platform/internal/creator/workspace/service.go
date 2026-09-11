package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const SessionTTL = 30 * 24 * time.Hour

const providerGitHub = "github"

var ErrAccountPurging = errors.New("account deletion is already in progress")

var ErrWorkspaceNotFound = errors.New("workspace not found")

type Service struct {
	Pool  *pgxpool.Pool
	OAuth *GitHubOAuth

	PurgeAnalytics     WorkspacePurge
	PurgeTestData      WorkspacePurge
	PurgeRunArtifacts  WorkspacePurge
	PurgeDownloads     WorkspacePurge
	PurgeCreation      WorkspacePurge
	PurgeSkills        WorkspacePurge
	PurgeImportSources WorkspacePurge

	DatasetObjectKeys          WorkspaceObjectKeys
	RunArtifactObjectKeys      WorkspaceObjectKeys
	DownloadArtifactObjectKeys WorkspaceObjectKeys
	WorkspaceQuiescent         WorkspaceQuiescence
}

func (s *Service) MayStoreObjects(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
	if db == nil {
		return false, errors.New("identity: object eligibility database is not configured")
	}
	return gen.New(db).WorkspaceAcceptsObjects(ctx, workspaceID)
}

func LockObjectWrite(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (bool, error) {
	q := gen.New(db)
	if err := q.LockWorkspaceObjectWrite(ctx, workspaceID); err != nil {
		return false, err
	}
	allowed, err := q.WorkspaceAcceptsObjects(ctx, workspaceID)
	if err != nil {
		return true, err
	}
	if !allowed {
		return true, ErrAccountPurging
	}
	return true, nil
}

func UnlockObjectWrite(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) error {
	_, err := gen.New(db).UnlockWorkspaceObjectWrite(ctx, workspaceID)
	return err
}

type User struct {
	ID                  pgtype.UUID
	Email               string
	DisplayName         string
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
	DeletedAt           pgtype.Timestamptz
	DeletionRequestedAt pgtype.Timestamptz
}

type Workspace struct {
	ID          pgtype.UUID
	OwnerUserID pgtype.UUID
	Name        string
	CreatedAt   pgtype.Timestamptz
	UpdatedAt   pgtype.Timestamptz
	IsCatalog   bool
}

func userDTO(row gen.User) User {
	return User{
		ID: row.ID, Email: row.Email, DisplayName: row.DisplayName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
		DeletionRequestedAt: row.DeletionRequestedAt,
	}
}

func workspaceDTO(row gen.Workspace) Workspace {
	return Workspace{
		ID: row.ID, OwnerUserID: row.OwnerUserID, Name: row.Name,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, IsCatalog: row.IsCatalog,
	}
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

type ExternalIdentity struct {
	Provider       string
	ProviderUserID string
	Email          string
	Name           string
	Login          string
}

func (s *Service) LoginOrSignup(ctx context.Context, id ExternalIdentity) (string, error) {
	user, err := s.queries().GetUserByIdentity(ctx, gen.GetUserByIdentityParams{
		Provider:       id.Provider,
		ProviderUserID: id.ProviderUserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		user, err = s.signup(ctx, id)
	}
	if err != nil {
		return "", err
	}
	if user.PurgeStartedAt.Valid {
		return "", ErrAccountPurging
	}
	return s.mintSession(ctx, user)
}

func (s *Service) signup(ctx context.Context, id ExternalIdentity) (gen.User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries().WithTx(tx)

	// Serializes concurrent first-login attempts for the same external
	// identity so two callbacks racing here cannot each create a user and
	// workspace before the identity's primary key rejects the loser.
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))",
		id.Provider, id.ProviderUserID,
	); err != nil {
		return gen.User{}, err
	}
	if existing, err := q.GetUserByIdentity(ctx, gen.GetUserByIdentityParams{
		Provider: id.Provider, ProviderUserID: id.ProviderUserID,
	}); err == nil {
		return existing, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return gen.User{}, err
	}
	user, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:       id.Email,
		DisplayName: id.Name,
	})
	if err != nil {
		return gen.User{}, err
	}
	if _, err := q.CreateWorkspace(ctx, gen.CreateWorkspaceParams{
		OwnerUserID: user.ID,
		Name:        id.Login,
	}); err != nil {
		return gen.User{}, err
	}
	if err := q.CreateIdentity(ctx, gen.CreateIdentityParams{
		UserID:         user.ID,
		Provider:       id.Provider,
		ProviderUserID: id.ProviderUserID,
	}); err != nil {
		return gen.User{}, err
	}
	return user, tx.Commit(ctx)
}

func (s *Service) mintSession(ctx context.Context, user gen.User) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	session, err := q.CreateSession(ctx, gen.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: pgconv.Timestamptz(time.Now().Add(SessionTTL)),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55000" && pgErr.Message == "session owner is being purged" {
			return "", ErrAccountPurging
		}
		return "", err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        user.ID,
		Action:       audit.ActionLogin,
		ResourceType: audit.ResourceSession,
		ResourceID:   session.ID,
	}); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}

func (s *Service) UserForToken(ctx context.Context, token string) (User, error) {
	row, err := s.queries().GetSessionUser(ctx, hashToken(token))
	return userDTO(row), err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	hash := hashToken(token)
	user, err := s.queries().GetSessionUser(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.queries().DeleteSession(ctx, hash)
	}
	if err != nil {
		return err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := q.DeleteSession(ctx, hash); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        user.ID,
		Action:       audit.ActionLogout,
		ResourceType: audit.ResourceSession,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const AccountDeletionGrace = 30 * 24 * time.Hour

func (s *Service) RequestAccountDeletion(ctx context.Context, user User) (User, error) {
	return s.setDeletionRequest(ctx, user, true)
}

func (s *Service) CancelAccountDeletion(ctx context.Context, user User) (User, error) {
	return s.setDeletionRequest(ctx, user, false)
}

func (s *Service) setDeletionRequest(ctx context.Context, user User, request bool) (User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	var updated gen.User
	action := audit.ActionAccountDeleteStop
	if request {
		updated, err = q.RequestAccountDeletion(ctx, user.ID)
		action = audit.ActionAccountDeleteAsk
	} else {
		updated, err = q.CancelAccountDeletion(ctx, user.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrAccountPurging
	}
	if err != nil {
		return User{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        user.ID,
		Action:       action,
		ResourceType: audit.ResourceAccount,
		ResourceID:   user.ID,
	}); err != nil {
		return User{}, err
	}
	return userDTO(updated), tx.Commit(ctx)
}

func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.queries().DeleteExpiredSessions(ctx)
}

func (s *Service) PersonalWorkspace(ctx context.Context, user User) (Workspace, error) {
	ws, err := s.queries().ListWorkspacesByOwner(ctx, user.ID)
	if err != nil {
		return Workspace{}, err
	}
	if len(ws) == 0 {
		return Workspace{}, errors.New("user has no workspace")
	}
	if len(ws) > 1 {
		return Workspace{}, fmt.Errorf(
			"user has %d workspaces; ADR-011 gives each account exactly one and every workspace scope is derived from this answer, so there is no safe one to pick",
			len(ws))
	}
	return workspaceDTO(ws[0]), nil
}

func (s *Service) DisplayNames(ctx context.Context, userIDs []pgtype.UUID) (map[pgtype.UUID]string, error) {
	names := make(map[pgtype.UUID]string, len(userIDs))
	if len(userIDs) == 0 {
		return names, nil
	}
	rows, err := gen.New(s.Pool).ListUserDisplayNames(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		names[row.ID] = row.DisplayName
	}
	return names, nil
}

func (s *Service) WorkspaceOwner(ctx context.Context, workspaceID pgtype.UUID) (pgtype.UUID, error) {
	return s.WorkspaceOwnerIn(ctx, s.Pool, workspaceID)
}

func (s *Service) WorkspaceOwnerIn(ctx context.Context, db gen.DBTX, workspaceID pgtype.UUID) (pgtype.UUID, error) {
	if db == nil {
		return pgtype.UUID{}, errors.New("identity: workspace owner lookup has no database handle")
	}
	owner, err := gen.New(db).GetWorkspaceOwner(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, pgconv.UUIDString(workspaceID))
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	return owner, nil
}

func (s *Service) AccountState(ctx context.Context, userID pgtype.UUID) (present bool, purging bool, err error) {
	return s.AccountStateIn(ctx, s.Pool, userID)
}

func (s *Service) AccountStateIn(ctx context.Context, db gen.DBTX, userID pgtype.UUID) (present bool, purging bool, err error) {
	if db == nil {
		return false, false, errors.New("identity: account state lookup has no database handle")
	}
	row, err := gen.New(db).GetUserAccountState(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return row.Present, row.Purging, nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
