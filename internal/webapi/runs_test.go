package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

func TestValidationRunStreamsSafeProgressAndCompletes(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		emit("box", lifecycle.ProgressUpdate{Message: "Inspecting Box configuration", Component: "Metadata Template:Contract", State: lifecycle.ProgressRunning, Current: 1, Total: 2})
		return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent, Detail: "raw provider diagnostic"}}, nil
	}
	runs := newRunManagerWithExecutors(validate, nil, func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) })
	run, err := runs.startValidation(config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"box"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, runs, run.ID)

	_, events, cancel, ok := runs.subscribe(run.ID)
	if !ok {
		t.Fatal("run was not found")
	}
	cancel()
	if events != nil {
		t.Fatal("completed run should not retain a live listener")
	}
	response, ok := runs.response(run.ID)
	if !ok || response.Status != runCompleted || len(response.Providers) != 1 || response.Providers[0].Status != string(lifecycle.StatusPresent) {
		t.Fatalf("response = %#v, found=%t", response, ok)
	}
}

func TestValidationAuthenticatesEveryProviderBeforeComponentChecks(t *testing.T) {
	var order []string
	plan := config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"box", "salesforce"}}
	items, err := validatePlanRunWith(context.Background(), plan, func(_ string, _ lifecycle.ProgressUpdate) {}, func(_ context.Context, provider string) error {
		order = append(order, "authenticate:"+provider)
		return nil
	}, func(_ string, provider string, _ lifecycle.Reporter) (lifecycle.Item, error) {
		order = append(order, "validate:"+provider)
		return lifecycle.Item{Provider: provider, Status: lifecycle.StatusPresent}, nil
	})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{"authenticate:box", "authenticate:salesforce", "validate:box", "validate:salesforce"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
}

func TestValidationStopsBeforeComponentChecksWhenAuthenticationFails(t *testing.T) {
	var validated bool
	var updates []lifecycle.ProgressUpdate
	plan := config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"box", "salesforce"}}
	items, err := validatePlanRunWith(context.Background(), plan, func(_ string, update lifecycle.ProgressUpdate) {
		updates = append(updates, update)
	}, func(_ context.Context, provider string) error {
		if provider == "salesforce" {
			return errors.New("session is stale")
		}
		return nil
	}, func(_ string, provider string, _ lifecycle.Reporter) (lifecycle.Item, error) {
		validated = true
		return lifecycle.Item{Provider: provider, Status: lifecycle.StatusPresent}, nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if validated {
		t.Fatal("component validation ran before every provider was authenticated")
	}
	if len(items) != 1 || items[0].Provider != "salesforce" || items[0].Status != lifecycle.StatusFailed {
		t.Fatalf("items = %#v", items)
	}
	if len(updates) != 4 || updates[0].Component != "Authentication" || updates[1].State != lifecycle.ProgressCompleted || updates[3].State != lifecycle.ProgressFailed {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestRunEndpointsProvideSSEAndRequireCompletedValidationForDeploy(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		emit("box", lifecycle.ProgressUpdate{Message: "Reading package", Component: "Workspace structure", State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
		return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent}}, nil
	}
	deploy := func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		emit("box", lifecycle.ProgressUpdate{Message: "No changes needed", State: lifecycle.ProgressCompleted})
		return items, nil
	}
	runs := newRunManagerWithExecutors(validate, deploy, time.Now)
	handler := NewHandlerWithOptions(ServerOptions{
		Runs: runs,
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"box"}}, nil
		},
	})

	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/runs", nil))
	if start.Code != http.StatusAccepted {
		t.Fatalf("start status = %d: %s", start.Code, start.Body.String())
	}
	var run runResponse
	if err := json.Unmarshal(start.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	waitForRun(t, runs, run.ID)

	historyResponse := httptest.NewRecorder()
	handler.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if historyResponse.Code != http.StatusOK || !strings.Contains(historyResponse.Body.String(), run.ID) {
		t.Fatalf("history = %d: %s", historyResponse.Code, historyResponse.Body.String())
	}

	stream := httptest.NewRecorder()
	handler.ServeHTTP(stream, httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID+"/events", nil))
	if stream.Code != http.StatusOK || !strings.Contains(stream.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream = %d %#v", stream.Code, stream.Header())
	}
	for _, required := range []string{"event: dispatch", "Reading package", "Run completed"} {
		if !strings.Contains(stream.Body.String(), required) {
			t.Fatalf("stream omitted %q: %s", required, stream.Body.String())
		}
	}
	for _, required := range []string{`"component":"Workspace structure"`, `"progressState":"completed"`, `"current":1`, `"total":1`} {
		if !strings.Contains(stream.Body.String(), required) {
			t.Fatalf("stream omitted structured progress %q: %s", required, stream.Body.String())
		}
	}
	if strings.Contains(stream.Body.String(), "raw provider diagnostic") {
		t.Fatalf("stream leaked raw diagnostic: %s", stream.Body.String())
	}
	if strings.Contains(stream.Body.String(), `\n`) {
		t.Fatalf("stream contains literal newline escapes instead of SSE boundaries: %q", stream.Body.String())
	}

	deployResponse := httptest.NewRecorder()
	handler.ServeHTTP(deployResponse, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/deploy", nil))
	if deployResponse.Code != http.StatusAccepted {
		t.Fatalf("deploy status = %d: %s", deployResponse.Code, deployResponse.Body.String())
	}
}

func TestValidationChangesEndpointReturnsFileByFilePreviews(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return []lifecycle.Item{{
			Provider: "salesforce", Status: lifecycle.StatusMissing,
			Changes: []salesforceapi.MetadataFileDiff{{Component: "Settings:Communities", Path: "settings/Communities.settings-meta.xml", Kind: "update", Before: "<enabled>false</enabled>", After: "<enabled>true</enabled>", Previewable: true}},
		}}, nil
	}
	runs := newRunManagerWithExecutors(validate, nil, time.Now)
	run, err := runs.startValidation(config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"salesforce"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, runs, run.ID)
	handler := NewHandlerWithOptions(ServerOptions{Runs: runs})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID+"/changes", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"component":"Settings:Communities"`) || !strings.Contains(response.Body.String(), `"previewable":true`) {
		t.Fatalf("changes = %d: %s", response.Code, response.Body.String())
	}
}

func TestDeploymentRequiresSuccessfulValidation(t *testing.T) {
	runs := newRunManagerWithExecutors(nil, nil, time.Now)
	if _, err := runs.startDeployment("unknown"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidationFailsWhenProviderReturnsFailedItem(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return []lifecycle.Item{{
			Provider: "salesforce", Status: lifecycle.StatusFailed,
			Detail:     "Unable to read Salesforce metadata: ERROR_HTTP_420",
			Diagnostic: `{"message":"Session unavailable","instance":"https://example.my.salesforce.com?sid=secret"}`,
		}}, nil
	}
	runs := newRunManagerWithExecutors(validate, nil, time.Now)
	run, err := runs.startValidation(config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"salesforce"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, runs, run.ID)

	response, ok := runs.response(run.ID)
	if !ok || response.Status != runFailed {
		t.Fatalf("response = %#v, found=%t", response, ok)
	}
	if len(response.Providers) != 1 || response.Providers[0].Status != string(lifecycle.StatusFailed) {
		t.Fatalf("providers = %#v", response.Providers)
	}
	diagnostic, ok := runs.diagnostics(run.ID)
	if !ok || !strings.Contains(diagnostic.Summary, "Salesforce could not be reached") {
		t.Fatalf("diagnostic = %#v, found=%t", diagnostic, ok)
	}
	if diagnostic.Provider != "Salesforce" || diagnostic.Code != "SALESFORCE_ORG_UNREACHABLE" || !strings.Contains(diagnostic.TechnicalDetail, "Session unavailable") {
		t.Fatalf("diagnostic did not preserve safe provider context: %#v", diagnostic)
	}
	if _, err := runs.startDeployment(run.ID); err == nil || !strings.Contains(err.Error(), "successful validation") {
		t.Fatalf("deploy error = %v", err)
	}
}

func TestEmitDeploymentResultDoesNotReportFailedProviderAsApplied(t *testing.T) {
	var provider string
	var update lifecycle.ProgressUpdate
	emitDeploymentResult(lifecycle.Item{
		Provider: "salesforce",
		Status:   lifecycle.StatusFailed,
		Detail:   "managed-package install request was rejected",
	}, func(actualProvider string, actualUpdate lifecycle.ProgressUpdate) {
		provider, update = actualProvider, actualUpdate
	})

	if provider != "salesforce" || update.State != lifecycle.ProgressFailed {
		t.Fatalf("provider=%q update=%#v", provider, update)
	}
	if !strings.Contains(update.Message, "configuration failed") || strings.Contains(update.Message, "configuration applied") {
		t.Fatalf("message = %q", update.Message)
	}
}

func TestRunningDeploymentDoesNotReuseValidationSuccessAsProviderCompletion(t *testing.T) {
	run := &deploymentRun{
		action: runActionDeploy,
		status: runRunning,
		items: []lifecycle.Item{
			{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true},
			{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true},
		},
		events: []runEvent{
			{Provider: "box", ProgressState: lifecycle.ProgressCompleted, Message: "Box configuration applied"},
			{Provider: "salesforce", ProgressState: lifecycle.ProgressRunning, Message: "Applying Salesforce configuration"},
			{Provider: "salesforce", Component: "Managed Package:Box for Salesforce 5.43", ProgressState: lifecycle.ProgressRunning, Message: "Salesforce reports in progress"},
		},
	}

	response := summarizeRun(run)
	if len(response.Providers) != 2 || response.Providers[0].Status != string(lifecycle.StatusPresent) || response.Providers[1].Status != string(lifecycle.StatusPending) {
		t.Fatalf("providers = %#v", response.Providers)
	}
}

func TestSummarizeRunIncludesDeployedResources(t *testing.T) {
	run := &deploymentRun{
		action: runActionDeploy,
		status: runCompleted,
		items: []lifecycle.Item{{
			Provider: "salesforce",
			Status:   lifecycle.StatusPresent,
			Resources: []lifecycle.ResourceReference{{
				Provider:  "salesforce",
				Component: "Salesforce Experience",
				Kind:      "experience_site",
				Name:      "CLM Experience",
				ID:        "0DB1",
				URL:       "https://example.my.site.com/clm",
			}},
		}},
	}

	response := summarizeRun(run)
	if len(response.Resources) != 1 || response.Resources[0].Kind != "experience_site" || response.Resources[0].URL != "https://example.my.site.com/clm" {
		t.Fatalf("resources = %#v", response.Resources)
	}
}

func TestRunHistorySurvivesRestartAndInterruptedRunIsMarkedFailed(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC) }
	store := &testRunStore{runs: []persistedRun{{
		ID: "web-previous-0001", Action: runActionValidate, Status: runRunning,
		CreatedAt: now(), Events: []runEvent{{Sequence: 1, At: now(), Type: "status", Message: "Run started", Status: runRunning}},
	}}}
	runs := newRunManagerWithStore(nil, nil, now, store)
	history := runs.history()
	if len(history) != 1 || history[0].Status != runFailed {
		t.Fatalf("history = %#v", history)
	}
	diagnostic, ok := runs.diagnostics("web-previous-0001")
	if !ok || !strings.Contains(diagnostic.Summary, "stopped") {
		t.Fatalf("diagnostic = %#v, found=%t", diagnostic, ok)
	}
	if len(store.runs) != 1 || store.runs[0].Status != runFailed {
		t.Fatalf("persisted runs = %#v", store.runs)
	}
}

func TestRestoredValidationCannotApplyWithoutItsLivePackage(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC) }
	runs := newRunManagerWithStore(nil, nil, now, &testRunStore{runs: []persistedRun{{
		ID: "web-previous-0002", Action: runActionValidate, Status: runCompleted, CreatedAt: now(),
	}}})
	if _, err := runs.startDeployment("web-previous-0002"); err == nil || !strings.Contains(err.Error(), "rerun validation") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompletedValidationRestoresItsPlanItemsAndChangePreviews(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC) }
	plan := config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"salesforce"}}
	items := []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusMissing, Changes: []salesforceapi.MetadataFileDiff{{Path: "settings/Communities.settings-meta.xml", Previewable: true}}}}
	runs := newRunManagerWithStore(nil, nil, now, &testRunStore{runs: []persistedRun{{
		ID: "web-previous-0003", Action: runActionValidate, Status: runCompleted, CreatedAt: now(), Plan: plan, Items: items,
	}}})
	changes, ok := runs.changes("web-previous-0003")
	if !ok || len(changes.Files) != 1 || changes.Files[0].Path != "settings/Communities.settings-meta.xml" {
		t.Fatalf("changes = %#v, found=%t", changes, ok)
	}
	runs.deploy = func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return items, nil
	}
	if _, err := runs.startDeployment("web-previous-0003"); err != nil {
		t.Fatalf("restored validation could not start deployment: %v", err)
	}
}

func TestDiagnosticEndpointProvidesSafeGuidance(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return nil, errors.New("ERROR_HTTP_420 secret-adjacent response at /private/path")
	}
	runs := newRunManagerWithExecutors(validate, nil, time.Now)
	run, err := runs.startValidation(config.SolutionPlan{TemplateID: "clm", PackagePath: "/package", Components: []string{"salesforce"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, runs, run.ID)
	handler := NewHandlerWithOptions(ServerOptions{Runs: runs})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID+"/diagnostics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, forbidden := range []string{"secret-adjacent", "/private/path", "ERROR_HTTP_420"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "Salesforce could not be reached") {
		t.Fatalf("diagnostic did not include actionable guidance: %s", body)
	}
}

func TestSafeDiagnosticExplainsMissingScratchOrg(t *testing.T) {
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider: "salesforce",
		Detail:   "Unable to inspect Salesforce org box-dispatch-scratch.",
		Diagnostic: `{
			"code":"NoScratchInfo",
			"message":"No information for scratch org with ID 00D000000000000 found in Dev Hub devhub@example.com."
		}`,
	})

	if diagnostic.Provider != "Salesforce" || diagnostic.Code != "SALESFORCE_SCRATCH_ORG_UNAVAILABLE" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if !strings.Contains(diagnostic.Summary, "no longer available") || len(diagnostic.NextSteps) != 3 {
		t.Fatalf("diagnostic was not actionable: %#v", diagnostic)
	}
	if strings.Contains(strings.ToLower(strings.Join(diagnostic.NextSteps, " ")), "cli") {
		t.Fatalf("diagnostic requires CLI knowledge: %#v", diagnostic.NextSteps)
	}
}

func TestSafeDiagnosticExplainsInvalidSalesforceSessionWithoutCallingItExpiredOrg(t *testing.T) {
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider: "salesforce",
		Detail:   "Unable to inspect installed Salesforce packages: INVALID_SESSION_ID: Session expired or invalid",
	})
	if diagnostic.Code != "SALESFORCE_SESSION_EXPIRED" || !strings.Contains(diagnostic.Summary, "session") {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if strings.Contains(strings.ToLower(diagnostic.Summary), "org is expired") {
		t.Fatalf("session failure was misclassified as an expired org: %#v", diagnostic)
	}
}

func TestSafeDiagnosticDoesNotRepeatProviderDetailInsideDiagnostic(t *testing.T) {
	providerDetail := "Salesforce did not return exactly one deployment user for admin@example.com"
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider:   "salesforce",
		Detail:     "Unable to inspect Salesforce permission-set assignments: " + providerDetail,
		Diagnostic: providerDetail,
	})
	if strings.Count(diagnostic.TechnicalDetail, providerDetail) != 1 {
		t.Fatalf("technical detail repeated the provider error: %q", diagnostic.TechnicalDetail)
	}
}

func TestSafeDiagnosticExplainsExpiredBoxOAuthSession(t *testing.T) {
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider: "box",
		Detail:   "Box OAuth session has expired. Return to Connect and reconnect the selected Box account: invalid_grant: Refresh token has expired",
	})
	if diagnostic.Provider != "Box" || diagnostic.Code != "BOX_OAUTH_SESSION_EXPIRED" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if !strings.Contains(strings.ToLower(diagnostic.Summary), "no longer authorized") || len(diagnostic.NextSteps) != 2 {
		t.Fatalf("diagnostic was not actionable: %#v", diagnostic)
	}
}

func TestSafeDiagnosticExplainsSalesforceMetadataTimeout(t *testing.T) {
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider: "salesforce",
		Detail:   `Unable to read Salesforce metadata: Post "https://example.my.salesforce.com/services/Soap/m/67.0": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
	})
	if diagnostic.Provider != "Salesforce" || diagnostic.Code != "SALESFORCE_METADATA_TIMEOUT" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if !strings.Contains(strings.ToLower(diagnostic.Summary), "metadata inventory") || len(diagnostic.NextSteps) != 2 {
		t.Fatalf("diagnostic was not actionable: %#v", diagnostic)
	}
}

func TestSafeDiagnosticExplainsIncompleteSalesforceMetadataResponse(t *testing.T) {
	diagnostic := safeDiagnostic(runActionValidate, &providerRunFailure{
		Provider: "salesforce",
		Detail:   "Unable to read Salesforce metadata: parse Salesforce metadata inventory: XML syntax error on line 1: unexpected EOF",
	})
	if diagnostic.Provider != "Salesforce" || diagnostic.Code != "SALESFORCE_METADATA_RESPONSE_INCOMPLETE" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if !strings.Contains(strings.ToLower(diagnostic.Summary), "before it was complete") || len(diagnostic.NextSteps) != 2 {
		t.Fatalf("diagnostic was not actionable: %#v", diagnostic)
	}
}

type testRunStore struct {
	runs []persistedRun
}

func (s *testRunStore) Load() ([]persistedRun, error) {
	return append([]persistedRun(nil), s.runs...), nil
}

func (s *testRunStore) Save(runs []persistedRun) error {
	s.runs = append([]persistedRun(nil), runs...)
	return nil
}

func waitForRun(t *testing.T, runs *runManager, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		run, ok := runs.response(id)
		if ok && (run.Status == runCompleted || run.Status == runFailed) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("run %s did not finish", id)
}
