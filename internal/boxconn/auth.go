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
	connectionID string
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
	if settingsErr == nil {
		settings = settings.HydrateBoxConnections()
		if app, ok := settings.SelectedBoxConnection(); ok {
			if resolved, ok := authForBoxApp(app); ok {
				return resolved, nil
			}
		}
		if settings.HasBoxCCG() {
			return AuthConfig{
				Method:       AuthCCG,
				ClientID:     settings.BoxCCGClientID,
				ClientSecret: settings.BoxCCGClientSecret,
				SubjectType:  settings.BoxCCGSubjectType,
				SubjectID:    settings.BoxCCGSubjectID,
			}, nil
		}
	}

	clientID, clientSecret := BoxOAuthApp()
	refreshToken := strings.TrimSpace(os.Getenv("BOX_REFRESH_TOKEN"))
	if clientID != "" && clientSecret != "" && refreshToken != "" {
		return AuthConfig{Method: AuthOAuth2, ClientID: clientID, ClientSecret: clientSecret, RefreshToken: refreshToken}, nil
	}

	if settingsErr == nil && settings.BoxDefaultConnection != "" && settings.BoxDefaultConnection != DispatchCCGName && settings.BoxDefaultConnection != DispatchOAuthName {
		return AuthConfig{}, fmt.Errorf("Box CLI connection %q is no longer supported; log in with Box or export BOX_CLIENT_ID, BOX_CLIENT_SECRET, and BOX_REFRESH_TOKEN", settings.BoxDefaultConnection)
	}
	return AuthConfig{}, fmt.Errorf("no Box authentication configured: log in with Box, or export BOX_CLIENT_ID, BOX_CLIENT_SECRET, and BOX_REFRESH_TOKEN")
}

// ResolveAuthForConnection resolves one saved Box connection without changing
// the connection selected for deployment. It deliberately has no environment
// fallback: live comparisons must query the two exact saved accounts requested
// by the operator.
func ResolveAuthForConnection(connectionID string) (AuthConfig, error) {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return AuthConfig{}, fmt.Errorf("select a saved Box connection")
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return AuthConfig{}, fmt.Errorf("load Box connections: %w", err)
	}
	settings = settings.HydrateBoxConnections()
	for _, app := range settings.BoxConnections {
		if app.ID != connectionID {
			continue
		}
		if resolved, ok := authForBoxApp(app); ok {
			return resolved, nil
		}
		return AuthConfig{}, fmt.Errorf("Box connection %q needs to be reconnected", app.Alias)
	}
	return AuthConfig{}, fmt.Errorf("saved Box connection was not found")
}

func authForBoxApp(app config.BoxAppConnection) (AuthConfig, bool) {
	if strings.TrimSpace(app.RefreshToken) != "" {
		clientID, clientSecret := BoxOAuthApp()
		return AuthConfig{
			Method: AuthOAuth2, connectionID: app.ID, ClientID: firstNonEmpty(app.ClientID, clientID),
			ClientSecret: firstNonEmpty(app.ClientSecret, clientSecret), RefreshToken: app.RefreshToken,
		}, true
	}
	if boxAppHasCCG(app) {
		return AuthConfig{
			Method: AuthCCG, connectionID: app.ID, ClientID: app.ClientID, ClientSecret: app.ClientSecret,
			SubjectType: app.SubjectType, SubjectID: app.SubjectID,
		}, true
	}
	return AuthConfig{}, false
}

func persistRotatedBoxRefresh(original AuthConfig, refreshToken string) {
	if original.connectionID == "" {
		return
	}
	_, _ = shellstate.UpdateConnectionSettings(func(settings config.ConnectionSettings) (config.ConnectionSettings, error) {
		settings = settings.HydrateBoxConnections()
		for index, app := range settings.BoxConnections {
			if app.ID != original.connectionID {
				continue
			}
			clientID, _ := BoxOAuthApp()
			if firstNonEmpty(app.ClientID, clientID) != original.ClientID || app.RefreshToken != original.RefreshToken {
				return settings, fmt.Errorf("Box connection changed during token refresh")
			}
			app.RefreshToken = refreshToken
			settings.BoxConnections[index] = app
			return settings.HydrateBoxConnections(), nil
		}
		return settings, fmt.Errorf("Box connection is no longer available")
	})
}

// InvalidateOAuthConnection clears readiness only for the connection whose
// captured refresh failed. A later selection or successful rotation is retained.
func (original AuthConfig) InvalidateOAuthConnection() {
	if original.connectionID == "" {
		return
	}
	_, _ = shellstate.UpdateConnectionSettings(func(settings config.ConnectionSettings) (config.ConnectionSettings, error) {
		settings = settings.HydrateBoxConnections()
		for index, app := range settings.BoxConnections {
			if app.ID != original.connectionID {
				continue
			}
			clientID, _ := BoxOAuthApp()
			if firstNonEmpty(app.ClientID, clientID) != original.ClientID || app.RefreshToken != original.RefreshToken {
				return settings, nil
			}
			app.VerifiedAt = ""
			settings.BoxConnections[index] = app
			if settings.BoxSelectedConnectionID == app.ID {
				delete(settings.VerifiedConnections, "box")
			}
			return settings, nil
		}
		return settings, nil
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boxAppHasCCG(app config.BoxAppConnection) bool {
	return strings.TrimSpace(app.ClientID) != "" && strings.TrimSpace(app.ClientSecret) != "" &&
		strings.TrimSpace(app.SubjectType) != "" && strings.TrimSpace(app.SubjectID) != ""
}

// HasLegacyCLISelection reports an old persisted Box CLI environment. The
// checker uses this to run Connect and show the precise migration error rather
// than collapsing it into a generic "credentials missing" state.
func HasLegacyCLISelection() bool {
	settings, err := shellstate.LoadConnectionSettings()
	return err == nil && settings.BoxDefaultConnection != "" && settings.BoxDefaultConnection != DispatchCCGName && settings.BoxDefaultConnection != DispatchOAuthName
}

// AccessToken mints a short-lived access token from a supported configuration.
func (c AuthConfig) AccessToken(ctx context.Context) (string, error) {
	switch c.Method {
	case AuthCCG:
		return CCGToken(ctx, c.ClientID, c.ClientSecret, c.SubjectType, c.SubjectID)
	case AuthOAuth2:
		token, err := RefreshOAuthToken(ctx, c.ClientID, c.ClientSecret, c.RefreshToken)
		if err != nil {
			return "", err
		}
		if token.RefreshToken != "" && token.RefreshToken != c.RefreshToken {
			persistRotatedBoxRefresh(c, token.RefreshToken)
		}
		return token.AccessToken, nil
	default:
		return "", fmt.Errorf("unsupported Box authentication method %q", c.Method)
	}
}
