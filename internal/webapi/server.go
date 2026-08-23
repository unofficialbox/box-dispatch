// Package webapi exposes a local, credential-safe control-plane boundary for
// the Dispatch web application. Browser requests execute the same explicit
// lifecycle operations as the terminal shell; the browser never sees secrets.
package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type deploymentStore func() ([]audit.DeploymentRecord, error)
type connectionStore func() (config.ConnectionSettings, error)

// ServerOptions makes the local API testable without touching operator state.
// Nil stores use the production audit and BCL-backed connection settings.
type ServerOptions struct {
	Profile           string
	DeploymentStore   deploymentStore
	ConnectionStore   connectionStore
	ConnectionSaver   connectionSaver
	BoxCheck          boxConnectionCheck
	SalesforceTargets salesforceTargetStore
	SalesforceCheck   salesforceCheck
	SalesforceCreate  salesforceScratchCreate
	PlanStore         planStore
	PlanSaver         planSaver
	Templates         templateStore
	PackageAssembler  packageAssembler
	Runs              *runManager
	Now               func() time.Time
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
	}
	if options.PlanStore == nil {
		options.PlanStore = loadPlan
	}
	if options.PlanSaver == nil {
		options.PlanSaver = savePlan
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
	scratchJobs := newScratchJobManager(options.SalesforceCreate, options.ConnectionStore, options.ConnectionSaver)
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
				writeJSON(w, http.StatusOK, detailDeployment(record))
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
		writeJSON(w, http.StatusOK, connectionSummary{Name: "Salesforce", Configured: true, AuthType: "Salesforce REST API", Selection: settings.SalesforceAlias, Status: "Needs availability check", ExpiresAt: settings.SalesforceExpirationDate})
	})
	mux.HandleFunc("POST /api/connections/salesforce/check", func(w http.ResponseWriter, r *http.Request) {
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
			return
		}
		if !settings.HasSalesforceREST() {
			writeError(w, http.StatusBadRequest, "connect a Salesforce org before checking availability")
			return
		}
		if strings.EqualFold(settings.SalesforceOrgType, "scratch") && settings.SalesforceExpirationDate != "" {
			expiresAt, parseErr := time.Parse("2006-01-02", settings.SalesforceExpirationDate)
			if parseErr == nil && expiresAt.Before(options.Now().UTC().Truncate(24*time.Hour)) {
				settings.SalesforceOrgStatus = "Expired"
				_ = options.ConnectionSaver(settings)
				writeError(w, http.StatusGone, "The selected Salesforce scratch org expired. Create a replacement scratch org to continue.")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		status, err := options.SalesforceCheck(ctx, targetCredential(settings))
		if err != nil {
			settings.SalesforceOrgStatus = "Unavailable"
			_ = options.ConnectionSaver(settings)
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		settings.SalesforceOrgID = status.OrgID
		settings.SalesforceOrgStatus = status.Status
		if settings.VerifiedConnections == nil {
			settings.VerifiedConnections = map[string]config.VerifiedConnection{}
		}
		settings.VerifiedConnections["salesforce"] = config.VerifiedConnection{VerifiedAt: options.Now().UTC().Format(time.RFC3339), Selection: settings.SalesforceAlias, Identity: status.Username, OrgID: status.OrgID, OrgStatus: status.Status, OrgType: settings.SalesforceOrgType, ExpiresAt: settings.SalesforceExpirationDate, AuthType: "Salesforce REST API"}
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "Salesforce is available, but Dispatch could not save the check")
			return
		}
		writeJSON(w, http.StatusOK, status)
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
			verification.Selection = boxconn.DispatchCCGName
		}
		if settings.VerifiedConnections == nil {
			settings.VerifiedConnections = map[string]config.VerifiedConnection{}
		}
		settings.VerifiedConnections["box"] = verification
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
		if !settings.HasBoxCCG() {
			writeError(w, http.StatusBadRequest, "connect a Box CCG app before checking availability")
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
			verification.Selection = boxconn.DispatchCCGName
		}
		if settings.VerifiedConnections == nil {
			settings.VerifiedConnections = map[string]config.VerifiedConnection{}
		}
		settings.VerifiedConnections["box"] = verification
		if err := options.ConnectionSaver(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "Box is available, but Dispatch could not save the check")
			return
		}
		writeJSON(w, http.StatusOK, presentBoxConnection(settings, verification))
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
		input.Alias = strings.TrimSpace(input.Alias)
		if input.Alias == "" {
			writeError(w, http.StatusBadRequest, "select an authenticated Salesforce org")
			return
		}
		settings, err := options.ConnectionStore()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "connection state is unavailable")
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
			Components: input.Components, TemplateID: input.TemplateID,
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
	StartedAt time.Time        `json:"startedAt"`
	Duration  string           `json:"duration"`
	RunID     string           `json:"runId,omitempty"`
	Providers []providerDetail `json:"providers"`
}

type providerSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type providerDetail struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	DeployedCount   int    `json:"deployedCount"`
	PresentCount    int    `json:"presentCount"`
	RemainingCount  int    `json:"remainingCount"`
	ManualItemCount int    `json:"manualItemCount"`
}

type connectionSummary struct {
	Name          string `json:"name"`
	Configured    bool   `json:"configured"`
	Verified      bool   `json:"verified"`
	AuthType      string `json:"authType,omitempty"`
	Selection     string `json:"selection,omitempty"`
	Status        string `json:"status,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	Alias         string `json:"alias,omitempty"`
	SubjectType   string `json:"subjectType,omitempty"`
	ClientIDHint  string `json:"clientIdHint,omitempty"`
	SubjectIDHint string `json:"subjectIdHint,omitempty"`
}

type planStore func() (config.SolutionPlan, error)
type planSaver func(config.SolutionPlan) error

type planResponse struct {
	Exists     bool            `json:"exists"`
	TemplateID string          `json:"templateId,omitempty"`
	Template   string          `json:"template,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Strategy   string          `json:"strategy"`
	Components []planComponent `json:"components"`
}

type planComponent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Verified   bool   `json:"verified"`
	Ready      bool   `json:"ready"`
}

type planUpdate struct {
	TemplateID string   `json:"templateId"`
	Template   string   `json:"template"`
	Repository string   `json:"repository"`
	Components []string `json:"components"`
	Strategy   string   `json:"strategy"`
}

func (p planUpdate) validate() error {
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
	response := planResponse{
		Exists: plan.TemplateID != "", TemplateID: plan.TemplateID,
		Template: plan.Template, Repository: plan.Repository, Strategy: plan.Strategy,
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
	name := record.TemplateID
	if name == "" {
		name = "Dispatch deployment"
	}
	return deploymentSummary{
		ID: record.DeploymentID, Name: name, Strategy: record.Strategy,
		CompletedAt: record.CompletedAt, Providers: providers,
	}
}

func detailDeployment(record audit.DeploymentRecord) deploymentDetail {
	summary := summarizeDeployment(record)
	providers := make([]providerDetail, 0, len(record.Providers))
	for _, provider := range record.Providers {
		providers = append(providers, providerDetail{
			Name: provider.Provider, Status: string(provider.StatusAfter),
			DeployedCount: len(provider.Deployed), PresentCount: len(provider.PresentAfter),
			RemainingCount:  len(provider.Remaining),
			ManualItemCount: len(provider.AdapterPending) + len(provider.Experimental),
		})
	}
	return deploymentDetail{
		deploymentSummary: summary, StartedAt: record.StartedAt,
		Duration: record.Duration, RunID: record.RunID, Providers: providers,
	}
}

func connectionSummaries(settings config.ConnectionSettings) []connectionSummary {
	verified := settings.VerifiedConnections
	boxVerification := verified["box"]
	salesforceVerification := verified["salesforce"]

	boxAuthType := ""
	boxConfigured := false
	if settings.HasBoxCCG() {
		boxConfigured = true
		boxAuthType = "client credentials"
	} else if hasOAuthEnvironment() {
		boxConfigured = true
		boxAuthType = "OAuth refresh token"
	}
	boxSummary := presentBoxConnection(settings, boxVerification)
	boxSummary.Configured = boxConfigured
	boxSummary.AuthType = boxAuthType
	connections := []connectionSummary{
		boxSummary,
		{
			Name: "Salesforce", Configured: settings.SalesforceAlias != "" || settings.HasSalesforceREST() || settings.HasSalesforceDevHub(), Verified: salesforceVerification.VerifiedAt != "",
			AuthType: func() string {
				if settings.HasSalesforceREST() || settings.HasSalesforceDevHub() {
					return "Salesforce REST API"
				}
				return "Salesforce CLI"
			}(),
			Selection: settings.SalesforceAlias, Status: settings.SalesforceOrgStatus,
			ExpiresAt: settings.SalesforceExpirationDate,
		},
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
	alias := strings.TrimSpace(settings.BoxCCGAlias)
	if alias == "" && settings.HasBoxCCG() {
		alias = "Box CCG"
	}
	selection := alias
	if selection == "" {
		selection = safeSelection(verification.Selection)
	}
	status := verification.AuthType
	if status == "" && settings.HasBoxCCG() {
		status = "Needs verification"
	}
	return connectionSummary{
		Name: "Box", Configured: settings.HasBoxCCG(), Verified: verification.VerifiedAt != "",
		AuthType: "client credentials", Selection: selection, Status: status, Alias: alias,
		SubjectType: settings.BoxCCGSubjectType, ClientIDHint: identifierHint(settings.BoxCCGClientID),
		SubjectIDHint: identifierHint(settings.BoxCCGSubjectID),
	}
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
