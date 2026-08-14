package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// SessionTTL is the fixed absolute session lifetime (ADR-020: no sliding renewal).
const SessionTTL = 30 * 24 * time.Hour

const providerGitHub = "github"

// Service wires OAuth identity to users, workspaces, and sessions.
type Service struct {
	Pool  *pgxpool.Pool
	OAuth *GitHubOAuth
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

// LoginOrSignup resolves the GitHub identity to a user — creating user,
// personal workspace, and identity in one transaction on first login
// (ADR-011 1:1 workspace, ADR-020) — and mints a session. It returns the raw
// session token; only its SHA-256 is stored.
func (s *Service) LoginOrSignup(ctx context.Context, gu GitHubUser) (string, error) {
	providerUserID := strconv.FormatInt(gu.ID, 10)

	user, err := s.queries().GetUserByIdentity(ctx, gen.GetUserByIdentityParams{
		Provider:       providerGitHub,
		ProviderUserID: providerUserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		user, err = s.signup(ctx, gu, providerUserID)
	}
	if err != nil {
		return "", err
	}
	return s.mintSession(ctx, user)
}

func (s *Service) signup(ctx context.Context, gu GitHubUser, providerUserID string) (gen.User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.User{}, err
	}
	defer tx.Rollback(ctx)

	q := s.queries().WithTx(tx)
	user, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:       gu.Email,
		DisplayName: gu.Name,
	})
	if err != nil {
		return gen.User{}, err
	}
	if _, err := q.CreateWorkspace(ctx, gen.CreateWorkspaceParams{
		OwnerUserID: user.ID,
		Name:        gu.Login,
	}); err != nil {
		return gen.User{}, err
	}
	if err := q.CreateIdentity(ctx, gen.CreateIdentityParams{
		UserID:         user.ID,
		Provider:       providerGitHub,
		ProviderUserID: providerUserID,
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
	hash := hashToken(token)
	if _, err := s.queries().CreateSession(ctx, gen.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: pgTime(time.Now().Add(SessionTTL)),
	}); err != nil {
		return "", err
	}
	return token, nil
}

// UserForToken resolves a cookie token to its user; expiry is checked in SQL.
func (s *Service) UserForToken(ctx context.Context, token string) (gen.User, error) {
	return s.queries().GetSessionUser(ctx, hashToken(token))
}

// Logout revokes the session; deleting a missing row is a no-op (idempotent).
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.queries().DeleteSession(ctx, hashToken(token))
}

// PersonalWorkspace returns the user's single personal workspace (ADR-011).
func (s *Service) PersonalWorkspace(ctx context.Context, user gen.User) (gen.Workspace, error) {
	ws, err := s.queries().ListWorkspacesByOwner(ctx, user.ID)
	if err != nil {
		return gen.Workspace{}, err
	}
	if len(ws) == 0 {
		return gen.Workspace{}, errors.New("user has no workspace")
	}
	return ws[0], nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
