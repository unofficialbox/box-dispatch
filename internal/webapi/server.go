// Package webapi exposes a local, credential-safe control-plane boundary for
// the Dispatch web application. Browser requests execute the same explicit
// lifecycle operations behind plain commands; the browser never sees secrets.
package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type deploymentStore func() ([]audit.DeploymentRecord, error)
type connectionStore func() (config.ConnectionSettings, error)
type salesforceExperiencePath func(context.Context, salesforceapi.Credential, string, string) (string, error)

// ServerOptions makes the local API testable without touching operator state.
// Nil stores use the production audit and BCL-backed connection settings.
type ServerOptions struct {
	Profile                  string
	DeploymentStore          deploymentStore
	ConnectionStore          connectionStore
	ConnectionSaver          connectionSaver
	BoxCheck                 boxConnectionCheck
	SalesforceTargets        salesforceTargetStore
	SalesforceCheck          salesforceCheck
	SalesforceDevHubCheck    salesforceDevHubCheck
	SalesforceCreate         salesforceScratchCreate
	SalesforcePackagePrepare salesforcePackagePrepare
	SalesforceOAuth          salesforceOAuthExchange
	SalesforceRefresh        salesforceTokenRefresh
	SalesforceScratchAccess  salesforceScratchAccess
	SalesforceScratchOpen    salesforceScratchOpen
	SalesforceExperiencePath salesforceExperiencePath
	BoxOAuth                 boxOAuthExchange
	SalesforceCallbackReady  func() bool
	BoxCallbackReady         func() bool
	PlanStore                planStore
	PlanSaver                planSaver
	DefaultsStore            deploymentDefaultsStore
	DefaultsSaver            deploymentDefaultsSaver
	Templates                templateStore
	PackageAssembler         packageAssembler
	Runs                     *runManager
	Now                      func() time.Time
}

// NewHandler returns the API exposed to a local browser. Mount it only on a
// loopback listener; the handler intentionally has no CORS policy.
func NewHandler(profile string) http.Handler {
	return NewHandlerWithOptions(ServerOptions{Profile: profile})
}

// NewHandlerWithOptions returns a handler with injectable read-only stores.
func NewHandlerWithOptions(options ServerOptions) http.Handler {
	if options.DeploymentStore == nil {
		options.DeploymentStore = audit.ListDeployments
	}
	if options.ConnectionStore == nil {
		options.ConnectionStore = loadConnections
	}
	if options.ConnectionSaver == nil {
		options.ConnectionSaver = savePersistedConnections
	}
	if options.BoxCheck == nil {
		options.BoxCheck = verifyBoxConnection
	}
	if options.SalesforceTargets == nil {
		options.SalesforceTargets = listSalesforceTargets
	}
	salesforceClient := salesforceapi.NewClient()
	if options.SalesforceCheck == nil {
		options.SalesforceCheck = salesforceClient.Check
	}
	if options.SalesforceCreate == nil {
		options.SalesforceCreate = salesforceClient.CreateScratch
		if options.SalesforceDevHubCheck == nil {
			options.SalesforceDevHubCheck = salesforceClient.HasDevHub
		}
	}
	if options.SalesforcePackagePrepare == nil {
		options.SalesforcePackagePrepare = func(ctx context.Context, plan config.SolutionPlan, credential salesforceapi.Credential, report func(scratchPackageProgress)) (string, error) {
			return prepareSalesforcePackages(ctx, salesforceClient, plan, credential, report)
		}
	}
	if options.SalesforceOAuth == nil {
		options.SalesforceOAuth = salesforceClient.ExchangeAuthorizationCode
	}
	if options.SalesforceRefresh == nil {
		options.SalesforceRefresh = salesforceClient.RefreshAccessToken
	}
	if options.SalesforceScratchAccess == nil {
		options.SalesforceScratchAccess = recoverSalesforceScratchAccess
	}
	if options.SalesforceScratchOpen == nil {
		options.SalesforceScratchOpen = openSalesforceScratch
	}
	if options.SalesforceExperiencePath == nil {
		options.SalesforceExperiencePath = salesforceClient.ExperienceEmployeePath
	}
	if options.BoxOAuth == nil {
		options.BoxOAuth = boxconn.ExchangeAuthorizationCode
	}
	if options.PlanStore == nil {
		options.PlanStore = loadPlan
	}
	if options.PlanSaver == nil {
		options.PlanSaver = savePlan
	}
	if options.DefaultsStore == nil {
		options.DefaultsStore = loadDeploymentDefaults
	}
	if options.DefaultsSaver == nil {
		options.DefaultsSaver = saveDeploymentDefaults
	}
	if options.Templates == nil {
		options.Templates = loadPackageTemplates
	}
	if options.PackageAssembler == nil {
		options.PackageAssembler = assemblePackage
	}
	if options.Runs == nil {
		options.Runs = newRunManager()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	profile := strings.TrimSpace(options.Profile)
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("BOX_DISPATCH_PROFILE"))
	}
	scratchJobs := newScratchJobManager(options.SalesforceCreate, options.SalesforcePackagePrepare, options.ConnectionStore, options.PlanStore, options.ConnectionSaver, options.SalesforceDevHubCheck)
	oauthJobs := newSalesforceOAuthManager(options.Now)
	boxOAuthJobs := newBoxOAuthManager(options.Now)
	if profile == "" {
		profile = "default"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Profile: profile, ServerTime: options.Now().UTC()})
	})
	mux.HandleFunc("GET /api/deployments", func(w http.ResponseWriter, _ *http.Request) {
		records, err := options.DeploymentStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "deployment history is unavailable")
			return
		}
		summaries := make([]deploymentSummary, 0, len(records))
		for _, record := range records {
			summaries = append(summaries, summarizeDeployment(record))
		}
		writeJSON(w, http.StatusOK, summaries)
	})
	mux.HandleFunc("GET /api/deployments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		records, err := options.DeploymentStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "deployment history is unavailable")
			return
		}
		for _, record := range records {
			if record.DeploymentID == id {
				settings, _ := options.ConnectionStore()
				writeJSON(w, http.StatusOK, detailDeployment(record, settings))
				return
			}
		}
		writeError(w, http.StatusNotFound, "deployment was not found")
	})
	mux.HandleFunc("GET /api/deployments/{id}/changes", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		records, err := options.DeploymentStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "deployment history is unavailable")
			return
		}
		for _, record := range records {
			if record.DeploymentID == id {
				writeJSON(w, http.StatusOK, runChangesResponse{Files: record.FileChanges()})
				return
			}
		}
		writeError(w, http.StatusNotFound, "deployment was not found")
	})
	mux.HandleFunc("GET /api/connections", func(w http.ResponseWriter, _ *http.Request) {
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, connectionSummaries(settings))
	})
	mux.HandleFunc("GET /api/connections/salesforce/options", func(w http.ResponseWriter, _ *http.Request) {
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = settings.HydrateSalesforceOrgs()
		if len(settings.SalesforceOrgs) > 0 {
			writeJSON(w, http.StatusOK, presentSalesforceOrgOptions(settings))
			return
		}
		if settings.HasSalesforceREST() {
			writeJSON(w, http.StatusOK, presentSalesforceRESTOption(settings))
			return
		}
		targets, err := options.SalesforceTargets()
		if err != nil {
			writeJSON(w, http.StatusOK, []salesforceConnectionOption{})
			return
		}
		writeJSON(w, http.StatusOK, presentSalesforceOptions(settings, targets))
	})
	mux.HandleFunc("PUT /api/connections/salesforce/rest", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input salesforceRESTInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || ensureEndOfJSON(decoder) != nil {
			writeError(w, http.StatusBadRequest, "Salesforce connection must be one valid JSON object")
			return
		}
		input = input.normalized()
		if err := input.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = saveSalesforceREST(settings, input)
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save the Salesforce connection")
			return
		}
		writeJSON(w, http.StatusOK, presentSalesforceConnection(settings))
	})
	mux.HandleFunc("POST /api/connections/salesforce/check", func(w http.ResponseWriter, r *http.Request) {
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = settings.HydrateSalesforceOrgs()
		if !settings.HasSalesforceREST() {
			writeError(w, http.StatusBadRequest, "connect a Salesforce org before checking availability")
			return
		}
		scratchConnection := isSalesforceScratchConnection(settings)
		if scratchConnection && settings.SalesforceExpirationDate != "" {
			expiresAt, parseErr := time.Parse("2006-01-02", settings.SalesforceExpirationDate)
			if parseErr == nil && expiresAt.Before(options.Now().UTC().Truncate(24*time.Hour)) {
				settings = settings.InvalidateSelectedSalesforceVerification("Expired")
				_ = options.ConnectionSaver(settings)
				writeError(w, http.StatusGone, "The selected Salesforce scratch org expired. Create a replacement scratch org to continue.")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		credential := targetCredential(settings)
		status, err := options.SalesforceCheck(ctx, credential)
		if err != nil {
			if refreshed, refreshErr := refreshSalesforceAccess(ctx, options.SalesforceRefresh, settings, credential); refreshErr == nil {
				settings = refreshed
				status, err = options.SalesforceCheck(ctx, targetCredential(settings))
			}
		}
		if err != nil {
			settings = settings.InvalidateSelectedSalesforceVerification("Unavailable")
			_ = options.ConnectionSaver(settings)
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		orgID := firstNonEmpty(status.OrgID, settings.SalesforceOrgID)
		username := firstNonEmpty(status.Username, selectedSalesforceUsername(settings))
		status.OrgID = orgID
		status.Username = username
		if orgID != "" {
			settings.SalesforceOrgID = orgID
		}
		settings.SalesforceOrgStatus = status.Status
		if status.InstanceURL != "" {
			settings.SalesforceInstanceURL = status.InstanceURL
		}
		settings = settings.SyncSelectedSalesforceOrg()
		if settings.VerifiedConnections == nil {
			settings.VerifiedConnections = map[string]config.VerifiedConnection{}
		}
		authType := "Salesforce REST API"
		if settings.SalesforceRefreshToken != "" {
			authType = "Salesforce OAuth"
		}
		settings.VerifiedConnections["salesforce"] = config.VerifiedConnection{VerifiedAt: options.Now().UTC().Format(time.RFC3339), Selection: settings.SalesforceAlias, Identity: username, OrgID: orgID, OrgStatus: status.Status, OrgType: settings.SalesforceOrgType, ExpiresAt: settings.SalesforceExpirationDate, AuthType: authType}
		if options.SalesforceDevHubCheck != nil {
			if hasHub, hubErr := options.SalesforceDevHubCheck(ctx, targetCredential(settings)); hubErr == nil && hasHub {
				settings = settings.MarkSelectedAsDevHub()
			}
		}
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "Salesforce is available, but Dispatch could not save the check")
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("GET /api/connections/salesforce/open", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = settings.HydrateSalesforceOrgs()
		if !settings.HasSalesforceREST() {
			writeError(w, http.StatusBadRequest, "connect a Salesforce org before opening Salesforce")
			return
		}
		scratchConnection := isSalesforceScratchConnection(settings)
		if scratchConnection && settings.SalesforceExpirationDate != "" {
			expiresAt, parseErr := time.Parse("2006-01-02", settings.SalesforceExpirationDate)
			if parseErr == nil && expiresAt.Before(options.Now().UTC().Truncate(24*time.Hour)) {
				settings = settings.InvalidateSelectedSalesforceVerification("Expired")
				_ = options.ConnectionSaver(settings)
				writeError(w, http.StatusGone, "The selected Salesforce scratch org expired. Create a replacement scratch org to continue.")
				return
			}
		}
		credential := targetCredential(settings)
		scratchTarget := ""
		if scratchConnection && settings.SalesforceRefreshToken == "" {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			recovered, target, recoverErr := recoverSelectedScratchAccess(ctx, options.SalesforceScratchAccess, settings)
			if recoverErr != nil {
				settings = settings.InvalidateSelectedSalesforceVerification("Reconnect required")
				_ = options.ConnectionSaver(settings)
				writeError(w, http.StatusUnauthorized, "Salesforce could not renew the selected scratch-org session. Reconnect the org, then try again.")
				return
			}
			settings = recovered
			scratchTarget = target
			credential = targetCredential(settings)
			if err := options.ConnectionSaver(settings); err != nil {
				writeError(w, http.StatusInternalServerError, "Salesforce renewed the scratch-org session, but Dispatch could not save it")
				return
			}
		} else if settings.SalesforceRefreshToken != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			refreshed, refreshErr := refreshSalesforceAccess(ctx, options.SalesforceRefresh, settings, credential)
			if refreshErr != nil {
				settings = settings.InvalidateSelectedSalesforceVerification("Reconnect required")
				_ = options.ConnectionSaver(settings)
				writeError(w, http.StatusUnauthorized, "Salesforce could not renew the selected org session. Reconnect the org, then try again.")
				return
			}
			settings = refreshed
			credential = targetCredential(settings)
			if err := options.ConnectionSaver(settings); err != nil {
				writeError(w, http.StatusInternalServerError, "Salesforce renewed the session, but Dispatch could not save it")
				return
			}
		}
		returnPath := "/lightning/page/home"
		switch strings.TrimSpace(r.URL.Query().Get("destination")) {
		case "box-settings":
			returnPath = "/lightning/n/box__Box_Settings"
		case "clm-app":
			returnPath = "/lightning/app/c__CLM_Demo"
		case "experience-site":
			networkID := strings.TrimSpace(r.URL.Query().Get("site"))
			if networkID == "" {
				writeError(w, http.StatusBadRequest, "the Experience Cloud site is missing")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			returnPath, err = options.SalesforceExperiencePath(ctx, credential, "", networkID)
			if err != nil {
				writeError(w, http.StatusBadGateway, "Salesforce could not open the Experience Cloud site for the selected employee")
				return
			}
		}
		launchURL := ""
		if scratchTarget != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			launchURL, err = options.SalesforceScratchOpen(ctx, scratchTarget, returnPath)
		} else {
			launchURL, err = salesforceFrontDoorURL(credential, returnPath)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "the selected Salesforce org is not ready to open")
			return
		}
		w.Header().Set("Location", launchURL)
		w.WriteHeader(http.StatusSeeOther)
	})
	mux.HandleFunc("POST /api/connections/salesforce/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if options.SalesforceCallbackReady != nil && !options.SalesforceCallbackReady() {
			writeError(w, http.StatusConflict, "Salesforce login needs port 1717. Stop anything using 1717 and restart Dispatch.")
			return
		}
		var input salesforceOAuthStartInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || ensureEndOfJSON(decoder) != nil {
			writeError(w, http.StatusBadRequest, "Salesforce login request must be one valid JSON object")
			return
		}
		started, err := oauthJobs.start(input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, started)
	})
	handleSalesforceOAuthCallback := func(w http.ResponseWriter, r *http.Request) {
		if oauthError := strings.TrimSpace(r.URL.Query().Get("error")); oauthError != "" {
			message := explainSalesforceOAuthError(oauthError, r.URL.Query().Get("error_description"))
			oauthJobs.failState(r.URL.Query().Get("state"), message)
			writeOAuthCallbackPage(w, "Salesforce login", message, false)
			return
		}
		job, err := oauthJobs.lookup(r.URL.Query().Get("state"))
		if err != nil {
			writeOAuthCallbackPage(w, "Salesforce login", err.Error(), false)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			oauthJobs.finish(job, "failed", "Salesforce did not return an authorization code", "", "", "")
			writeOAuthCallbackPage(w, "Salesforce login", job.Message, false)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		token, err := options.SalesforceOAuth(ctx, salesforceapi.AuthorizationCodeRequest{
			LoginURL: job.loginURL, ClientID: job.clientID, ClientSecret: job.clientSecret,
			RedirectURL: job.redirectURL, Code: code, CodeVerifier: job.verifier,
		})
		if err != nil {
			oauthJobs.finish(job, "failed", err.Error(), "", "", "")
			writeOAuthCallbackPage(w, "Salesforce login", err.Error(), false)
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			oauthJobs.finish(job, "failed", "Connection settings are unavailable", "", "", "")
			writeOAuthCallbackPage(w, "Salesforce login", "Connection settings are unavailable", false)
			return
		}
		status, checkErr := options.SalesforceCheck(ctx, salesforceapi.Credential{
			InstanceURL: token.InstanceURL, AccessToken: token.AccessToken,
			ClientID: job.clientID, ClientSecret: job.clientSecret,
		})
		if checkErr != nil {
			oauthJobs.finish(job, "failed", checkErr.Error(), "", "", "")
			writeOAuthCallbackPage(w, "Salesforce login", checkErr.Error(), false)
			return
		}
		username := status.Username
		orgID := status.OrgID
		role := job.Role
		if role == "devhub" && options.SalesforceDevHubCheck != nil {
			hasHub, hubErr := options.SalesforceDevHubCheck(ctx, salesforceapi.Credential{
				InstanceURL: token.InstanceURL, AccessToken: token.AccessToken,
				ClientID: job.clientID, ClientSecret: job.clientSecret,
			})
			if hubErr == nil && !hasHub {
				settings = applySalesforceOAuthToken(settings, "org", job.clientID, job.clientSecret, token, username, orgID)
				if err := options.ConnectionSaver(settings); err != nil {
					oauthJobs.finish(job, "failed", "Salesforce login succeeded, but Dispatch could not save the connection", username, username, orgID)
					writeOAuthCallbackPage(w, "Salesforce login", "Salesforce login succeeded, but Dispatch could not save the connection", false)
					return
				}
				message := "This Salesforce org is not a Dev Hub. Enable Dev Hub in Setup, then log in as that org."
				oauthJobs.finish(job, "failed", message, username, username, orgID)
				writeOAuthCallbackPage(w, "Salesforce login", message, false)
				return
			}
		}
		settings = applySalesforceOAuthToken(settings, role, job.clientID, job.clientSecret, token, username, orgID)
		if settings.VerifiedConnections == nil {
			settings.VerifiedConnections = map[string]config.VerifiedConnection{}
		}
		settings.VerifiedConnections["salesforce"] = config.VerifiedConnection{
			VerifiedAt: options.Now().UTC().Format(time.RFC3339), Selection: settings.SalesforceAlias,
			Identity: username, OrgID: orgID, OrgStatus: status.Status, AuthType: "Salesforce OAuth",
		}
		if err := options.ConnectionSaver(settings); err != nil {
			oauthJobs.finish(job, "failed", "Salesforce login succeeded, but Dispatch could not save the connection", username, username, orgID)
			writeOAuthCallbackPage(w, "Salesforce login", "Salesforce login succeeded, but Dispatch could not save the connection", false)
			return
		}
		message := "Salesforce org connected and verified."
		if job.Role == "devhub" {
			message = "Salesforce Dev Hub connected."
		}
		alias := settings.SalesforceAlias
		if job.Role == "devhub" {
			alias = settings.SalesforceDevHubAlias
		}
		oauthJobs.finish(job, "active", message, alias, username, orgID)
		writeOAuthCallbackPage(w, "Salesforce login", message, true)
	}
	mux.HandleFunc("GET /OauthRedirect", handleSalesforceOAuthCallback)
	mux.HandleFunc("GET /api/connections/salesforce/oauth/callback", handleSalesforceOAuthCallback)
	mux.HandleFunc("GET /api/connections/salesforce/oauth/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, ok := oauthJobs.get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "Salesforce login was not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("POST /api/salesforce/scratch-orgs", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input scratchCreateInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || ensureEndOfJSON(decoder) != nil {
			writeError(w, http.StatusBadRequest, "scratch-org request must be one valid JSON object")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		if !settings.HasSalesforceDevHub() {
			writeError(w, http.StatusBadRequest, "connect a Salesforce Dev Hub before creating a scratch org")
			return
		}
		writeJSON(w, http.StatusAccepted, scratchJobs.start(input))
	})
	mux.HandleFunc("GET /api/salesforce/scratch-orgs/latest", func(w http.ResponseWriter, _ *http.Request) {
		job, ok := scratchJobs.latest()
		if !ok {
			writeError(w, http.StatusNotFound, "scratch-org request was not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("GET /api/salesforce/scratch-orgs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, ok := scratchJobs.get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "scratch-org request was not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("PUT /api/connections/box", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input boxConnectionInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || ensureEndOfJSON(decoder) != nil {
			writeError(w, http.StatusBadRequest, "Box connection must be one valid JSON object")
			return
		}
		input = input.normalized()
		if err := input.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = saveBoxCCGSelection(settings, input)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		verification, err := options.BoxCheck(ctx, settings)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		verification.VerifiedAt = options.Now().UTC().Format(time.RFC3339)
		if verification.Selection == "" {
			verification.Selection = firstNonEmpty(settings.BoxCCGAlias, boxconn.DispatchCCGName)
		}
		settings = settings.MarkBoxVerified(verification)
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "Box accepted the connection, but Dispatch could not save it")
			return
		}
		writeJSON(w, http.StatusOK, presentBoxConnection(settings, verification))
	})
	mux.HandleFunc("POST /api/connections/box/check", func(w http.ResponseWriter, r *http.Request) {
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = settings.HydrateBoxConnections()
		if !settings.HasBoxConnection() && !hasOAuthEnvironment() {
			writeError(w, http.StatusBadRequest, "connect Box before checking availability")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		verification, err := options.BoxCheck(ctx, settings)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		verification.VerifiedAt = options.Now().UTC().Format(time.RFC3339)
		if verification.Selection == "" {
			verification.Selection = firstNonEmpty(settings.BoxCCGAlias, boxconn.DispatchCCGName)
		}
		settings = settings.MarkBoxVerified(verification)
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "Box is available, but Dispatch could not save the check")
			return
		}
		writeJSON(w, http.StatusOK, presentBoxConnection(settings, verification))
	})
	mux.HandleFunc("PUT /api/connections/box/selection", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input boxConnectionSelection
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || ensureEndOfJSON(decoder) != nil {
			writeError(w, http.StatusBadRequest, "Box connection selection must be one valid JSON object")
			return
		}
		input.ID = strings.TrimSpace(input.ID)
		input.Alias = strings.TrimSpace(input.Alias)
		if input.ID == "" && input.Alias == "" {
			writeError(w, http.StatusBadRequest, "select a connected Box app")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings, err = settings.SelectBoxConnection(firstNonEmpty(input.ID, input.Alias))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save the Box selection")
			return
		}
		writeJSON(w, http.StatusOK, presentBoxConnection(settings, settings.VerifiedConnections["box"]))
	})
	mux.HandleFunc("DELETE /api/connections/box/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeError(w, http.StatusBadRequest, "select a connected Box app to remove")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings, err = settings.RemoveBoxConnection(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove the Box connection")
			return
		}
		writeJSON(w, http.StatusOK, presentBoxConnection(settings, settings.VerifiedConnections["box"]))
	})
	mux.HandleFunc("POST /api/connections/box/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		if options.BoxCallbackReady != nil && !options.BoxCallbackReady() {
			writeError(w, http.StatusConflict, "Box login needs port 4400. Stop anything using 4400 and restart Dispatch.")
			return
		}
		started, err := boxOAuthJobs.start()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, started)
	})
	handleBoxOAuthCallback := func(w http.ResponseWriter, r *http.Request) {
		if oauthError := strings.TrimSpace(r.URL.Query().Get("error")); oauthError != "" {
			message := firstNonEmpty(r.URL.Query().Get("error_description"), oauthError, "Box login did not finish")
			boxOAuthJobs.failState(r.URL.Query().Get("state"), message)
			writeOAuthCallbackPage(w, "Box login", message, false)
			return
		}
		job, err := boxOAuthJobs.lookup(r.URL.Query().Get("state"))
		if err != nil {
			writeOAuthCallbackPage(w, "Box login", err.Error(), false)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			boxOAuthJobs.finish(job, "failed", "Box did not return an authorization code", "", "", "")
			writeOAuthCallbackPage(w, "Box login", job.Message, false)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		token, err := options.BoxOAuth(ctx, boxconn.AuthorizationCodeRequest{
			ClientID: job.clientID, ClientSecret: job.clientSecret, RedirectURL: job.redirectURL,
			Code: code, CodeVerifier: job.verifier,
		})
		if err != nil {
			boxOAuthJobs.finish(job, "failed", err.Error(), "", "", "")
			writeOAuthCallbackPage(w, "Box login", err.Error(), false)
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			boxOAuthJobs.finish(job, "failed", "Connection settings are unavailable", "", "", "")
			writeOAuthCallbackPage(w, "Box login", "Connection settings are unavailable", false)
			return
		}
		settings = applyBoxOAuthToken(settings, job.clientID, job.clientSecret, token, "", "", "")
		verification, checkErr := options.BoxCheck(ctx, settings)
		if checkErr != nil {
			boxOAuthJobs.finish(job, "failed", checkErr.Error(), "", "", "")
			writeOAuthCallbackPage(w, "Box login", checkErr.Error(), false)
			return
		}
		verification.VerifiedAt = options.Now().UTC().Format(time.RFC3339)
		settings = applyBoxOAuthToken(settings, job.clientID, job.clientSecret, token, verification.Identity, verification.Account, verification.Enterprise)
		settings = settings.MarkBoxVerified(verification)
		if err := options.ConnectionSaver(settings); err != nil {
			boxOAuthJobs.finish(job, "failed", "Box login succeeded, but Dispatch could not save the connection", verification.Identity, verification.Identity, verification.Account)
			writeOAuthCallbackPage(w, "Box login", "Box login succeeded, but Dispatch could not save the connection", false)
			return
		}
		boxOAuthJobs.finish(job, "active", "Box connected and verified.", settings.BoxCCGAlias, verification.Identity, verification.Account)
		writeOAuthCallbackPage(w, "Box login", "Box connected and verified.", true)
	}
	mux.HandleFunc("GET /oauth/callback", handleBoxOAuthCallback)
	mux.HandleFunc("GET /api/connections/box/oauth/callback", handleBoxOAuthCallback)
	mux.HandleFunc("GET /api/connections/box/oauth/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, ok := boxOAuthJobs.get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "Box login was not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("DELETE /api/connections/salesforce/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeError(w, http.StatusBadRequest, "select a connected Salesforce org to remove")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings, err = settings.RemoveSalesforceOrg(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove the Salesforce org")
			return
		}
		writeJSON(w, http.StatusOK, presentSalesforceConnection(settings))
	})
	mux.HandleFunc("PUT /api/connections/salesforce", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input salesforceConnectionSelection
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "connection selection must be valid JSON")
			return
		}
		if err := ensureEndOfJSON(decoder); err != nil {
			writeError(w, http.StatusBadRequest, "connection selection must contain one JSON object")
			return
		}
		input.ID = strings.TrimSpace(input.ID)
		input.Alias = strings.TrimSpace(input.Alias)
		if input.ID == "" && input.Alias == "" {
			writeError(w, http.StatusBadRequest, "select an authenticated Salesforce org")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		settings = settings.HydrateSalesforceOrgs()
		if selected, err := settings.SelectSalesforceOrg(firstNonEmpty(input.ID, input.Alias)); err == nil {
			if selected.VerifiedConnections != nil {
				delete(selected.VerifiedConnections, "salesforce")
			}
			settings = selected
			if err := options.ConnectionSaver(settings); err != nil {
				writeError(w, http.StatusInternalServerError, "could not save the Salesforce selection")
				return
			}
			writeJSON(w, http.StatusOK, presentSalesforceConnection(settings))
			return
		}
		targets, err := options.SalesforceTargets()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "authenticated Salesforce orgs are unavailable")
			return
		}
		for _, target := range targets {
			if strings.EqualFold(target.Alias, input.Alias) && target.Healthy(options.Now()) {
				settings = saveSalesforceSelection(settings, target)
				if err := options.ConnectionSaver(settings); err != nil {
					writeError(w, http.StatusInternalServerError, "could not save the Salesforce selection")
					return
				}
				writeJSON(w, http.StatusOK, connectionSummary{Name: "Salesforce", Configured: true, Selection: settings.SalesforceAlias, Status: settings.SalesforceOrgStatus, ExpiresAt: settings.SalesforceExpirationDate})
				return
			}
		}
		writeError(w, http.StatusBadRequest, "select a currently authenticated Salesforce org")
	})
	mux.HandleFunc("GET /api/plan", func(w http.ResponseWriter, _ *http.Request) {
		plan, err := options.PlanStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "saved plan is unavailable")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, presentPlan(plan, settings))
	})
	mux.HandleFunc("GET /api/defaults", func(w http.ResponseWriter, _ *http.Request) {
		defaults, err := options.DefaultsStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "deployment defaults are unavailable")
			return
		}
		plan, err := options.PlanStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "saved plan is unavailable")
			return
		}
		templates, err := options.Templates()
		if err != nil || len(templates) == 0 {
			writeError(w, http.StatusInternalServerError, "solution quickstarts are unavailable")
			return
		}
		writeJSON(w, http.StatusOK, presentDeploymentDefaults(resolveDeploymentDefaults(defaults, plan, templates)))
	})
	mux.HandleFunc("PUT /api/defaults", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input deploymentDefaultsUpdate
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "deployment defaults must be valid JSON with supported fields")
			return
		}
		if err := ensureEndOfJSON(decoder); err != nil {
			writeError(w, http.StatusBadRequest, "deployment defaults must contain one JSON object")
			return
		}
		input = input.normalized()
		if err := input.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		templates, err := options.Templates()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "solution quickstarts are unavailable")
			return
		}
		selected, ok := packageTemplateByID(templates, input.TemplateID)
		if !ok {
			writeError(w, http.StatusBadRequest, "choose an available solution quickstart")
			return
		}
		defaults := config.DeploymentDefaults{TemplateID: selected.ID, Template: selected.Name, Repository: selected.repository, Components: input.Components, Strategy: input.Strategy}
		if err := options.DefaultsSaver(defaults); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save deployment defaults")
			return
		}
		writeJSON(w, http.StatusOK, presentDeploymentDefaults(defaults))
	})
	mux.HandleFunc("PUT /api/plan", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input planUpdate
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "plan must be valid JSON with supported fields")
			return
		}
		if err := ensureEndOfJSON(decoder); err != nil {
			writeError(w, http.StatusBadRequest, "plan must contain one JSON object")
			return
		}
		input = input.normalized()
		if err := input.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing, err := options.PlanStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "saved plan is unavailable")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		plan := config.SolutionPlan{
			Name: input.Name, Components: input.Components, TemplateID: input.TemplateID,
			Template: input.Template, Repository: input.Repository, Strategy: input.Strategy,
			// PackagePath is intentionally not browser-addressable in this slice.
			PackagePath: existing.PackagePath,
		}
		if err := setPackageStrategy(plan, input.Strategy, options.Now()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update the package deployment strategy")
			return
		}
		if err := options.PlanSaver(plan); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save the plan")
			return
		}
		writeJSON(w, http.StatusOK, presentPlan(plan, settings))
	})
	mux.HandleFunc("GET /api/templates", func(w http.ResponseWriter, _ *http.Request) {
		templates, err := options.Templates()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "solution quickstarts are unavailable")
			return
		}
		writeJSON(w, http.StatusOK, templates)
	})
	mux.HandleFunc("POST /api/packages", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input packageAssemblyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "package request must be valid JSON")
			return
		}
		if err := ensureEndOfJSON(decoder); err != nil {
			writeError(w, http.StatusBadRequest, "package request must contain one JSON object")
			return
		}
		input = input.normalized()
		if err := input.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		templates, err := options.Templates()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "solution quickstarts are unavailable")
			return
		}
		var selected packageTemplate
		for _, template := range templates {
			if template.ID == input.TemplateID {
				selected = template
				break
			}
		}
		if selected.ID == "" {
			writeError(w, http.StatusBadRequest, "choose an available solution quickstart")
			return
		}
		plan, err := options.PackageAssembler(selected, input.Components, input.Strategy)
		if err != nil {
			// Workspace and git diagnostics can reveal local paths. Keep those
			// details in the local service log while giving the browser a complete
			// recovery action that does not assume terminal knowledge.
			writeError(w, http.StatusInternalServerError, "Dispatch could not assemble the selected template. Verify the template source is available, then try again.")
			return
		}
		plan.Name = input.Name
		if err := options.PlanSaver(plan); err != nil {
			writeError(w, http.StatusInternalServerError, "Dispatch could not save the assembled deployment plan")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		writeJSON(w, http.StatusCreated, presentPlan(plan, settings))
	})
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, options.Runs.history())
	})
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, _ *http.Request) {
		plan, err := options.PlanStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "saved plan is unavailable")
			return
		}
		run, err := options.Runs.startValidation(plan)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	})
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, ok := options.Runs.response(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "run was not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	})
	mux.HandleFunc("GET /api/runs/{id}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		diagnostic, ok := options.Runs.diagnostics(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "diagnostic guidance is unavailable for this run")
			return
		}
		writeJSON(w, http.StatusOK, diagnostic)
	})
	mux.HandleFunc("GET /api/runs/{id}/changes", func(w http.ResponseWriter, r *http.Request) {
		changes, ok := options.Runs.changes(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "run was not found")
			return
		}
		writeJSON(w, http.StatusOK, changes)
	})
	mux.HandleFunc("POST /api/runs/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
		run, err := options.Runs.startDeployment(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	})
	mux.HandleFunc("GET /api/runs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		streamRunEvents(w, r, options.Runs)
	})
	return noStore(http.HandlerFunc(mux.ServeHTTP))
}

type healthResponse struct {
	Profile    string    `json:"profile"`
	ServerTime time.Time `json:"serverTime"`
}

type deploymentSummary struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Strategy    string            `json:"strategy"`
	CompletedAt time.Time         `json:"completedAt"`
	Providers   []providerSummary `json:"providers"`
}

type deploymentDetail struct {
	deploymentSummary
	StartedAt       time.Time        `json:"startedAt"`
	Duration        string           `json:"duration"`
	RunID           string           `json:"runId,omitempty"`
	ChangesRecorded bool             `json:"changesRecorded"`
	ChangeCount     int              `json:"changeCount"`
	Providers       []providerDetail `json:"providers"`
}

type providerSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type providerDetail struct {
	Name               string   `json:"name"`
	Status             string   `json:"status"`
	DeployedCount      int      `json:"deployedCount"`
	PresentCount       int      `json:"presentCount"`
	RemainingCount     int      `json:"remainingCount"`
	ManualItemCount    int      `json:"manualItemCount"`
	DeployedComponents []string `json:"deployedComponents"`
	EnvironmentID      string   `json:"environmentId,omitempty"`
	LaunchURL          string   `json:"launchUrl,omitempty"`
}

type connectionSummary struct {
	Name             string                       `json:"name"`
	Configured       bool                         `json:"configured"`
	Verified         bool                         `json:"verified"`
	AuthType         string                       `json:"authType,omitempty"`
	Selection        string                       `json:"selection,omitempty"`
	Status           string                       `json:"status,omitempty"`
	ExpiresAt        string                       `json:"expiresAt,omitempty"`
	Alias            string                       `json:"alias,omitempty"`
	SubjectType      string                       `json:"subjectType,omitempty"`
	ClientIDHint     string                       `json:"clientIdHint,omitempty"`
	SubjectIDHint    string                       `json:"subjectIdHint,omitempty"`
	RestConfigured   bool                         `json:"restConfigured,omitempty"`
	DevHubConfigured bool                         `json:"devHubConfigured,omitempty"`
	OAuthConfigured  bool                         `json:"oauthConfigured,omitempty"`
	LaunchURL        string                       `json:"launchUrl,omitempty"`
	Orgs             []salesforceConnectionOption `json:"orgs,omitempty"`
	Connections      []boxConnectionOption        `json:"connections,omitempty"`
}

type planStore func() (config.SolutionPlan, error)
type planSaver func(config.SolutionPlan) error
type deploymentDefaultsStore func() (config.DeploymentDefaults, error)
type deploymentDefaultsSaver func(config.DeploymentDefaults) error

type planResponse struct {
	Exists     bool            `json:"exists"`
	Name       string          `json:"name,omitempty"`
	TemplateID string          `json:"templateId,omitempty"`
	Template   string          `json:"template,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Strategy   string          `json:"strategy"`
	Components []planComponent `json:"components"`
}

type deploymentDefaultsResponse struct {
	TemplateID string   `json:"templateId"`
	Template   string   `json:"template"`
	Repository string   `json:"repository"`
	Strategy   string   `json:"strategy"`
	Components []string `json:"components"`
}

type deploymentDefaultsUpdate struct {
	TemplateID string   `json:"templateId"`
	Strategy   string   `json:"strategy"`
	Components []string `json:"components"`
}

func (d deploymentDefaultsUpdate) normalized() deploymentDefaultsUpdate {
	d.TemplateID = strings.TrimSpace(d.TemplateID)
	d.Strategy = strings.ToLower(strings.TrimSpace(d.Strategy))
	if d.Strategy == "" {
		d.Strategy = solution.StrategyReuse
	}
	for index, component := range d.Components {
		d.Components[index] = strings.ToLower(strings.TrimSpace(component))
	}
	return d
}

func (d deploymentDefaultsUpdate) validate() error {
	request := packageAssemblyRequest{Name: "Deployment defaults", TemplateID: d.TemplateID, Components: d.Components, Strategy: d.Strategy}
	return request.validate()
}

type planComponent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Verified   bool   `json:"verified"`
	Ready      bool   `json:"ready"`
}

type planUpdate struct {
	Name       string   `json:"name"`
	TemplateID string   `json:"templateId"`
	Template   string   `json:"template"`
	Repository string   `json:"repository"`
	Components []string `json:"components"`
	Strategy   string   `json:"strategy"`
}

func (p planUpdate) validate() error {
	if p.Name == "" {
		return fmt.Errorf("enter a deployment name")
	}
	if utf8.RuneCountInString(p.Name) > 80 {
		return fmt.Errorf("deployment name must be 80 characters or fewer")
	}
	if p.Strategy != solution.StrategyReuse && p.Strategy != solution.StrategyCreateNew {
		return fmt.Errorf("choose reuse existing or create new")
	}
	if p.TemplateID == "" || p.Template == "" || p.Repository == "" {
		return fmt.Errorf("template, template ID, and repository are required")
	}
	if len(p.Components) == 0 {
		return fmt.Errorf("select at least Box")
	}
	seen := map[string]bool{}
	hasBox := false
	for _, component := range p.Components {
		if component != "box" && component != "salesforce" {
			return fmt.Errorf("%q is not available for browser planning", component)
		}
		if seen[component] {
			return fmt.Errorf("%q is selected more than once", component)
		}
		seen[component] = true
		hasBox = hasBox || component == "box"
	}
	if !hasBox {
		return fmt.Errorf("Box is required")
	}
	return nil
}

func (p planUpdate) normalized() planUpdate {
	p.Name = normalizeDeploymentName(p.Name)
	p.TemplateID = strings.TrimSpace(p.TemplateID)
	p.Template = strings.TrimSpace(p.Template)
	p.Repository = strings.TrimSpace(p.Repository)
	p.Strategy = strings.ToLower(strings.TrimSpace(p.Strategy))
	if p.Strategy == "" {
		p.Strategy = solution.StrategyReuse
	}
	for index, component := range p.Components {
		p.Components[index] = strings.ToLower(strings.TrimSpace(component))
	}
	return p
}

func presentPlan(plan config.SolutionPlan, settings config.ConnectionSettings) planResponse {
	name := strings.TrimSpace(plan.Name)
	if name == "" {
		name = strings.TrimSpace(plan.Template)
	}
	response := planResponse{
		Exists: plan.TemplateID != "", TemplateID: plan.TemplateID,
		Name: name, Template: plan.Template, Repository: plan.Repository, Strategy: plan.Strategy,
		Components: make([]planComponent, 0, len(plan.Components)),
	}
	if response.Strategy == "" {
		response.Strategy = solution.StrategyReuse
	}
	connections := connectionSummaries(settings)
	byName := make(map[string]connectionSummary, len(connections))
	for _, connection := range connections {
		byName[strings.ToLower(connection.Name)] = connection
	}
	for _, component := range plan.Components {
		connection := byName[component]
		response.Components = append(response.Components, planComponent{
			ID: component, Name: providerName(component), Configured: connection.Configured,
			Verified: connection.Verified, Ready: connection.Configured && connection.Verified,
		})
	}
	return response
}

func providerName(provider string) string {
	switch provider {
	case "box":
		return "Box"
	case "salesforce":
		return "Salesforce"
	default:
		return provider
	}
}

func ensureEndOfJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("extra JSON value")
		}
		return err
	}
	return nil
}

func summarizeDeployment(record audit.DeploymentRecord) deploymentSummary {
	providers := make([]providerSummary, 0, len(record.Providers))
	for _, provider := range record.Providers {
		providers = append(providers, providerSummary{Name: provider.Provider, Status: string(provider.StatusAfter)})
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		name = record.TemplateID
	}
	if name == "" {
		name = "Dispatch deployment"
	}
	return deploymentSummary{
		ID: record.DeploymentID, Name: name, Strategy: record.Strategy,
		CompletedAt: record.CompletedAt, Providers: providers,
	}
}

func detailDeployment(record audit.DeploymentRecord, settings config.ConnectionSettings) deploymentDetail {
	summary := summarizeDeployment(record)
	providers := make([]providerDetail, 0, len(record.Providers))
	for _, provider := range record.Providers {
		environmentID, launchURL := deploymentEnvironment(provider, settings)
		providers = append(providers, providerDetail{
			Name: provider.Provider, Status: string(provider.StatusAfter),
			DeployedCount: len(provider.Deployed), PresentCount: len(provider.PresentAfter),
			RemainingCount:     len(provider.Remaining),
			ManualItemCount:    len(provider.AdapterPending) + len(provider.Experimental),
			DeployedComponents: append([]string(nil), provider.Deployed...),
			EnvironmentID:      environmentID, LaunchURL: launchURL,
		})
	}
	return deploymentDetail{
		deploymentSummary: summary, StartedAt: record.StartedAt,
		Duration: record.Duration, RunID: record.RunID, ChangesRecorded: record.ChangesRecorded,
		ChangeCount: len(record.FileChanges()), Providers: providers,
	}
}

func deploymentEnvironment(provider audit.ProviderRecord, settings config.ConnectionSettings) (string, string) {
	switch strings.ToLower(provider.Provider) {
	case "box":
		if strings.TrimSpace(provider.EnvironmentID) != "" {
			return provider.EnvironmentID, "https://app.box.com/"
		}
		if selected, ok := settings.SelectedBoxConnection(); ok {
			return selected.Enterprise, "https://app.box.com/"
		}
	case "salesforce":
		environmentID := strings.TrimSpace(provider.EnvironmentID)
		for _, resource := range provider.Resources {
			if resource.Kind != "organization" || strings.TrimSpace(resource.ID) == "" {
				continue
			}
			if environmentID == "" {
				environmentID = resource.ID
			}
			launchURL := strings.TrimSpace(resource.URL)
			if selected, ok := settings.SelectedSalesforceOrg(); ok && selected.OrgID == environmentID {
				launchURL = "/api/connections/salesforce/open"
			}
			return environmentID, launchURL
		}
		if environmentID != "" {
			if selected, ok := settings.SelectedSalesforceOrg(); ok && selected.OrgID == environmentID {
				return environmentID, "/api/connections/salesforce/open"
			}
		}
	}
	return "", ""
}

func connectionSummaries(settings config.ConnectionSettings) []connectionSummary {
	verified := settings.VerifiedConnections
	boxVerification := verified["box"]

	boxAuthType := ""
	boxConfigured := false
	if settings.HasBoxOAuth() || hasOAuthEnvironment() {
		boxConfigured = true
		boxAuthType = "Box OAuth"
	} else if settings.HasBoxCCG() {
		boxConfigured = true
		boxAuthType = "client credentials"
	}
	boxSummary := presentBoxConnection(settings, boxVerification)
	boxSummary.Configured = boxConfigured
	boxSummary.AuthType = boxAuthType
	if boxSummary.Configured {
		boxSummary.LaunchURL = "https://app.box.com/"
	}
	connections := []connectionSummary{
		boxSummary,
		presentSalesforceConnection(settings),
	}
	if settings.DatabricksProfile != "" || settings.DatabricksHost != "" {
		connections = append(connections, connectionSummary{
			Name: "Databricks", Configured: true, Verified: verified["databricks"].VerifiedAt != "",
			Selection: settings.DatabricksProfile,
		})
	}
	if settings.AWSProfile != "" || settings.AWSRegion != "" {
		connections = append(connections, connectionSummary{
			Name: "AWS", Configured: true, Verified: verified["aws"].VerifiedAt != "",
			Selection: settings.AWSProfile,
		})
	}
	return connections
}

func presentBoxConnection(settings config.ConnectionSettings, verification config.VerifiedConnection) connectionSummary {
	settings = settings.HydrateBoxConnections()
	alias := strings.TrimSpace(settings.BoxCCGAlias)
	if alias == "" && settings.HasBoxOAuth() {
		alias = firstNonEmpty(verification.Identity, "Box")
	}
	if alias == "" && settings.HasBoxCCG() {
		alias = "Box CCG"
	}
	selection := alias
	if selection == "" {
		selection = safeSelection(verification.Selection)
	}
	authType := "client credentials"
	if settings.HasBoxOAuth() || hasOAuthEnvironment() {
		authType = "Box OAuth"
	}
	return connectionSummary{
		Name: "Box", Configured: settings.HasBoxConnection() || hasOAuthEnvironment(), Verified: verification.VerifiedAt != "",
		AuthType: authType, Selection: selection, Status: connectionReadiness(verification.VerifiedAt != ""),
		Alias: alias, SubjectType: settings.BoxCCGSubjectType, ClientIDHint: identifierHint(settings.BoxCCGClientID),
		SubjectIDHint: identifierHint(settings.BoxCCGSubjectID), OAuthConfigured: boxconn.HasBoxOAuthApp(),
		Connections: presentBoxConnectionOptions(settings),
	}
}

func presentSalesforceConnection(settings config.ConnectionSettings) connectionSummary {
	settings = settings.HydrateSalesforceOrgs()
	verification := settings.VerifiedConnections["salesforce"]
	alias := strings.TrimSpace(settings.SalesforceAlias)
	if alias == "" && settings.HasSalesforceREST() {
		alias = restConnectionAlias()
	}
	authType := ""
	switch {
	case settings.SalesforceRefreshToken != "" || settings.SalesforceDevHubRefreshToken != "":
		authType = "Salesforce OAuth"
	case settings.HasSalesforceREST() || settings.HasSalesforceDevHub():
		authType = "Salesforce REST API"
	case alias != "":
		authType = "Salesforce CLI"
	}
	launchURL := ""
	if settings.HasSalesforceREST() {
		launchURL = "/api/connections/salesforce/open"
	}
	return connectionSummary{
		Name: "Salesforce", Configured: alias != "" || settings.HasSalesforceREST() || settings.HasSalesforceDevHub() || len(settings.SalesforceOrgs) > 0,
		Verified: verification.VerifiedAt != "", AuthType: authType, Selection: alias,
		Status: connectionReadiness(verification.VerifiedAt != ""), ExpiresAt: settings.SalesforceExpirationDate, Alias: alias,
		ClientIDHint: identifierHint(settings.SalesforceClientID), RestConfigured: settings.HasSalesforceREST(),
		DevHubConfigured: settings.HasSalesforceDevHub(), OAuthConfigured: settings.HasSalesforceOAuthApp(),
		LaunchURL: launchURL,
		Orgs:      presentSalesforceOrgOptions(settings),
	}
}

func safeHTTPSLaunchURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	parsed.Path = "/"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func identifierHint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "Configured"
	}
	return "Ending in " + value[len(value)-4:]
}

func hasOAuthEnvironment() bool {
	return strings.TrimSpace(os.Getenv("BOX_CLIENT_ID")) != "" &&
		strings.TrimSpace(os.Getenv("BOX_CLIENT_SECRET")) != "" &&
		strings.TrimSpace(os.Getenv("BOX_REFRESH_TOKEN")) != ""
}

func safeSelection(value string) string {
	if strings.EqualFold(value, "ccg") {
		return "CCG"
	}
	return ""
}

func loadConnections() (config.ConnectionSettings, error) {
	// Imported lazily through this small seam to keep the public API mapper free
	// from BCL persistence details.
	return loadPersistedConnections()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func streamRunEvents(w http.ResponseWriter, r *http.Request, runs *runManager) {
	snapshot, events, cancel, ok := runs.subscribe(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "run was not found")
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "event streaming is unavailable")
		return
	}
	writeEvents(w, flusher, snapshot)
	for {
		if events == nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			writeEvents(w, flusher, []runEvent{event})
			if event.Status == runCompleted || event.Status == runFailed {
				return
			}
		}
	}
}

func writeEvents(w http.ResponseWriter, flusher http.Flusher, events []runEvent) {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "event: dispatch\ndata: %s\n\n", data)
	}
	flusher.Flush()
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
