package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

type connectionSaver func(config.ConnectionSettings) error
type salesforceTargetStore func() ([]salesforceorg.Target, error)
type boxConnectionCheck func(context.Context, config.ConnectionSettings) (config.VerifiedConnection, error)

var boxCurrentUserURL = "https://api.box.com/2.0/users/me?fields=id,login,name,enterprise"

type salesforceConnectionOption struct {
	ID        string `json:"id,omitempty"`
	Alias     string `json:"alias"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Selected  bool   `json:"selected"`
	DevHub    bool   `json:"devHub,omitempty"`
	Username  string `json:"username,omitempty"`
	OrgID     string `json:"orgId,omitempty"`
}

type salesforceConnectionSelection struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

// boxConnectionInput mirrors the supported Dispatch CCG setup without ever
// returning secret material to the browser.
type boxConnectionInput struct {
	Alias        string `json:"alias"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	SubjectType  string `json:"subjectType"`
	SubjectID    string `json:"subjectId"`
}

func (b boxConnectionInput) normalized() boxConnectionInput {
	b.Alias = strings.TrimSpace(b.Alias)
	b.ClientID = strings.TrimSpace(b.ClientID)
	b.ClientSecret = strings.TrimSpace(b.ClientSecret)
	b.SubjectType = strings.ToLower(strings.TrimSpace(b.SubjectType))
	b.SubjectID = strings.TrimSpace(b.SubjectID)
	if b.SubjectType == "" {
		b.SubjectType = "user"
	}
	if b.Alias == "" {
		b.Alias = "Box CCG"
	}
	return b
}

func (b boxConnectionInput) validate() error {
	if b.ClientID == "" || b.ClientSecret == "" || b.SubjectID == "" {
		return fmt.Errorf("client ID, client secret, and subject ID are required")
	}
	if b.SubjectType != "user" && b.SubjectType != "enterprise" {
		return fmt.Errorf("subject type must be user or enterprise")
	}
	return nil
}

func saveBoxCCGSelection(settings config.ConnectionSettings, input boxConnectionInput) config.ConnectionSettings {
	settings = settings.UpsertBoxConnection(config.BoxAppConnection{
		Alias: input.Alias, ClientID: input.ClientID, ClientSecret: input.ClientSecret,
		SubjectType: input.SubjectType, SubjectID: input.SubjectID,
	}, true)
	settings.BoxDefaultConnection = boxconn.DispatchCCGName
	if settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "box")
	}
	return settings
}

type boxConnectionOption struct {
	ID            string `json:"id,omitempty"`
	Alias         string `json:"alias"`
	Status        string `json:"status"`
	Selected      bool   `json:"selected"`
	Identity      string `json:"identity,omitempty"`
	SubjectType   string `json:"subjectType,omitempty"`
	ClientIDHint  string `json:"clientIdHint,omitempty"`
	SubjectIDHint string `json:"subjectIdHint,omitempty"`
}

type boxConnectionSelection struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

func presentBoxConnectionOptions(settings config.ConnectionSettings) []boxConnectionOption {
	settings = settings.HydrateBoxConnections()
	options := make([]boxConnectionOption, 0, len(settings.BoxConnections))
	for _, app := range settings.BoxConnections {
		options = append(options, boxConnectionOption{
			ID: app.ID, Alias: app.Alias, Status: connectionReadiness(app.VerifiedAt != ""),
			Selected: app.ID == settings.BoxSelectedConnectionID, Identity: app.Identity,
			SubjectType: app.SubjectType, ClientIDHint: identifierHint(app.ClientID),
			SubjectIDHint: identifierHint(app.SubjectID),
		})
	}
	return options
}

func connectionReadiness(ready bool) string {
	if ready {
		return "Ready"
	}
	return "Not ready"
}

func presentSalesforceOrgStatus(status string, selected, verified bool) string {
	if selected {
		return connectionReadiness(verified)
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "ready":
		return "Ready"
	default:
		return "Not ready"
	}
}

func verifyBoxConnection(ctx context.Context, settings config.ConnectionSettings) (config.VerifiedConnection, error) {
	settings = settings.HydrateBoxConnections()
	token, authType, refreshToken, err := boxAccessToken(ctx, settings)
	if err != nil {
		return config.VerifiedConnection{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, boxCurrentUserURL, nil)
	if err != nil {
		return config.VerifiedConnection{}, fmt.Errorf("prepare Box identity check: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return config.VerifiedConnection{}, fmt.Errorf("Box could not be reached: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return config.VerifiedConnection{}, fmt.Errorf("Box identity check returned %s", response.Status)
	}
	var user struct {
		ID         string `json:"id"`
		Login      string `json:"login"`
		Enterprise struct {
			ID string `json:"id"`
		} `json:"enterprise"`
	}
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return config.VerifiedConnection{}, fmt.Errorf("read Box identity: %w", err)
	}
	if strings.TrimSpace(user.ID) == "" {
		return config.VerifiedConnection{}, fmt.Errorf("Box identity check returned no acting user")
	}
	selection := firstNonEmpty(settings.BoxCCGAlias, user.Login, boxconn.DispatchOAuthName)
	return config.VerifiedConnection{
		Selection:    selection,
		Identity:     user.Login,
		Account:      user.ID,
		Enterprise:   user.Enterprise.ID,
		AuthType:     authType,
		RefreshToken: refreshToken,
	}, nil
}

func boxAccessToken(ctx context.Context, settings config.ConnectionSettings) (string, string, string, error) {
	if app, ok := settings.SelectedBoxConnection(); ok && app.RefreshToken != "" {
		clientID, clientSecret := boxconn.BoxOAuthApp()
		token, err := boxconn.RefreshOAuthToken(ctx, firstNonEmpty(app.ClientID, clientID), firstNonEmpty(app.ClientSecret, clientSecret), app.RefreshToken)
		if err != nil {
			return "", "", "", fmt.Errorf("Box rejected the OAuth2 login: %w", err)
		}
		return token.AccessToken, string(boxconn.AuthOAuth2), token.RefreshToken, nil
	}
	if settings.HasBoxCCG() {
		token, err := boxconn.CCGTokenFromSettings(ctx, settings)
		if err != nil {
			return "", "", "", fmt.Errorf("Box rejected the Client Credentials Grant settings: %w", err)
		}
		return token, string(boxconn.AuthCCG), "", nil
	}
	clientID, clientSecret := boxconn.BoxOAuthApp()
	refreshToken := strings.TrimSpace(os.Getenv("BOX_REFRESH_TOKEN"))
	if clientID != "" && clientSecret != "" && refreshToken != "" {
		token, err := boxconn.RefreshOAuthToken(ctx, clientID, clientSecret, refreshToken)
		if err != nil {
			return "", "", "", fmt.Errorf("Box rejected the OAuth2 login: %w", err)
		}
		return token.AccessToken, string(boxconn.AuthOAuth2), token.RefreshToken, nil
	}
	return "", "", "", fmt.Errorf("connect Box with OAuth before checking availability")
}

func loadPersistedConnections() (config.ConnectionSettings, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return settings, err
	}
	// Environment variables provide a server-side bootstrap path for CI and
	// headless use. Persisted browser settings take precedence.
	if settings.SalesforceInstanceURL == "" {
		settings.SalesforceInstanceURL = strings.TrimSpace(os.Getenv("SALESFORCE_INSTANCE_URL"))
	}
	if settings.SalesforceAccessToken == "" {
		settings.SalesforceAccessToken = strings.TrimSpace(os.Getenv("SALESFORCE_ACCESS_TOKEN"))
	}
	if settings.SalesforceRefreshToken == "" {
		settings.SalesforceRefreshToken = strings.TrimSpace(os.Getenv("SALESFORCE_REFRESH_TOKEN"))
	}
	if settings.SalesforceDevHubURL == "" {
		settings.SalesforceDevHubURL = strings.TrimSpace(os.Getenv("SALESFORCE_DEV_HUB_URL"))
	}
	if settings.SalesforceDevHubToken == "" {
		settings.SalesforceDevHubToken = strings.TrimSpace(os.Getenv("SALESFORCE_DEV_HUB_ACCESS_TOKEN"))
	}
	if settings.SalesforceClientID == "" {
		settings.SalesforceClientID = strings.TrimSpace(os.Getenv("SALESFORCE_CLIENT_ID"))
	}
	if settings.SalesforceClientSecret == "" {
		settings.SalesforceClientSecret = strings.TrimSpace(os.Getenv("SALESFORCE_CLIENT_SECRET"))
	}
	return settings, nil
}

func savePersistedConnections(settings config.ConnectionSettings) error {
	return shellstate.SaveConnectionSettings(settings)
}

func listSalesforceTargets() ([]salesforceorg.Target, error) {
	return salesforceorg.ListTargets()
}

func recoverSalesforceScratchAccess(ctx context.Context, orgID, username, instanceURL string) (salesforceorg.ScratchAccess, error) {
	return salesforceorg.RecoverScratchAccess(ctx, orgID, username, instanceURL)
}

func openSalesforceScratch(ctx context.Context, target, returnPath string) (string, error) {
	return salesforceorg.OpenScratchURL(ctx, target, returnPath)
}

func presentSalesforceOptions(settings config.ConnectionSettings, targets []salesforceorg.Target) []salesforceConnectionOption {
	options := make([]salesforceConnectionOption, 0, len(targets))
	for _, target := range targets {
		if !target.Healthy(time.Now()) {
			continue
		}
		kind := "Org"
		if target.IsScratch() {
			kind = "Scratch org"
		}
		options = append(options, salesforceConnectionOption{
			Alias: target.Alias, Kind: kind, Status: target.Status,
			ExpiresAt: target.ExpirationDate, Selected: strings.EqualFold(target.Alias, settings.SalesforceAlias),
		})
	}
	return options
}

func presentSalesforceOrgOptions(settings config.ConnectionSettings) []salesforceConnectionOption {
	options := make([]salesforceConnectionOption, 0, len(settings.SalesforceOrgs))
	for _, org := range settings.SalesforceOrgs {
		kind := "Org"
		switch {
		case org.DevHub:
			kind = "Dev Hub"
		case strings.EqualFold(org.OrgType, "scratch"):
			kind = "Scratch org"
		}
		selected := org.ID == settings.SalesforceSelectedOrgID
		verified := selected && settings.VerifiedConnections["salesforce"].VerifiedAt != ""
		options = append(options, salesforceConnectionOption{
			ID: org.ID, Alias: org.Alias, Kind: kind, Status: presentSalesforceOrgStatus(org.Status, selected, verified),
			ExpiresAt: org.ExpirationDate, Selected: selected, DevHub: org.DevHub, Username: org.Username, OrgID: org.OrgID,
		})
	}
	return options
}

func presentSalesforceRESTOption(settings config.ConnectionSettings) []salesforceConnectionOption {
	if !settings.HasSalesforceREST() {
		return []salesforceConnectionOption{}
	}
	alias := strings.TrimSpace(settings.SalesforceAlias)
	if alias == "" {
		alias = restConnectionAlias()
	}
	kind := "Org"
	if strings.EqualFold(settings.SalesforceOrgType, "scratch") {
		kind = "Scratch org"
	}
	return []salesforceConnectionOption{{Alias: alias, Kind: kind, Status: settings.SalesforceOrgStatus, ExpiresAt: settings.SalesforceExpirationDate, Selected: true}}
}

func saveSalesforceSelection(settings config.ConnectionSettings, target salesforceorg.Target) config.ConnectionSettings {
	settings.SalesforceAlias = target.Alias
	settings.SalesforceOrgID = ""
	settings.SalesforceOrgStatus = target.Status
	settings.SalesforceExpirationDate = target.ExpirationDate
	settings.SalesforceOrgType = "persistent"
	if target.IsScratch() {
		settings.SalesforceOrgType = "scratch"
	}
	if settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "salesforce")
	}
	return settings
}

func loadPlan() (config.SolutionPlan, error) {
	return shellstate.LoadSolutionPlan()
}

func savePlan(plan config.SolutionPlan) error {
	return shellstate.SaveSolutionPlan(plan)
}

func loadDeploymentDefaults() (config.DeploymentDefaults, error) {
	return shellstate.LoadDeploymentDefaults()
}

func saveDeploymentDefaults(defaults config.DeploymentDefaults) error {
	return shellstate.SaveDeploymentDefaults(defaults)
}
