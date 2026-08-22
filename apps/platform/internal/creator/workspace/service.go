package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// SessionTTL is the fixed absolute session lifetime (ADR-020: no sliding renewal).
const SessionTTL = 30 * 24 * time.Hour

const providerGitHub = "github"

// Service wires OAuth identity to users, workspaces, and sessions.
type Service struct {
	Pool  *pgxpool.Pool
	OAuth *GitHubOAuth

	// The account purge's cross-context steps, all of them required. Each one
	// belongs to the context that owns the rows it clears, and arrives here from
	// a composition root rather than an import because every context imports this
	// one for its workspace scope (iron rule 3), so this one can import none of
	// them (ADR-034). purge.go holds the order and the refusal.
	PurgeAnalytics     WorkspacePurge
	PurgeTestData      WorkspacePurge
	PurgeRunArtifacts  WorkspacePurge
	PurgeDownloads     WorkspacePurge
	PurgeSkills        WorkspacePurge
	PurgeImportSources WorkspacePurge

	// Object-key readers run before the transaction and are split by owner just
	// like the row purges. All three are required; see purge.go.
	DatasetObjectKeys          WorkspaceObjectKeys
	RunArtifactObjectKeys      WorkspaceObjectKeys
	DownloadArtifactObjectKeys WorkspaceObjectKeys
}

// User and Workspace are Identity's published language. Persistence rows stay
// inside this context so schema changes do not become cross-context contracts.
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

// ExternalIdentity is an identity a provider has already verified (GitHub
// OAuth, the dev provider, later LDAP). LoginOrSignup trusts it; verifying
// credentials against the provider is the caller's job.
type ExternalIdentity struct {
	Provider       string
	ProviderUserID string
	Email          string
	Name           string
	Login          string // workspace name on first login
}

// LoginOrSignup resolves an external identity to a user — creating user,
// personal workspace, and identity in one transaction on first login
// (ADR-011 1:1 workspace, ADR-020) — and mints a session. It returns the raw
// session token; only its SHA-256 is stored.
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
	return s.mintSession(ctx, user)
}

func (s *Service) signup(ctx context.Context, id ExternalIdentity) (gen.User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries().WithTx(tx)
	// The optimistic lookup in LoginOrSignup keeps established logins cheap.
	// Serialize only first-login contenders for the same provider identity, then
	// re-check under the lock so two OAuth callbacks cannot create an orphan user
	// and workspace before one loses the identity primary-key race.
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
		return "", err
	}
	// CORE-008: the session row and its audit event commit together, so a
	// session that exists always has a login on the trail.
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

// UserForToken resolves a cookie token to its user; expiry is checked in SQL.
func (s *Service) UserForToken(ctx context.Context, token string) (User, error) {
	row, err := s.queries().GetSessionUser(ctx, hashToken(token))
	return userDTO(row), err
}

// Logout revokes the session; deleting a missing row is a no-op (idempotent).
// The audit event commits with the revocation (CORE-008). An unknown or already
// expired token leaves no trail: there is no session to revoke and no verified
// actor to attribute one to.
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

// AccountDeletionGrace is how long a deletion request stays cancellable before
// the purge runs and becomes irreversible (PDM-006 6.1).
const AccountDeletionGrace = 30 * 24 * time.Hour

// RequestAccountDeletion starts the grace period (CORE-007). The account stays
// fully usable meanwhile — that is the point of a cancellable window — so this
// deliberately does not touch users.deleted_at, which is what every read and
// the login path treat as "gone" (see 0013).
//
// Idempotent: asking twice keeps the original start time.
func (s *Service) RequestAccountDeletion(ctx context.Context, user User) (User, error) {
	return s.setDeletionRequest(ctx, user, true)
}

// CancelAccountDeletion withdraws the request at any point in the grace period.
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
		return User{}, errors.New("account not found")
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

// CleanupExpiredSessions batch-deletes sessions past their absolute expiry.
// Idempotent: an empty result on repeated calls is expected, not an error
// (ADR-020, matches the ADR-008 cleanup convention). The caller is
// responsible for scheduling; no cron/queue is wired here.
func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.queries().DeleteExpiredSessions(ctx)
}

// PersonalWorkspace returns the user's single personal workspace (ADR-011).
func (s *Service) PersonalWorkspace(ctx context.Context, user User) (Workspace, error) {
	ws, err := s.queries().ListWorkspacesByOwner(ctx, user.ID)
	if err != nil {
		return Workspace{}, err
	}
	if len(ws) == 0 {
		return Workspace{}, errors.New("user has no workspace")
	}
	return workspaceDTO(ws[0]), nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
