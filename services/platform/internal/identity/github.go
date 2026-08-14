// Package identity owns login, sessions, and resolving the authenticated user
// (CORE-005, ADR-020). Authorization stays workspace-scoped in SQL (ADR-011);
// this package only answers "who is calling".
package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GitHubOAuth drives the authorization-code flow with plain net/http.
// ponytail: stdlib only; x/oauth2 buys nothing for a single fixed provider.
type GitHubOAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// AuthBase and APIBase exist so tests can point at a stub server.
	AuthBase string // default https://github.com/login/oauth
	APIBase  string // default https://api.github.com
	Client   *http.Client
}

func (g *GitHubOAuth) authBase() string {
	if g.AuthBase != "" {
		return g.AuthBase
	}
	return "https://github.com/login/oauth"
}

func (g *GitHubOAuth) apiBase() string {
	if g.APIBase != "" {
		return g.APIBase
	}
	return "https://api.github.com"
}

func (g *GitHubOAuth) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return http.DefaultClient
}

// AuthURL returns the GitHub authorization URL for the given one-shot state.
// Scope user:email is needed to read the primary verified email (ADR-020).
func (g *GitHubOAuth) AuthURL(state string) string {
	q := url.Values{
		"client_id":    {g.ClientID},
		"redirect_uri": {g.RedirectURL},
		"scope":        {"user:email"},
		"state":        {state},
	}
	return g.authBase() + "/authorize?" + q.Encode()
}

// Exchange trades the callback code for an access token.
func (g *GitHubOAuth) Exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"code":          {code},
		"redirect_uri":  {g.RedirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.authBase()+"/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := g.doJSON(req, &body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("github token exchange failed: %s", body.Error)
	}
	return body.AccessToken, nil
}

// GitHubUser is the subset of the GitHub user we persist. ID is the identity
// key; email and name are display data only (ADR-020).
type GitHubUser struct {
	ID    int64
	Login string
	Name  string
	Email string
}

// FetchUser loads the authenticated user, falling back to /user/emails when the
// profile email is private.
func (g *GitHubOAuth) FetchUser(ctx context.Context, token string) (GitHubUser, error) {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := g.getAPI(ctx, token, "/user", &u); err != nil {
		return GitHubUser{}, err
	}
	if u.Email == "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := g.getAPI(ctx, token, "/user/emails", &emails); err != nil {
			return GitHubUser{}, err
		}
		for _, e := range emails {
			if e.Primary && e.Verified {
				u.Email = e.Email
				break
			}
		}
	}
	if u.Email == "" {
		return GitHubUser{}, fmt.Errorf("github account has no primary verified email")
	}
	if u.Name == "" {
		u.Name = u.Login
	}
	return GitHubUser{ID: u.ID, Login: u.Login, Name: u.Name, Email: u.Email}, nil
}

func (g *GitHubOAuth) getAPI(ctx context.Context, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	return g.doJSON(req, out)
}

func (g *GitHubOAuth) doJSON(req *http.Request, out any) error {
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("github %s: status %d: %s", req.URL.Path, resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
