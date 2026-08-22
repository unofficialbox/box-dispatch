package webapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

type salesforceCheck func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error)
type salesforceScratchCreate func(context.Context, salesforceapi.Credential, salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error)

type salesforceRESTInput struct {
	InstanceURL  string `json:"instanceUrl"`
	AccessToken  string `json:"accessToken"`
	DevHubURL    string `json:"devHubUrl"`
	DevHubToken  string `json:"devHubAccessToken"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

func (input salesforceRESTInput) normalized() salesforceRESTInput {
	input.InstanceURL = strings.TrimRight(strings.TrimSpace(input.InstanceURL), "/")
	input.AccessToken = strings.TrimSpace(input.AccessToken)
	input.DevHubURL = strings.TrimRight(strings.TrimSpace(input.DevHubURL), "/")
	input.DevHubToken = strings.TrimSpace(input.DevHubToken)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.ClientSecret = strings.TrimSpace(input.ClientSecret)
	return input
}

func (input salesforceRESTInput) validate() error {
	if (input.InstanceURL == "") != (input.AccessToken == "") {
		return fmt.Errorf("Salesforce org URL and access token must be provided together")
	}
	devHubFields := 0
	for _, value := range []string{input.DevHubURL, input.DevHubToken, input.ClientID} {
		if value != "" {
			devHubFields++
		}
	}
	if devHubFields > 0 && devHubFields < 3 {
		return fmt.Errorf("Dev Hub URL, Dev Hub access token, and Connected App client ID must be provided together")
	}
	if input.InstanceURL == "" && devHubFields == 0 {
		return fmt.Errorf("provide a Salesforce org connection, a Dev Hub connection, or both")
	}
	if (input.DevHubURL != "" && !strings.HasPrefix(input.DevHubURL, "https://")) || (input.InstanceURL != "" && !strings.HasPrefix(input.InstanceURL, "https://")) {
		return fmt.Errorf("Salesforce URLs must use HTTPS")
	}
	return nil
}

func saveSalesforceREST(settings config.ConnectionSettings, input salesforceRESTInput) config.ConnectionSettings {
	targetChanged := input.InstanceURL != "" && input.AccessToken != ""
	if targetChanged {
		settings.SalesforceInstanceURL = input.InstanceURL
		settings.SalesforceAccessToken = input.AccessToken
	}
	if input.DevHubURL != "" && input.DevHubToken != "" && input.ClientID != "" {
		settings.SalesforceDevHubURL = input.DevHubURL
		settings.SalesforceDevHubToken = input.DevHubToken
		settings.SalesforceClientID = input.ClientID
		settings.SalesforceClientSecret = input.ClientSecret
	}
	if targetChanged && settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "salesforce")
	}
	return settings
}

func targetCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	return salesforceapi.Credential{InstanceURL: settings.SalesforceInstanceURL, AccessToken: settings.SalesforceAccessToken, ClientID: settings.SalesforceClientID, ClientSecret: settings.SalesforceClientSecret}
}

func devHubCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	return salesforceapi.Credential{InstanceURL: settings.SalesforceDevHubURL, AccessToken: settings.SalesforceDevHubToken, ClientID: settings.SalesforceClientID, ClientSecret: settings.SalesforceClientSecret}
}

type scratchCreateInput struct {
	Alias        string `json:"alias"`
	OrgName      string `json:"orgName"`
	DurationDays int    `json:"durationDays"`
}

type scratchJob struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	Alias          string `json:"alias,omitempty"`
	Username       string `json:"username,omitempty"`
	OrgID          string `json:"orgId,omitempty"`
	ExpirationDate string `json:"expirationDate,omitempty"`
}

type scratchJobManager struct {
	mu     sync.RWMutex
	jobs   map[string]scratchJob
	create salesforceScratchCreate
	load   connectionStore
	save   connectionSaver
}

func newScratchJobManager(create salesforceScratchCreate, load connectionStore, save connectionSaver) *scratchJobManager {
	return &scratchJobManager{jobs: map[string]scratchJob{}, create: create, load: load, save: save}
}

func (manager *scratchJobManager) start(input scratchCreateInput) scratchJob {
	job := scratchJob{ID: randomID(), Status: "queued", Message: "Waiting to create the Salesforce scratch org", Alias: strings.TrimSpace(input.Alias)}
	manager.mu.Lock()
	manager.jobs[job.ID] = job
	manager.mu.Unlock()
	go manager.run(job.ID, input)
	return job
}

func (manager *scratchJobManager) get(id string) (scratchJob, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	job, ok := manager.jobs[id]
	return job, ok
}

func (manager *scratchJobManager) update(id string, update func(*scratchJob)) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	job := manager.jobs[id]
	update(&job)
	manager.jobs[id] = job
}

func (manager *scratchJobManager) run(id string, input scratchCreateInput) {
	manager.update(id, func(job *scratchJob) {
		job.Status = "creating"
		job.Message = "Salesforce is provisioning the scratch org"
	})
	settings, err := manager.load()
	if err != nil {
		manager.fail(id, "Connection settings are unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	org, err := manager.create(ctx, devHubCredential(settings), salesforceapi.ScratchRequest{Alias: input.Alias, OrgName: input.OrgName, DurationDays: input.DurationDays})
	if err != nil {
		manager.fail(id, err.Error())
		return
	}
	settings.SalesforceAlias = org.Alias
	settings.SalesforceOrgID = org.OrgID
	settings.SalesforceOrgType = "scratch"
	settings.SalesforceOrgStatus = org.Status
	settings.SalesforceExpirationDate = org.ExpirationDate
	settings.SalesforceInstanceURL = org.InstanceURL
	settings.SalesforceAccessToken = org.AccessToken
	if settings.VerifiedConnections == nil {
		settings.VerifiedConnections = map[string]config.VerifiedConnection{}
	}
	settings.VerifiedConnections["salesforce"] = config.VerifiedConnection{VerifiedAt: time.Now().UTC().Format(time.RFC3339), Selection: org.Alias, Identity: org.Username, OrgID: org.OrgID, OrgStatus: org.Status, OrgType: "scratch", ExpiresAt: org.ExpirationDate, AuthType: "Salesforce REST API"}
	if err := manager.save(settings); err != nil {
		manager.fail(id, "The scratch org was created, but Dispatch could not save the connection")
		return
	}
	manager.update(id, func(job *scratchJob) {
		job.Status = "active"
		job.Message = "Scratch org created and selected"
		job.Alias = org.Alias
		job.Username = org.Username
		job.OrgID = org.OrgID
		job.ExpirationDate = org.ExpirationDate
	})
}

func (manager *scratchJobManager) fail(id, message string) {
	manager.update(id, func(job *scratchJob) { job.Status = "failed"; job.Message = message })
}

func randomID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("scratch-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
