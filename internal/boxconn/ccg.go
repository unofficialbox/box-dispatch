package boxconn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

// TokenURL is the Box OAuth2 token endpoint. It is a var so tests can point it
// at a local server.
var TokenURL = "https://api.box.com/oauth2/token"

// CCGToken mints a Client Credentials Grant access token for the given subject.
// subjectType "user" makes the token act as that user, so objects it creates are
// owned by the user; "enterprise" acts as the service account. Both carry the CCG
// app's enterprise scopes (e.g. Doc Gen) the CLI's OAuth token can lack.
func CCGToken(ctx context.Context, clientID, clientSecret, subjectType, subjectID string) (string, error) {
	form := url.Values{
		"grant_type":       {"client_credentials"},
		"client_id":        {clientID},
		"client_secret":    {clientSecret},
		"box_subject_type": {subjectType},
		"box_subject_id":   {subjectID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request Box CCG token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Box CCG token request returned %s: %s", resp.Status, tokenError(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse Box CCG token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("Box CCG token response contained no access token")
	}
	return payload.AccessToken, nil
}

// CCGTokenFromSettings mints a token for the CCG app captured in settings.
func CCGTokenFromSettings(ctx context.Context, settings config.ConnectionSettings) (string, error) {
	return CCGToken(ctx, settings.BoxCCGClientID, settings.BoxCCGClientSecret, settings.BoxCCGSubjectType, settings.BoxCCGSubjectID)
}

// tokenError extracts the human-readable message from a Box OAuth error body.
func tokenError(body []byte) string {
	var payload struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != "" {
		if payload.Description != "" {
			return payload.Error + ": " + payload.Description
		}
		return payload.Error
	}
	return strings.TrimSpace(string(body))
}
