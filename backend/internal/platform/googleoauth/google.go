// Package googleoauth signs people in with Google using OpenID Connect's
// authorization code flow with PKCE, on net/http alone.
//
// The identity comes from Google's userinfo endpoint, called with the access
// token this server just received from Google's token endpoint over TLS. Both
// calls go straight from this server to Google, so no ID token signature has
// to be verified and no JWT library is needed.
package googleoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"receh/internal/domain"
)

// Google's endpoints, from https://accounts.google.com/.well-known/openid-configuration.
const (
	DefaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

type Provider struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// Endpoints; empty means Google's. Tests point them at a fake server.
	AuthURLBase string
	TokenURL    string
	UserinfoURL string
	HTTP        *http.Client
}

func New(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		AuthURLBase: DefaultAuthURL, TokenURL: DefaultTokenURL, UserinfoURL: DefaultUserinfoURL,
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthURL is where the browser goes to pick a Google account.
func (p *Provider) AuthURL(state, codeChallenge string) string {
	q := url.Values{
		"client_id":             {p.ClientID},
		"redirect_uri":          {p.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	return p.AuthURLBase + "?" + q.Encode()
}

// Exchange trades the authorization code for the person's identity. Errors
// describe the failure for the server log; callers show users a generic message.
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier string) (*domain.OAuthIdentity, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.RedirectURL},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := p.doJSON(req, &tok); err != nil {
		return nil, fmt.Errorf("token endpoint: %w", err)
	}
	if tok.Error != "" || tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint: %s %s", tok.Error, tok.Description)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, p.UserinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := p.doJSON(req, &info); err != nil {
		return nil, fmt.Errorf("userinfo endpoint: %w", err)
	}
	if info.Sub == "" || info.Email == "" {
		return nil, errors.New("userinfo endpoint: missing sub or email")
	}
	return &domain.OAuthIdentity{Subject: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified, Name: info.Name}, nil
}

func (p *Provider) doJSON(req *http.Request, dst any) error {
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	// A token error is JSON with status 400; decode it so the log says why.
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("status %d: undecodable body", resp.StatusCode)
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
