package webapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type salesforceCheck func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error)
type salesforceDevHubCheck func(context.Context, salesforceapi.Credential) (bool, error)
type salesforceScratchCreate func(context.Context, salesforceapi.Credential, salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error)
type salesforcePackagePrepare func(context.Context, config.SolutionPlan, salesforceapi.Credential, func(scratchPackageProgress)) (string, error)
type salesforceScratchAccess func(context.Context, string, string, string) (salesforceorg.ScratchAccess, error)
type salesforceScratchOpen func(context.Context, string, string) (string, error)

type scratchPackageProgress struct {
	Status    string
	Message   string
	RequestID string
}

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
	if (input.DevHubURL == "") != (input.DevHubToken == "") {
		return fmt.Errorf("Dev Hub URL and access token must be provided together")
	}
	if input.InstanceURL == "" && input.DevHubURL == "" {
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
		settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
			Alias: restConnectionAlias(), InstanceURL: input.InstanceURL, AccessToken: input.AccessToken,
			Status: "Needs availability check", OrgType: "persistent",
		}, true)
	}
	if input.DevHubURL != "" && input.DevHubToken != "" {
		if input.ClientID != "" {
			settings.SalesforceClientID = input.ClientID
			settings.SalesforceClientSecret = input.ClientSecret
		}
		settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
			Alias:       firstNonEmpty(settings.SalesforceDevHubAlias, "Connected Salesforce Dev Hub"),
			InstanceURL: input.DevHubURL, AccessToken: input.DevHubToken, OrgType: "persistent",
			ClientID: input.ClientID, ClientSecret: input.ClientSecret, Status: "Active", DevHub: true,
		}, false)
	}
	if targetChanged && settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "salesforce")
	}
	return settings
}

func restConnectionAlias() string {
	return "Connected Salesforce org"
}

func targetCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	settings = settings.HydrateSalesforceOrgs()
	clientID, clientSecret := salesforceOAuthClient(settings, config.SalesforceOrgConnection{})
	if org, ok := settings.SelectedSalesforceOrg(); ok {
		clientID, clientSecret = salesforceOAuthClient(settings, org)
	}
	return salesforceapi.Credential{InstanceURL: settings.SalesforceInstanceURL, AccessToken: settings.SalesforceAccessToken, ClientID: clientID, ClientSecret: clientSecret}
}

func isSalesforceScratchConnection(settings config.ConnectionSettings) bool {
	if strings.EqualFold(strings.TrimSpace(settings.SalesforceOrgType), "scratch") {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(settings.SalesforceInstanceURL))
	return err == nil && strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".scratch.my.salesforce.com")
}

func recoverSelectedScratchAccess(ctx context.Context, recover salesforceScratchAccess, settings config.ConnectionSettings) (config.ConnectionSettings, string, error) {
	if recover == nil {
		return settings, "", fmt.Errorf("Salesforce scratch-org session recovery is unavailable")
	}
	settings = settings.HydrateSalesforceOrgs()
	org, ok := settings.SelectedSalesforceOrg()
	if !ok {
		return settings, "", fmt.Errorf("select a Salesforce scratch org")
	}
	access, err := recover(ctx, org.OrgID, org.Username, org.InstanceURL)
	if err != nil {
		return settings, "", err
	}
	org.Username = firstNonEmpty(access.Username, org.Username)
	org.OrgID = firstNonEmpty(access.OrgID, org.OrgID)
	org.OrgType = "scratch"
	org.InstanceURL = firstNonEmpty(access.InstanceURL, org.InstanceURL)
	org.AccessToken = access.AccessToken
	org.ExpirationDate = firstNonEmpty(access.ExpirationDate, org.ExpirationDate)
	return settings.UpsertSalesforceOrg(org, true), access.Target, nil
}

func salesforceOAuthClient(settings config.ConnectionSettings, org config.SalesforceOrgConnection) (string, string) {
	clientID, clientSecret := settings.SalesforceClientID, settings.SalesforceClientSecret
	if strings.TrimSpace(org.ClientID) != "" {
		clientID, clientSecret = org.ClientID, org.ClientSecret
	}
	clientID = firstNonEmpty(clientID, salesforceapi.LoginClientID)
	if clientID == salesforceapi.LoginClientID {
		clientSecret = ""
	}
	return clientID, clientSecret
}

func salesforceRefreshLoginURLs(instanceURL string) []string {
	urls := make([]string, 0, 3)
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" || seen[strings.ToLower(raw)] {
			return
		}
		seen[strings.ToLower(raw)] = true
		urls = append(urls, raw)
	}
	add(instanceURL)
	add(salesforceapi.DefaultProductionLogin)
	add(salesforceapi.DefaultSandboxLogin)
	return urls
}

func selectedSalesforceUsername(settings config.ConnectionSettings) string {
	for _, org := range settings.SalesforceOrgs {
		if org.ID == settings.SalesforceSelectedOrgID {
			return org.Username
		}
	}
	return ""
}

func refreshSalesforceAccess(ctx context.Context, refresh salesforceTokenRefresh, settings config.ConnectionSettings, credential salesforceapi.Credential) (config.ConnectionSettings, error) {
	if refresh == nil {
		return settings, fmt.Errorf("Salesforce token refresh is unavailable")
	}
	clientID := firstNonEmpty(credential.ClientID, settings.SalesforceClientID, salesforceapi.LoginClientID)
	if settings.SalesforceRefreshToken == "" || clientID == "" {
		return settings, fmt.Errorf("Salesforce refresh token is unavailable")
	}
	var lastErr error
	for _, loginURL := range salesforceRefreshLoginURLs(settings.SalesforceInstanceURL) {
		refreshed, err := refresh(ctx, salesforceapi.RefreshRequest{
			LoginURL: loginURL, ClientID: clientID,
			ClientSecret: credential.ClientSecret, RefreshToken: settings.SalesforceRefreshToken,
		})
		if err != nil {
			lastErr = err
			continue
		}
		settings.SalesforceAccessToken = refreshed.AccessToken
		if refreshed.RefreshToken != "" {
			settings.SalesforceRefreshToken = refreshed.RefreshToken
		}
		if refreshed.InstanceURL != "" {
			settings.SalesforceInstanceURL = refreshed.InstanceURL
		}
		return settings.SyncSelectedSalesforceOrg(), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Salesforce token refresh failed")
	}
	return settings, lastErr
}

func salesforceFrontDoorURL(credential salesforceapi.Credential, returnPath string) (string, error) {
	instanceURL := safeHTTPSLaunchURL(credential.InstanceURL)
	if instanceURL == "" || strings.TrimSpace(credential.AccessToken) == "" {
		return "", fmt.Errorf("Salesforce connection is incomplete")
	}
	frontDoor, err := url.Parse(instanceURL)
	if err != nil {
		return "", fmt.Errorf("Salesforce org URL is invalid")
	}
	frontDoor.Path = "/secur/frontdoor.jsp"
	if strings.TrimSpace(returnPath) == "" {
		returnPath = "/lightning/page/home"
	}
	frontDoor.RawQuery = url.Values{
		"sid":    {credential.AccessToken},
		"retURL": {returnPath},
	}.Encode()
	return frontDoor.String(), nil
}

func devHubCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	settings = settings.HydrateSalesforceOrgs()
	clientID, clientSecret := salesforceOAuthClient(settings, config.SalesforceOrgConnection{})
	if hub, ok := settings.DevHubOrg(); ok {
		clientID, clientSecret = salesforceOAuthClient(settings, hub)
	}
	return salesforceapi.Credential{InstanceURL: settings.SalesforceDevHubURL, AccessToken: settings.SalesforceDevHubToken, ClientID: clientID, ClientSecret: clientSecret}
}

type scratchCreateInput struct {
	Alias                 string `json:"alias"`
	OrgName               string `json:"orgName"`
	DurationDays          int    `json:"durationDays"`
	InstallManagedPackage bool   `json:"installManagedPackage"`
}

type scratchJob struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	Alias          string `json:"alias,omitempty"`
	Username       string `json:"username,omitempty"`
	OrgID          string `json:"orgId,omitempty"`
	ExpirationDate string `json:"expirationDate,omitempty"`
	PackageStatus  string `json:"packageStatus,omitempty"`
	PackageMessage string `json:"packageMessage,omitempty"`
	PackageRequest string `json:"packageRequestId,omitempty"`
}

type scratchJobManager struct {
	mu             sync.RWMutex
	jobs           map[string]scratchJob
	latestID       string
	create         salesforceScratchCreate
	preparePackage salesforcePackagePrepare
	hasHub         salesforceDevHubCheck
	load           connectionStore
	loadPlan       planStore
	save           connectionSaver
}

func newScratchJobManager(create salesforceScratchCreate, preparePackage salesforcePackagePrepare, load connectionStore, loadPlan planStore, save connectionSaver, hasHub salesforceDevHubCheck) *scratchJobManager {
	return &scratchJobManager{jobs: map[string]scratchJob{}, create: create, preparePackage: preparePackage, hasHub: hasHub, load: load, loadPlan: loadPlan, save: save}
}

func (manager *scratchJobManager) start(input scratchCreateInput) scratchJob {
	job := scratchJob{ID: randomID(), Status: "queued", Message: "Waiting to create the Salesforce scratch org", Alias: strings.TrimSpace(input.Alias)}
	manager.mu.Lock()
	manager.jobs[job.ID] = job
	manager.latestID = job.ID
	manager.mu.Unlock()
	go manager.run(job.ID, input)
	return job
}

func (manager *scratchJobManager) latest() (scratchJob, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	job, ok := manager.jobs[manager.latestID]
	return job, ok
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
	settings = settings.HydrateSalesforceOrgs()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	credential := manager.credentialForCreate(ctx, settings)
	if !credential.Valid() {
		manager.fail(id, "connect a Salesforce Dev Hub before creating a scratch org")
		return
	}
	org, err := manager.create(ctx, credential, salesforceapi.ScratchRequest{Alias: input.Alias, OrgName: input.OrgName, DurationDays: input.DurationDays})
	if err != nil {
		manager.fail(id, explainScratchCreateFailure(err, settings))
		return
	}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: org.Alias, Username: org.Username, OrgID: org.OrgID, OrgType: "scratch",
		InstanceURL: org.InstanceURL, AccessToken: org.AccessToken, RefreshToken: org.RefreshToken,
		ClientID: salesforceapi.ScratchSignupClientID, Status: org.Status, ExpirationDate: org.ExpirationDate,
	}, true)
	if settings.VerifiedConnections == nil {
		settings.VerifiedConnections = map[string]config.VerifiedConnection{}
	}
	authType := "Salesforce REST API"
	if org.RefreshToken != "" {
		authType = "Salesforce OAuth"
	}
	settings.VerifiedConnections["salesforce"] = config.VerifiedConnection{VerifiedAt: time.Now().UTC().Format(time.RFC3339), Selection: org.Alias, Identity: org.Username, OrgID: org.OrgID, OrgStatus: org.Status, OrgType: "scratch", ExpiresAt: org.ExpirationDate, AuthType: authType}
	if err := manager.save(settings); err != nil {
		manager.fail(id, "The scratch org was created, but Dispatch could not save the connection")
		return
	}
	manager.update(id, func(job *scratchJob) {
		job.Status = "preparing"
		job.Message = "Scratch org created and selected"
		job.Alias = org.Alias
		job.Username = org.Username
		job.OrgID = org.OrgID
		job.ExpirationDate = org.ExpirationDate
	})
	if !input.InstallManagedPackage || manager.preparePackage == nil || manager.loadPlan == nil {
		manager.update(id, func(job *scratchJob) { job.Status = "active" })
		return
	}
	plan, err := manager.loadPlan()
	if err != nil || strings.TrimSpace(plan.PackagePath) == "" {
		manager.update(id, func(job *scratchJob) {
			job.Status = "active"
			job.PackageStatus = "skipped"
			job.PackageMessage = "Managed packages will be checked during validation"
		})
		return
	}
	manager.update(id, func(job *scratchJob) {
		job.PackageStatus = "checking"
		job.PackageMessage = "Checking required Salesforce managed packages"
	})
	orgCredential := salesforceapi.Credential{InstanceURL: org.InstanceURL, AccessToken: org.AccessToken, ClientID: salesforceapi.ScratchSignupClientID}
	message, prepareErr := manager.preparePackage(ctx, plan, orgCredential, func(progress scratchPackageProgress) {
		manager.update(id, func(job *scratchJob) {
			job.PackageStatus = progress.Status
			job.PackageMessage = progress.Message
			job.PackageRequest = progress.RequestID
		})
	})
	manager.update(id, func(job *scratchJob) {
		job.Status = "active"
		if prepareErr != nil {
			job.PackageStatus = "failed"
			job.PackageMessage = prepareErr.Error()
			job.Message = "Scratch org created, but managed-package setup needs attention"
			return
		}
		job.PackageStatus = "complete"
		job.PackageMessage = message
		job.Message = "Scratch org and managed packages are ready"
	})
}

func prepareSalesforcePackages(ctx context.Context, client *salesforceapi.Client, plan config.SolutionPlan, credential salesforceapi.Credential, report func(scratchPackageProgress)) (string, error) {
	manifest, err := solution.Load(plan.PackagePath)
	if err != nil {
		return "", fmt.Errorf("read required Salesforce packages: %w", err)
	}
	if len(manifest.Salesforce.RequiredPackages) == 0 {
		return "No managed packages are required", nil
	}
	installed, err := client.ListInstalledPackages(ctx, credential)
	if err != nil {
		return "", err
	}
	installedCount := 0
	for _, requirement := range manifest.Salesforce.RequiredPackages {
		if salesforcePackageInstalled(requirement, installed) {
			installedCount++
			continue
		}
		label := firstNonEmpty(requirement.Name, requirement.Namespace, "Salesforce managed package")
		if requirement.VersionNumber != "" {
			label += " " + requirement.VersionNumber
		}
		report(scratchPackageProgress{Status: "installing", Message: "Submitting " + label + " to Salesforce"})
		if err := client.InstallPackageWithProgress(ctx, credential, requirement.VersionID, requirement.SecurityType, func(progress salesforceapi.PackageInstallProgress) {
			status := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(progress.Status), "_", " "))
			message := "Salesforce reports " + firstNonEmpty(status, "processing")
			if progress.Elapsed >= time.Second {
				message += " · " + progress.Elapsed.Round(time.Second).String() + " elapsed"
			}
			report(scratchPackageProgress{Status: "installing", Message: message, RequestID: progress.RequestID})
		}); err != nil {
			return "", fmt.Errorf("install %s: %w", label, err)
		}
	}
	if installedCount == len(manifest.Salesforce.RequiredPackages) {
		return "Required managed packages are already installed", nil
	}
	return "Required managed packages are installed", nil
}

func salesforcePackageInstalled(requirement solution.SalesforcePackageRequirement, installed []salesforceapi.InstalledPackage) bool {
	for _, candidate := range installed {
		identityMatches := strings.EqualFold(strings.TrimSpace(candidate.Namespace), strings.TrimSpace(requirement.Namespace))
		if requirement.Namespace == "" {
			identityMatches = candidate.PackageID == requirement.PackageID || strings.EqualFold(candidate.Name, requirement.Name)
		}
		if identityMatches && (requirement.VersionID == "" || candidate.VersionID == requirement.VersionID || salesforcePackageVersionAtLeast(candidate.VersionNumber, requirement.VersionNumber)) {
			return true
		}
	}
	return false
}

func salesforcePackageVersionAtLeast(installed, required string) bool {
	if strings.TrimSpace(required) == "" {
		return true
	}
	installedParts := strings.Split(installed, ".")
	requiredParts := strings.Split(required, ".")
	if len(installedParts) != 4 || len(requiredParts) != 4 {
		return false
	}
	for index := range requiredParts {
		installedPart, installedErr := strconv.Atoi(installedParts[index])
		requiredPart, requiredErr := strconv.Atoi(requiredParts[index])
		if installedErr != nil || requiredErr != nil {
			return false
		}
		if installedPart != requiredPart {
			return installedPart > requiredPart
		}
	}
	return true
}

func (manager *scratchJobManager) credentialForCreate(ctx context.Context, settings config.ConnectionSettings) salesforceapi.Credential {
	settings = settings.HydrateSalesforceOrgs()
	seen := map[string]bool{}
	ordered := make([]salesforceapi.Credential, 0, len(settings.SalesforceOrgs)+1)
	add := func(credential salesforceapi.Credential) {
		if !credential.Valid() {
			return
		}
		key := credential.InstanceURL + "\n" + credential.AccessToken
		if seen[key] {
			return
		}
		seen[key] = true
		ordered = append(ordered, credential)
	}
	add(devHubCredential(settings))
	for _, org := range settings.SalesforceOrgs {
		if strings.EqualFold(org.OrgType, "scratch") {
			continue
		}
		clientID, clientSecret := salesforceOAuthClient(settings, org)
		add(salesforceapi.Credential{
			InstanceURL: org.InstanceURL, AccessToken: org.AccessToken,
			ClientID: clientID, ClientSecret: clientSecret,
		})
	}
	if manager.hasHub == nil {
		if len(ordered) == 0 {
			return salesforceapi.Credential{}
		}
		return ordered[0]
	}
	for _, credential := range ordered {
		ok, err := manager.hasHub(ctx, credential)
		if err == nil && ok {
			return credential
		}
	}
	if len(ordered) == 0 {
		return salesforceapi.Credential{}
	}
	return ordered[0]
}

func (manager *scratchJobManager) fail(id, message string) {
	manager.update(id, func(job *scratchJob) { job.Status = "failed"; job.Message = message })
}

func explainScratchCreateFailure(err error, settings config.ConnectionSettings) string {
	message := err.Error()
	if !strings.Contains(message, "does not have Dev Hub enabled") {
		return message
	}
	if hub, ok := settings.DevHubOrg(); ok {
		name := firstNonEmpty(hub.Username, hub.Alias, hub.InstanceURL)
		if name != "" {
			return strings.Replace(message, "this org", name, 1)
		}
	}
	return message
}

func randomID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("scratch-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
