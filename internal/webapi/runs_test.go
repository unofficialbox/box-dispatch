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
)

func TestValidationRunStreamsSafeProgressAndCompletes(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, emit func(string, string)) ([]lifecycle.Item, error) {
		emit("box", "Inspecting Box configuration")
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

func TestRunEndpointsProvideSSEAndRequireCompletedValidationForDeploy(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, emit func(string, string)) ([]lifecycle.Item, error) {
		emit("box", "Reading package")
		return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent}}, nil
	}
	deploy := func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, emit func(string, string)) ([]lifecycle.Item, error) {
		emit("box", "No changes needed")
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

func TestDeploymentRequiresSuccessfulValidation(t *testing.T) {
	runs := newRunManagerWithExecutors(nil, nil, time.Now)
	if _, err := runs.startDeployment("unknown"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidationFailsWhenProviderReturnsFailedItem(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, _ func(string, string)) ([]lifecycle.Item, error) {
		return []lifecycle.Item{{
			Provider: "salesforce", Status: lifecycle.StatusFailed,
			Detail: "Unable to read Salesforce metadata: ERROR_HTTP_420",
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
	if _, err := runs.startDeployment(run.ID); err == nil || !strings.Contains(err.Error(), "successful validation") {
		t.Fatalf("deploy error = %v", err)
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

func TestDiagnosticEndpointProvidesSafeGuidance(t *testing.T) {
	validate := func(_ context.Context, _ config.SolutionPlan, _ []lifecycle.Item, _ func(string, string)) ([]lifecycle.Item, error) {
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
