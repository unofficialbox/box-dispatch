package salesforceapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	DefaultProductionLogin = "https://login.salesforce.com"
	DefaultSandboxLogin    = "https://test.salesforce.com"
	DefaultOAuthScopes     = "api refresh_token id"
	// LoginClientID is Salesforce's public PlatformCLI connected app — the same
	// client the Salesforce CLI uses for browser login and scratch-org signup.
	LoginClientID    = "PlatformCLI"
	LoginCallbackURL = "http://localhost:1717/OauthRedirect"
	// Scratch signup reuses the public login client. ScratchOrgInfo issues an
	// authorization code that cannot carry a PKCE verifier from Dispatch.
	ScratchSignupClientID    = LoginClientID
	ScratchSignupCallbackURL = LoginCallbackURL
)

type AuthorizationRequest struct {
	LoginURL      string
	ClientID      string
	RedirectURL   string
	State         string
	CodeChallenge string
	Scopes        string
}

type AuthorizationCodeRequest struct {
	LoginURL     string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Code         string
	CodeVerifier string
}

type RefreshRequest struct {
	LoginURL     string
	ClientID     string
	ClientSecret string
	RefreshToken string
}

type TokenResponse struct {
	AccessToken  string
	RefreshToken string
	InstanceURL  string
	ID           string
}

func NormalizeLoginURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "", "production", "login.salesforce.com":
		return DefaultProductionLogin, nil
	case "sandbox", "test.salesforce.com":
		return DefaultSandboxLogin, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("Salesforce login URL is invalid")
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("Salesforce login URL must use HTTPS")
	}
	host := strings.ToLower(parsed.Host)
	if host != "login.salesforce.com" && host != "test.salesforce.com" && !strings.HasSuffix(host, ".salesforce.com") {
		return "", fmt.Errorf("Salesforce login URL is not a Salesforce host")
	}
	return "https://" + parsed.Host, nil
}

func AuthorizationURL(request AuthorizationRequest) (string, error) {
	loginURL, err := NormalizeLoginURL(request.LoginURL)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(request.ClientID) == "" {
		return "", fmt.Errorf("a Salesforce Connected App consumer key is required")
	}
	if strings.TrimSpace(request.RedirectURL) == "" || strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.CodeChallenge) == "" {
		return "", fmt.Errorf("Salesforce OAuth request is incomplete")
	}
	scopes := strings.TrimSpace(request.Scopes)
	if scopes == "" {
		scopes = DefaultOAuthScopes
	}
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {request.ClientID},
		"redirect_uri":          {request.RedirectURL},
		"state":                 {request.State},
		"scope":                 {scopes},
		"code_challenge":        {request.CodeChallenge},
		"code_challenge_method": {"S256"},
	}
	return loginURL + "/services/oauth2/authorize?" + values.Encode(), nil
}

func NewPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func (c *Client) ExchangeAuthorizationCode(ctx context.Context, request AuthorizationCodeRequest) (TokenResponse, error) {
	values := url.Values{
		"grant_type":   {"authorization_code"},
		"client_id":    {request.ClientID},
		"redirect_uri": {request.RedirectURL},
		"code":         {request.Code},
	}
	if strings.TrimSpace(request.CodeVerifier) != "" {
		values.Set("code_verifier", request.CodeVerifier)
	}
	if request.ClientSecret != "" {
		values.Set("client_secret", request.ClientSecret)
	}
	return c.postToken(ctx, request.LoginURL, values)
}

func (c *Client) RefreshAccessToken(ctx context.Context, request RefreshRequest) (TokenResponse, error) {
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {request.ClientID},
		"refresh_token": {request.RefreshToken},
	}
	if request.ClientSecret != "" {
		values.Set("client_secret", request.ClientSecret)
	}
	token, err := c.postToken(ctx, request.LoginURL, values)
	if err != nil {
		return TokenResponse{}, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = request.RefreshToken
	}
	return token, nil
}

func tokenBaseURL(loginURL string) (string, error) {
	if normalized, err := NormalizeLoginURL(loginURL); err == nil {
		return normalized, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(loginURL))
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("Salesforce login URL is invalid")
	}
	host, _, _ := strings.Cut(parsed.Host, ":")
	if parsed.Scheme == "http" && (host == "127.0.0.1" || host == "localhost") {
		return parsed.Scheme + "://" + parsed.Host, nil
	}
	return "", fmt.Errorf("Salesforce login URL must use HTTPS")
}

func (c *Client) postToken(ctx context.Context, loginURL string, values url.Values) (TokenResponse, error) {
	base, err := tokenBaseURL(loginURL)
	if err != nil {
		return TokenResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/services/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient().Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("Salesforce token request failed: %w", err)
	}
	defer response.Body.Close()
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		InstanceURL  string `json:"instance_url"`
		ID           string `json:"id"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if err := decodeResponse(response, &result); err != nil {
		return TokenResponse{}, fmt.Errorf("Salesforce token request failed: %w", err)
	}
	if result.AccessToken == "" || result.InstanceURL == "" {
		return TokenResponse{}, fmt.Errorf("Salesforce token request failed: %s", firstNonEmpty(result.Description, result.Error, "no access token returned"))
	}
	return TokenResponse{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, InstanceURL: strings.TrimRight(result.InstanceURL, "/"), ID: result.ID}, nil
}
