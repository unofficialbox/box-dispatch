package webapi

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

type salesforceOAuthExchange func(context.Context, salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error)
type salesforceTokenRefresh func(context.Context, salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error)

type salesforceOAuthStartInput struct {
	LoginHost    string `json:"loginHost"`
	Role         string `json:"role"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type salesforceOAuthStartResponse struct {
	ID           string `json:"id"`
	AuthorizeURL string `json:"authorizeUrl"`
	LoginHost    string `json:"loginHost"`
	Role         string `json:"role"`
}

type salesforceOAuthJob struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Alias        string `json:"alias,omitempty"`
	Username     string `json:"username,omitempty"`
	OrgID        string `json:"orgId,omitempty"`
	Role         string `json:"role,omitempty"`
	state        string
	verifier     string
	redirectURL  string
	loginURL     string
	clientID     string
	clientSecret string
	expires      time.Time
}

type salesforceOAuthManager struct {
	mu      sync.Mutex
	jobs    map[string]*salesforceOAuthJob
	byState map[string]string
	now     func() time.Time
}

func newSalesforceOAuthManager(now func() time.Time) *salesforceOAuthManager {
	if now == nil {
		now = time.Now
	}
	return &salesforceOAuthManager{jobs: map[string]*salesforceOAuthJob{}, byState: map[string]string{}, now: now}
}

func (manager *salesforceOAuthManager) start(input salesforceOAuthStartInput) (salesforceOAuthStartResponse, error) {
	loginURL, err := salesforceapi.NormalizeLoginURL(input.LoginHost)
	if err != nil {
		return salesforceOAuthStartResponse{}, err
	}
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role == "" {
		role = "org"
	}
	if role != "org" && role != "devhub" {
		return salesforceOAuthStartResponse{}, fmt.Errorf("Salesforce login role must be org or devhub")
	}
	verifier, challenge, err := salesforceapi.NewPKCE()
	if err != nil {
		return salesforceOAuthStartResponse{}, fmt.Errorf("could not start Salesforce login")
	}
	state := randomID()
	authorizeURL, err := salesforceapi.AuthorizationURL(salesforceapi.AuthorizationRequest{
		LoginURL: loginURL, ClientID: salesforceapi.LoginClientID, RedirectURL: salesforceapi.LoginCallbackURL,
		State: state, CodeChallenge: challenge,
	})
	if err != nil {
		return salesforceOAuthStartResponse{}, err
	}
	job := &salesforceOAuthJob{
		ID: randomID(), Status: "pending", Message: "Waiting for Salesforce login", Role: role,
		state: state, verifier: verifier, redirectURL: salesforceapi.LoginCallbackURL, loginURL: loginURL,
		clientID: salesforceapi.LoginClientID, expires: manager.now().Add(2 * time.Minute),
	}
	manager.mu.Lock()
	manager.jobs[job.ID] = job
	manager.byState[state] = job.ID
	manager.mu.Unlock()
	return salesforceOAuthStartResponse{ID: job.ID, AuthorizeURL: authorizeURL, LoginHost: loginURL, Role: role}, nil
}

func (manager *salesforceOAuthManager) get(id string) (salesforceOAuthJob, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job, ok := manager.jobs[id]
	if !ok {
		return salesforceOAuthJob{}, false
	}
	return job.public(), true
}

func (job salesforceOAuthJob) public() salesforceOAuthJob {
	return salesforceOAuthJob{ID: job.ID, Status: job.Status, Message: job.Message, Alias: job.Alias, Username: job.Username, OrgID: job.OrgID, Role: job.Role}
}

func (manager *salesforceOAuthManager) lookup(state string) (*salesforceOAuthJob, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	id, ok := manager.byState[strings.TrimSpace(state)]
	if !ok {
		return nil, fmt.Errorf("Salesforce login session was not found")
	}
	job := manager.jobs[id]
	if job == nil {
		return nil, fmt.Errorf("Salesforce login session was not found")
	}
	if manager.now().After(job.expires) {
		job.Status = "failed"
		job.Message = "Salesforce login timed out. Start login again from Dispatch."
		delete(manager.byState, job.state)
		return job, fmt.Errorf("%s", job.Message)
	}
	if job.Status != "pending" {
		return job, fmt.Errorf("Salesforce login session was already used")
	}
	return job, nil
}

func (manager *salesforceOAuthManager) finish(job *salesforceOAuthJob, status, message, alias, username, orgID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job.Status = status
	job.Message = message
	job.Alias = alias
	job.Username = username
	job.OrgID = orgID
	delete(manager.byState, job.state)
}

func applySalesforceOAuthToken(settings config.ConnectionSettings, role, clientID, clientSecret string, token salesforceapi.TokenResponse, username, orgID string) config.ConnectionSettings {
	alias := firstNonEmpty(username, restConnectionAlias())
	if role == "devhub" {
		alias = firstNonEmpty(username, "Connected Salesforce Dev Hub")
	}
	storedClientID := firstNonEmpty(clientID, salesforceapi.LoginClientID)
	selectOrg := role != "devhub" || strings.TrimSpace(settings.SalesforceSelectedOrgID) == ""
	return settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: alias, Username: username, OrgID: orgID, OrgType: "persistent",
		InstanceURL: token.InstanceURL, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken,
		ClientID: storedClientID, ClientSecret: clientSecret, Status: "Ready", DevHub: role == "devhub",
	}, selectOrg)
}

func explainSalesforceOAuthError(code, description string) string {
	combined := strings.ToLower(strings.TrimSpace(code) + " " + strings.TrimSpace(description))
	if strings.Contains(combined, "cross-org") {
		return "Salesforce rejected this login for that org. Start login again from Dispatch."
	}
	return firstNonEmpty(strings.TrimSpace(description), strings.TrimSpace(code), "Salesforce login did not finish")
}

func (manager *salesforceOAuthManager) failState(state, message string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	id, ok := manager.byState[strings.TrimSpace(state)]
	if !ok {
		return
	}
	job := manager.jobs[id]
	if job == nil {
		return
	}
	job.Status = "failed"
	job.Message = message
	delete(manager.byState, job.state)
}

const oauthCallbackPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  :root { color: #172845; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  * { box-sizing: border-box; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #f8fbff; }
  main { width: min(420px, calc(100vw - 32px)); padding: 32px 28px; border: 1px solid #d7e2ee; border-radius: 16px; background: #fff; box-shadow: 0 8px 24px rgba(20, 46, 82, .06); }
  .brand { display: flex; align-items: center; gap: 12px; margin: 0 0 22px; color: #10264e; font-size: 18px; font-weight: 650; letter-spacing: -.4px; }
  .brand-icon { display: grid; width: 40px; height: 40px; place-items: center; border-radius: 10px; background: #0061d3; color: #fff; font-size: 18px; font-weight: 800; letter-spacing: -2px; line-height: 1; }
  h1 { margin: 0 0 8px; font-size: 22px; letter-spacing: -.4px; }
  p { margin: 0; color: #536a89; line-height: 1.5; }
  p + p { margin-top: 10px; }
  .ok h1 { color: #14854f; }
  .failed h1 { color: #c33a2c; }
</style>
</head>
<body>
<main class="%s">
  <p class="brand"><span class="brand-icon" aria-hidden="true">B/</span>Box Dispatch</p>
  <h1>%s</h1>
  <p>%s</p>
  <p>You can close this tab and return to Dispatch.</p>
</main>
</body>
</html>`

func writeOAuthCallbackPage(w http.ResponseWriter, title, message string, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	status := title + " did not finish"
	tone := "failed"
	if ok {
		status = strings.TrimSuffix(title, " login") + " connected"
		tone = "ok"
	}
	_, _ = fmt.Fprintf(w, oauthCallbackPage, html.EscapeString(title), tone, html.EscapeString(status), html.EscapeString(message))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
