package webapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

type boxOAuthExchange func(context.Context, boxconn.AuthorizationCodeRequest) (boxconn.TokenResponse, error)

type boxOAuthStartResponse struct {
	ID           string `json:"id"`
	AuthorizeURL string `json:"authorizeUrl"`
}

type boxOAuthJob struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Alias        string `json:"alias,omitempty"`
	Identity     string `json:"identity,omitempty"`
	Account      string `json:"account,omitempty"`
	state        string
	verifier     string
	redirectURL  string
	clientID     string
	clientSecret string
	expires      time.Time
}

type boxOAuthManager struct {
	mu      sync.Mutex
	jobs    map[string]*boxOAuthJob
	byState map[string]string
	now     func() time.Time
}

func newBoxOAuthManager(now func() time.Time) *boxOAuthManager {
	if now == nil {
		now = time.Now
	}
	return &boxOAuthManager{jobs: map[string]*boxOAuthJob{}, byState: map[string]string{}, now: now}
}

func (manager *boxOAuthManager) start() (boxOAuthStartResponse, error) {
	clientID, clientSecret := boxconn.BoxOAuthApp()
	if clientID == "" || clientSecret == "" {
		return boxOAuthStartResponse{}, fmt.Errorf("set BOX_CLIENT_ID and BOX_CLIENT_SECRET in .env before logging in")
	}
	verifier, challenge, err := boxconn.NewPKCE()
	if err != nil {
		return boxOAuthStartResponse{}, fmt.Errorf("could not start Box login")
	}
	state := randomID()
	authorizeURL, err := boxconn.AuthorizationURL(boxconn.AuthorizationRequest{
		ClientID: clientID, RedirectURL: boxconn.LoginCallbackURL, State: state, CodeChallenge: challenge,
	})
	if err != nil {
		return boxOAuthStartResponse{}, err
	}
	job := &boxOAuthJob{
		ID: randomID(), Status: "pending", Message: "Waiting for Box login",
		state: state, verifier: verifier, redirectURL: boxconn.LoginCallbackURL,
		clientID: clientID, clientSecret: clientSecret, expires: manager.now().Add(2 * time.Minute),
	}
	manager.mu.Lock()
	manager.jobs[job.ID] = job
	manager.byState[state] = job.ID
	manager.mu.Unlock()
	return boxOAuthStartResponse{ID: job.ID, AuthorizeURL: authorizeURL}, nil
}

func (manager *boxOAuthManager) get(id string) (boxOAuthJob, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job, ok := manager.jobs[id]
	if !ok {
		return boxOAuthJob{}, false
	}
	return job.public(), true
}

func (job boxOAuthJob) public() boxOAuthJob {
	return boxOAuthJob{ID: job.ID, Status: job.Status, Message: job.Message, Alias: job.Alias, Identity: job.Identity, Account: job.Account}
}

func (manager *boxOAuthManager) lookup(state string) (*boxOAuthJob, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	id, ok := manager.byState[strings.TrimSpace(state)]
	if !ok {
		return nil, fmt.Errorf("Box login session was not found")
	}
	job := manager.jobs[id]
	if job == nil {
		return nil, fmt.Errorf("Box login session was not found")
	}
	if manager.now().After(job.expires) {
		job.Status = "failed"
		job.Message = "Box login timed out. Start login again from Dispatch."
		delete(manager.byState, job.state)
		return job, fmt.Errorf("%s", job.Message)
	}
	if job.Status != "pending" {
		return job, fmt.Errorf("Box login session was already used")
	}
	return job, nil
}

func (manager *boxOAuthManager) finish(job *boxOAuthJob, status, message, alias, identity, account string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job.Status = status
	job.Message = message
	job.Alias = alias
	job.Identity = identity
	job.Account = account
	delete(manager.byState, job.state)
}

func (manager *boxOAuthManager) failState(state, message string) {
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

func applyBoxOAuthToken(settings config.ConnectionSettings, clientID, clientSecret string, token boxconn.TokenResponse, identity, account, enterprise string) config.ConnectionSettings {
	settings.BoxDefaultConnection = boxconn.DispatchOAuthName
	app := config.BoxAppConnection{
		Alias: identity, ClientID: clientID, ClientSecret: clientSecret,
		RefreshToken: token.RefreshToken, AccessToken: token.AccessToken,
		Identity: identity, Account: account, Enterprise: enterprise,
	}
	if existing, ok := settings.SelectedBoxConnection(); ok && (existing.RefreshToken == token.RefreshToken || (existing.Account == account && account != "")) {
		app.ID = existing.ID
	}
	return settings.UpsertBoxConnection(app, true)
}
