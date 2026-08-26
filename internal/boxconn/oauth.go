package boxconn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	AuthorizeURL      = "https://account.box.com/api/oauth2/authorize"
	LoginCallbackURL  = "http://localhost:4400/oauth/callback"
	DispatchOAuthName = "box-dispatch-oauth"
)

type AuthorizationRequest struct {
	ClientID      string
	RedirectURL   string
	State         string
	CodeChallenge string
}

type AuthorizationCodeRequest struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Code         string
	CodeVerifier string
}

func BoxOAuthApp() (clientID, clientSecret string) {
	return strings.TrimSpace(os.Getenv("BOX_CLIENT_ID")), strings.TrimSpace(os.Getenv("BOX_CLIENT_SECRET"))
}

func HasBoxOAuthApp() bool {
	clientID, clientSecret := BoxOAuthApp()
	return clientID != "" && clientSecret != ""
}

func AuthorizationURL(request AuthorizationRequest) (string, error) {
	if strings.TrimSpace(request.ClientID) == "" {
		return "", fmt.Errorf("set BOX_CLIENT_ID and BOX_CLIENT_SECRET in .env before logging in")
	}
	if strings.TrimSpace(request.RedirectURL) == "" || strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.CodeChallenge) == "" {
		return "", fmt.Errorf("Box OAuth request is incomplete")
	}
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {request.ClientID},
		"redirect_uri":          {request.RedirectURL},
		"state":                 {request.State},
		"code_challenge":        {request.CodeChallenge},
		"code_challenge_method": {"S256"},
	}
	return AuthorizeURL + "?" + values.Encode(), nil
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

func ExchangeAuthorizationCode(ctx context.Context, request AuthorizationCodeRequest) (TokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {request.ClientID},
		"client_secret": {request.ClientSecret},
		"redirect_uri":  {request.RedirectURL},
		"code":          {request.Code},
	}
	if request.CodeVerifier != "" {
		form.Set("code_verifier", request.CodeVerifier)
	}
	return tokenResponseFromForm(ctx, form, "OAuth2")
}
