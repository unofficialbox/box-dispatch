// Package boxconn resolves the non-secret choice of Box authentication used by
// both connection checks and deployment. The supported paths are a captured
// CCG app or OAuth 2 refresh-token credentials from the environment.
package boxconn

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

const DispatchCCGName = "box-dispatch-ccg"

type AuthMethod string

const (
	AuthCCG    AuthMethod = "CCG"
	AuthOAuth2 AuthMethod = "OAuth2"
)

// AuthConfig contains the credentials needed to mint a Box access token. It
// is intentionally process-local; callers must never persist or render it.
type AuthConfig struct {
	Method       AuthMethod
	ClientID     string
	ClientSecret string
	RefreshToken string
	SubjectType  string
	SubjectID    string
}

// Selection is safe to persist with a verified-connection snapshot. It lets
// Dispatch recheck only when the selected CCG app or OAuth client changes.
func (c AuthConfig) Selection() string {
	if c.Method == AuthCCG {
		return DispatchCCGName
	}
	return "oauth2:" + c.ClientID
}

// ResolveAuth is the single authentication decision for both Connect and
// lifecycle work. A legacy Box CLI selection is rejected explicitly rather
// than letting Connect and Deploy use different credentials.
func ResolveAuth() (AuthConfig, error) {
	settings, settingsErr := shellstate.LoadConnectionSettings()
	if settingsErr == nil && settings.HasBoxCCG() && prefersCCG(settings) {
		return AuthConfig{
			Method:       AuthCCG,
			ClientID:     settings.BoxCCGClientID,
			ClientSecret: settings.BoxCCGClientSecret,
			SubjectType:  settings.BoxCCGSubjectType,
			SubjectID:    settings.BoxCCGSubjectID,
		}, nil
	}

	clientID := strings.TrimSpace(os.Getenv("BOX_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("BOX_CLIENT_SECRET"))
	refreshToken := strings.TrimSpace(os.Getenv("BOX_REFRESH_TOKEN"))
	if clientID != "" && clientSecret != "" && refreshToken != "" {
		return AuthConfig{Method: AuthOAuth2, ClientID: clientID, ClientSecret: clientSecret, RefreshToken: refreshToken}, nil
	}

	if settingsErr == nil && settings.BoxDefaultConnection != "" && settings.BoxDefaultConnection != DispatchCCGName {
		return AuthConfig{}, fmt.Errorf("Box CLI connection %q is no longer supported; configure BOX_CLIENT_ID, BOX_CLIENT_SECRET, and BOX_REFRESH_TOKEN, or select a Box CCG app", settings.BoxDefaultConnection)
	}
	if settingsErr == nil && settings.HasBoxCCG() {
		return AuthConfig{}, fmt.Errorf("the saved Box CCG app is not selected; select it again or configure OAuth2 refresh-token credentials")
	}
	return AuthConfig{}, fmt.Errorf("no Box authentication configured: add a CCG app in Dispatch or export BOX_CLIENT_ID, BOX_CLIENT_SECRET, and BOX_REFRESH_TOKEN")
}

func prefersCCG(settings config.ConnectionSettings) bool {
	return settings.BoxDefaultConnection == "" || settings.BoxDefaultConnection == DispatchCCGName
}

// HasLegacyCLISelection reports an old persisted Box CLI environment. The
// checker uses this to run Connect and show the precise migration error rather
// than collapsing it into a generic "credentials missing" state.
func HasLegacyCLISelection() bool {
	settings, err := shellstate.LoadConnectionSettings()
	return err == nil && settings.BoxDefaultConnection != "" && settings.BoxDefaultConnection != DispatchCCGName
}

// AccessToken mints a short-lived access token from a supported configuration.
func (c AuthConfig) AccessToken(ctx context.Context) (string, error) {
	switch c.Method {
	case AuthCCG:
		return CCGToken(ctx, c.ClientID, c.ClientSecret, c.SubjectType, c.SubjectID)
	case AuthOAuth2:
		return OAuthToken(ctx, c.ClientID, c.ClientSecret, c.RefreshToken)
	default:
		return "", fmt.Errorf("unsupported Box authentication method %q", c.Method)
	}
}
