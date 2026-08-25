package webapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
)

type runAction string

const (
	runActionValidate runAction = "validate"
	runActionDeploy   runAction = "deploy"
)

type runStatus string

const (
	runQueued    runStatus = "queued"
	runRunning   runStatus = "running"
	runCompleted runStatus = "completed"
	runFailed    runStatus = "failed"
)

type runEvent struct {
	Sequence      int                     `json:"sequence"`
	At            time.Time               `json:"at"`
	Type          string                  `json:"type"`
	Provider      string                  `json:"provider,omitempty"`
	Message       string                  `json:"message"`
	Status        runStatus               `json:"status,omitempty"`
	Component     string                  `json:"component,omitempty"`
	ProgressState lifecycle.ProgressState `json:"progressState,omitempty"`
	Current       int                     `json:"current,omitempty"`
	Total         int                     `json:"total,omitempty"`
}

type runResponse struct {
	ID          string            `json:"id"`
	Action      runAction         `json:"action"`
	Status      runStatus         `json:"status"`
	CreatedAt   time.Time         `json:"createdAt"`
	StartedAt   *time.Time        `json:"startedAt,omitempty"`
	CompletedAt *time.Time        `json:"completedAt,omitempty"`
	Providers   []providerSummary `json:"providers,omitempty"`
}

type runDiagnostic struct {
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	Provider        string   `json:"provider,omitempty"`
	Code            string   `json:"code,omitempty"`
	NextSteps       []string `json:"nextSteps"`
	TechnicalDetail string   `json:"technicalDetail,omitempty"`
}

type persistedRun struct {
	ID          string            `json:"id"`
	Action      runAction         `json:"action"`
	Status      runStatus         `json:"status"`
	CreatedAt   time.Time         `json:"createdAt"`
	StartedAt   *time.Time        `json:"startedAt,omitempty"`
	CompletedAt *time.Time        `json:"completedAt,omitempty"`
	Providers   []providerSummary `json:"providers,omitempty"`
	Events      []runEvent        `json:"events,omitempty"`
	Diagnostic  *runDiagnostic    `json:"diagnostic,omitempty"`
}

type runExecutor func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error)

type runManager struct {
	mu       sync.RWMutex
	runs     map[string]*deploymentRun
	next     int
	now      func() time.Time
	validate runExecutor
	deploy   runExecutor
	store    runStore
}

type deploymentRun struct {
	id          string
	action      runAction
	status      runStatus
	plan        config.SolutionPlan
	items       []lifecycle.Item
	createdAt   time.Time
	startedAt   *time.Time
	completedAt *time.Time
	events      []runEvent
	listeners   map[chan runEvent]struct{}
	diagnostic  *runDiagnostic
}

func newRunManager() *runManager {
	return newRunManagerWithStore(validatePlanRun, deployPlanRun, time.Now, defaultRunStore())
}

func newRunManagerWithExecutors(validate, deploy runExecutor, now func() time.Time) *runManager {
	return newRunManagerWithStore(validate, deploy, now, memoryRunStore{})
}

func newRunManagerWithStore(validate, deploy runExecutor, now func() time.Time, store runStore) *runManager {
	manager := &runManager{runs: map[string]*deploymentRun{}, validate: validate, deploy: deploy, now: now, store: store}
	manager.restoreHistory()
	return manager
}

func (m *runManager) startValidation(plan config.SolutionPlan) (runResponse, error) {
	if plan.TemplateID == "" || plan.PackagePath == "" {
		return runResponse{}, fmt.Errorf("assemble a package in Dispatch before validation")
	}
	return m.start(runActionValidate, plan, nil, m.validate), nil
}

func (m *runManager) startDeployment(validationID string) (runResponse, error) {
	m.mu.RLock()
	validation := m.runs[validationID]
	if validation == nil {
		m.mu.RUnlock()
		return runResponse{}, fmt.Errorf("validation run was not found")
	}
	if validation.action != runActionValidate || validation.status != runCompleted {
		m.mu.RUnlock()
		return runResponse{}, fmt.Errorf("complete a successful validation before applying changes")
	}
	if validation.plan.PackagePath == "" {
		m.mu.RUnlock()
		return runResponse{}, fmt.Errorf("rerun validation after restarting the local API before applying changes")
	}
	plan := validation.plan
	items := append([]lifecycle.Item(nil), validation.items...)
	m.mu.RUnlock()
	for _, item := range items {
		if item.Status == lifecycle.StatusFailed {
			return runResponse{}, fmt.Errorf("resolve validation failures before applying changes")
		}
	}
	return m.start(runActionDeploy, plan, items, m.deploy), nil
}

func (m *runManager) start(action runAction, plan config.SolutionPlan, items []lifecycle.Item, executor runExecutor) runResponse {
	m.mu.Lock()
	m.next++
	now := m.now().UTC()
	run := &deploymentRun{
		id: fmt.Sprintf("web-%s-%04d", now.Format("20060102T150405Z"), m.next), action: action,
		status: runQueued, plan: plan, items: items, createdAt: now, listeners: map[chan runEvent]struct{}{},
	}
	m.runs[run.id] = run
	m.appendEventLocked(run, "status", "", "Run queued", runQueued)
	response := summarizeRun(run)
	m.saveLocked()
	m.mu.Unlock()
	go m.execute(run.id, executor)
	return response
}

func (m *runManager) execute(id string, executor runExecutor) {
	m.mu.Lock()
	run := m.runs[id]
	if run == nil {
		m.mu.Unlock()
		return
	}
	now := m.now().UTC()
	run.status, run.startedAt = runRunning, &now
	m.appendEventLocked(run, "status", "", "Run started", runRunning)
	plan, items := run.plan, append([]lifecycle.Item(nil), run.items...)
	m.saveLocked()
	m.mu.Unlock()

	result, err := executor(context.Background(), plan, items, func(provider string, update lifecycle.ProgressUpdate) {
		m.mu.Lock()
		if current := m.runs[id]; current != nil {
			m.appendProgressEventLocked(current, provider, update)
		}
		m.mu.Unlock()
	})
	if err == nil {
		err = failedRunResult(result)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	run = m.runs[id]
	if run == nil {
		return
	}
	run.items = result
	completed := m.now().UTC()
	run.completedAt = &completed
	if err != nil {
		run.status = runFailed
		run.diagnostic = safeDiagnostic(run.action, err)
		m.appendEventLocked(run, "status", "", "Run needs attention. Review the diagnostic guidance before trying again.", runFailed)
		m.saveLocked()
		return
	}
	run.status = runCompleted
	m.appendEventLocked(run, "status", "", "Run completed", runCompleted)
	m.saveLocked()
}

type providerRunFailure struct {
	Provider   string
	Detail     string
	Diagnostic string
}

func (f *providerRunFailure) Error() string {
	return providerName(f.Provider) + ": " + f.Detail
}

func failedRunResult(items []lifecycle.Item) error {
	failures := make([]string, 0)
	var primary *providerRunFailure
	for _, item := range items {
		if item.Status != lifecycle.StatusFailed {
			continue
		}
		detail := strings.TrimSpace(item.Detail)
		if detail == "" {
			detail = "provider returned a failed result"
		}
		failures = append(failures, providerName(item.Provider)+": "+detail)
		if primary == nil {
			primary = &providerRunFailure{Provider: item.Provider, Detail: detail, Diagnostic: strings.TrimSpace(item.Diagnostic)}
		}
	}
	if len(failures) == 0 {
		return nil
	}
	primary.Detail = strings.Join(failures, "; ")
	return primary
}

func (m *runManager) response(id string) (runResponse, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run := m.runs[id]
	if run == nil {
		return runResponse{}, false
	}
	return summarizeRun(run), true
}

func (m *runManager) history() []runResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runs := make([]runResponse, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, summarizeRun(run))
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs
}

func (m *runManager) diagnostics(id string) (runDiagnostic, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run := m.runs[id]
	if run == nil || run.diagnostic == nil {
		return runDiagnostic{}, false
	}
	return *run.diagnostic, true
}

func (m *runManager) subscribe(id string) ([]runEvent, <-chan runEvent, func(), bool) {
	m.mu.Lock()
	run := m.runs[id]
	if run == nil {
		m.mu.Unlock()
		return nil, nil, nil, false
	}
	snapshot := append([]runEvent(nil), run.events...)
	if run.status == runCompleted || run.status == runFailed {
		m.mu.Unlock()
		return snapshot, nil, func() {}, true
	}
	listener := make(chan runEvent, 1024)
	run.listeners[listener] = struct{}{}
	cancel := func() {
		m.mu.Lock()
		delete(run.listeners, listener)
		m.mu.Unlock()
	}
	m.mu.Unlock()
	return snapshot, listener, cancel, true
}

func (m *runManager) appendEventLocked(run *deploymentRun, eventType, provider, message string, status runStatus) {
	event := runEvent{Sequence: len(run.events) + 1, At: m.now().UTC(), Type: eventType, Provider: provider, Message: message, Status: status}
	run.events = append(run.events, event)
	for listener := range run.listeners {
		select {
		case listener <- event:
		default:
		}
	}
}

func (m *runManager) appendProgressEventLocked(run *deploymentRun, provider string, update lifecycle.ProgressUpdate) {
	event := runEvent{
		Sequence: len(run.events) + 1, At: m.now().UTC(), Type: "activity", Provider: provider,
		Message: update.Message, Status: run.status, Component: update.Component,
		ProgressState: update.State, Current: update.Current, Total: update.Total,
	}
	run.events = append(run.events, event)
	for listener := range run.listeners {
		select {
		case listener <- event:
		default:
		}
	}
}

func summarizeRun(run *deploymentRun) runResponse {
	providers := make([]providerSummary, 0, len(run.items))
	for _, item := range run.items {
		providers = append(providers, providerSummary{Name: item.Provider, Status: string(item.Status)})
	}
	return runResponse{ID: run.id, Action: run.action, Status: run.status, CreatedAt: run.createdAt, StartedAt: run.startedAt, CompletedAt: run.completedAt, Providers: providers}
}

func (m *runManager) restoreHistory() {
	if m.store == nil {
		return
	}
	persisted, err := m.store.Load()
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, saved := range persisted {
		if saved.ID == "" {
			continue
		}
		run := &deploymentRun{
			id: saved.ID, action: saved.Action, status: saved.Status, createdAt: saved.CreatedAt,
			startedAt: saved.StartedAt, completedAt: saved.CompletedAt,
			items: summariesToItems(saved.Providers), events: append([]runEvent(nil), saved.Events...),
			diagnostic: saved.Diagnostic, listeners: map[chan runEvent]struct{}{},
		}
		if run.status == runQueued || run.status == runRunning {
			completed := m.now().UTC()
			run.status, run.completedAt = runFailed, &completed
			run.diagnostic = &runDiagnostic{
				Title:           "Local run was interrupted",
				Summary:         "Dispatch stopped before this run completed, so no provider result was recorded.",
				Code:            "RUN_INTERRUPTED",
				NextSteps:       []string{"Confirm the local Dispatch service is running, then start validation again."},
				TechnicalDetail: "The local service stopped while the run was queued or active.",
			}
			run.events = append(run.events, runEvent{Sequence: len(run.events) + 1, At: completed, Type: "status", Message: "Run was interrupted when the local API stopped.", Status: runFailed})
		}
		m.runs[run.id] = run
	}
	m.next = len(m.runs)
	m.saveLocked()
}

func (m *runManager) saveLocked() {
	if m.store == nil {
		return
	}
	persisted := make([]persistedRun, 0, len(m.runs))
	for _, run := range m.runs {
		persisted = append(persisted, persistedRun{
			ID: run.id, Action: run.action, Status: run.status, CreatedAt: run.createdAt,
			StartedAt: run.startedAt, CompletedAt: run.completedAt, Providers: summarizeRun(run).Providers,
			Events: append([]runEvent(nil), run.events...), Diagnostic: run.diagnostic,
		})
	}
	_ = m.store.Save(persisted)
}

func summariesToItems(summaries []providerSummary) []lifecycle.Item {
	items := make([]lifecycle.Item, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, lifecycle.Item{Provider: summary.Name, Status: lifecycle.Status(summary.Status)})
	}
	return items
}

func safeDiagnostic(action runAction, err error) *runDiagnostic {
	summary := "Dispatch could not complete the provider operation."
	nextSteps := []string{"Check the selected connection and package configuration, then run validation again."}
	provider := ""
	detail := err.Error()
	var providerFailure *providerRunFailure
	if errors.As(err, &providerFailure) {
		provider = providerName(providerFailure.Provider)
		detail = strings.TrimSpace(strings.Join([]string{providerFailure.Detail, providerFailure.Diagnostic}, "\n\n"))
		if provider != "" {
			summary = provider + " could not complete validation."
		}
	}
	if action == runActionDeploy {
		summary = "Dispatch could not apply the validated configuration."
		nextSteps = []string{"Review the provider configuration and retry deployment after validation succeeds."}
	}
	message := strings.ToLower(detail)
	code := "PROVIDER_OPERATION_FAILED"
	switch {
	case strings.Contains(message, "box oauth session has expired"), strings.Contains(message, "box") && strings.Contains(message, "invalid_grant"):
		summary = "The selected Box connection is no longer authorized."
		nextSteps = []string{"Return to Connect and reconnect the selected Box account.", "Run validation again after Dispatch verifies the renewed connection."}
		code = "BOX_OAUTH_SESSION_EXPIRED"
		provider = "Box"
	case strings.Contains(message, "context deadline exceeded"), strings.Contains(message, "client.timeout exceeded"):
		summary = "Salesforce metadata inventory did not respond before Dispatch's provider timeout."
		nextSteps = []string{"Retry validation; Dispatch will keep reporting metadata progress while it waits.", "If it continues, reconnect the selected Salesforce org and retry."}
		code = "SALESFORCE_METADATA_TIMEOUT"
		provider = "Salesforce"
	case strings.Contains(message, "parse salesforce metadata inventory") && (strings.Contains(message, "unexpected eof") || strings.Contains(message, ": eof")):
		summary = "Salesforce ended a metadata inventory response before it was complete."
		nextSteps = []string{"Retry validation; Dispatch retries one incomplete metadata response automatically.", "If it continues, reconnect the selected Salesforce org and retry."}
		code = "SALESFORCE_METADATA_RESPONSE_INCOMPLETE"
		provider = "Salesforce"
	case strings.Contains(message, "conflict"):
		summary = "Existing Salesforce source conflicts with the package you are applying."
		nextSteps = []string{"Resolve the source conflicts in the Salesforce project.", "Run validation again before applying changes."}
		code = "SALESFORCE_SOURCE_CONFLICT"
	case strings.Contains(message, "http 420"), strings.Contains(message, "http_420"), strings.Contains(message, "unable to read salesforce metadata"):
		summary = "Salesforce could not be reached with the current org session."
		nextSteps = []string{"Confirm the selected Salesforce org is active and authenticated.", "Recheck the connection, then run validation again."}
		code = "SALESFORCE_ORG_UNREACHABLE"
		provider = "Salesforce"
	case strings.Contains(message, "noscratchinfo"), strings.Contains(message, "no information for scratch org"), strings.Contains(message, "unable to inspect salesforce org"):
		summary = "The selected Salesforce scratch org is no longer available through its Dev Hub."
		nextSteps = []string{
			"Return to Connect and reconnect the Salesforce org to refresh its authorization.",
			"If the org has expired or was deleted, choose an active org or create a replacement scratch org.",
			"Run validation again after the replacement connection is verified.",
		}
		code = "SALESFORCE_SCRATCH_ORG_UNAVAILABLE"
		provider = "Salesforce"
	case strings.Contains(message, "invalid_session_id"), strings.Contains(message, "session expired"), strings.Contains(message, "session invalid"):
		summary = "The selected Salesforce session is no longer valid."
		nextSteps = []string{"Return to Connect and reconnect the selected Salesforce org.", "Run validation again after Dispatch confirms the refreshed connection."}
		code = "SALESFORCE_SESSION_EXPIRED"
		provider = "Salesforce"
	case strings.Contains(message, "expired"), strings.Contains(message, "scratch org") && strings.Contains(message, "inactive"):
		summary = "The selected Salesforce org is expired or inactive."
		nextSteps = []string{"Choose an active Salesforce org on the Connect step.", "Run validation again with the replacement org."}
		code = "SALESFORCE_ORG_INACTIVE"
		provider = "Salesforce"
	case strings.Contains(message, "managed package"):
		summary = "The required managed package is unavailable in the selected Salesforce org."
		nextSteps = []string{"Install or update the managed package, then run validation again."}
		code = "SALESFORCE_PACKAGE_MISSING"
		provider = "Salesforce"
	case strings.Contains(message, "system administrator"):
		summary = "The selected Salesforce user is not a System Administrator."
		nextSteps = []string{"Return to Connect and choose a Salesforce org authenticated by a System Administrator.", "Run validation again."}
		code = "SALESFORCE_ADMIN_REQUIRED"
		provider = "Salesforce"
	case strings.Contains(message, "maximum size of request"):
		summary = "The Salesforce metadata request exceeded the provider limit."
		nextSteps = []string{"Reduce the number of Salesforce components in this deployment.", "Run validation again with a smaller component set."}
		code = "SALESFORCE_REQUEST_TOO_LARGE"
		provider = "Salesforce"
	case strings.Contains(message, "did not contain json"), strings.Contains(message, "unreadable"):
		summary = "Salesforce returned an unreadable response during validation."
		nextSteps = []string{"Reconnect the Salesforce org and retry validation.", "If the problem continues, update the local Salesforce provider integration."}
		code = "SALESFORCE_RESPONSE_UNREADABLE"
		provider = "Salesforce"
	}
	title := "Run needs attention"
	if provider != "" {
		title = provider + " validation needs attention"
	}
	return &runDiagnostic{
		Title:           title,
		Summary:         summary,
		Provider:        provider,
		Code:            code,
		NextSteps:       nextSteps,
		TechnicalDetail: sanitizeDiagnosticDetail(detail),
	}
}

var (
	diagnosticSecretPattern = regexp.MustCompile(`(?i)("?(?:access_token|refresh_token|client_secret|authorization)"?\s*[:=]\s*"?)[^",\s]+`)
	diagnosticBearerPattern = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	diagnosticTokenPattern  = regexp.MustCompile(`(?i)\b[a-z0-9._-]*secret[a-z0-9._-]*\b`)
	diagnosticPathPattern   = regexp.MustCompile(`(?:/Users|/private|/var|/tmp)/[^\s"']+`)
	diagnosticQueryPattern  = regexp.MustCompile(`(https?://[^\s?]+)\?[^\s]+`)
)

func sanitizeDiagnosticDetail(value string) string {
	value = strings.TrimSpace(value)
	value = diagnosticSecretPattern.ReplaceAllString(value, `${1}[redacted]`)
	value = diagnosticBearerPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = diagnosticTokenPattern.ReplaceAllString(value, "[redacted]")
	value = diagnosticPathPattern.ReplaceAllString(value, "[local path]")
	value = diagnosticQueryPattern.ReplaceAllString(value, `${1}?[redacted]`)
	value = strings.ReplaceAll(value, "ERROR_HTTP_420", "HTTP 420")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 {
			return r
		}
		return -1
	}, value)
	const maxDiagnosticLength = 8000
	if len(value) > maxDiagnosticLength {
		value = value[:maxDiagnosticLength] + "\n… diagnostic truncated"
	}
	if value == "" {
		return "The provider did not return additional technical detail."
	}
	return value
}

func validatePlanRun(ctx context.Context, plan config.SolutionPlan, _ []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
	items := make([]lifecycle.Item, 0, len(plan.Components))
	for _, provider := range plan.Components {
		if err := ctx.Err(); err != nil {
			return items, err
		}
		emit(provider, lifecycle.ProgressUpdate{Message: "Validating " + providerName(provider) + " configuration", State: lifecycle.ProgressActivity})
		item, err := lifecycle.ValidateProvider(plan.PackagePath, provider, func(update lifecycle.ProgressUpdate) { emit(provider, update) })
		if err != nil {
			return items, err
		}
		items = append(items, item)
		if item.Status == lifecycle.StatusFailed {
			emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " validation failed: " + item.Detail, State: lifecycle.ProgressFailed})
			continue
		}
		emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " validation complete", State: lifecycle.ProgressCompleted})
	}
	return items, nil
}

func deployPlanRun(ctx context.Context, plan config.SolutionPlan, items []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
	before := append([]lifecycle.Item(nil), items...)
	startedAt := time.Now().UTC()
	for index, item := range items {
		if err := ctx.Err(); err != nil {
			return items, err
		}
		if item.Status != lifecycle.StatusMissing || !item.Deployable {
			emit(item.Provider, lifecycle.ProgressUpdate{Message: providerName(item.Provider) + " requires no supported changes", State: lifecycle.ProgressCompleted})
			continue
		}
		emit(item.Provider, lifecycle.ProgressUpdate{Message: "Applying " + providerName(item.Provider) + " configuration", State: lifecycle.ProgressRunning})
		result, err := lifecycle.DeployProvider(plan.PackagePath, item, func(update lifecycle.ProgressUpdate) { emit(item.Provider, update) })
		if err != nil {
			return items, err
		}
		items[index] = result
		emitDeploymentResult(result, emit)
	}
	if _, err := audit.ExportDeployment(plan.PackagePath, before, items, startedAt, time.Now().UTC()); err != nil {
		return items, fmt.Errorf("record deployment audit: %w", err)
	}
	return items, nil
}

func emitDeploymentResult(item lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) {
	if item.Status == lifecycle.StatusFailed {
		emit(item.Provider, lifecycle.ProgressUpdate{Message: providerName(item.Provider) + " configuration failed: " + item.Detail, State: lifecycle.ProgressFailed})
		return
	}
	emit(item.Provider, lifecycle.ProgressUpdate{Message: providerName(item.Provider) + " configuration applied", State: lifecycle.ProgressCompleted})
}
