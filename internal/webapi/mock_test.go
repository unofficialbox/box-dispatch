package webapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMockHandlerRunsCompleteValidationAndDeploymentWithoutProviders(t *testing.T) {
	server := httptest.NewServer(NewMockHandler())
	defer server.Close()
	connections, err := http.Get(server.URL + "/api/connections")
	if err != nil {
		t.Fatal(err)
	}
	connectionBody, _ := io.ReadAll(connections.Body)
	_ = connections.Body.Close()
	if connections.StatusCode != http.StatusOK || strings.Count(string(connectionBody), `"verified":true`) != 2 {
		t.Fatalf("connections = %d: %s", connections.StatusCode, connectionBody)
	}
	for _, forbidden := range []string{"mock-access", "mock-refresh"} {
		if strings.Contains(string(connectionBody), forbidden) {
			t.Fatalf("mock response leaked %q: %s", forbidden, connectionBody)
		}
	}
	packageResponse, err := http.Post(server.URL+"/api/packages", "application/json", bytes.NewBufferString(`{"name":"Northstar CLM rollout","templateId":"clm","components":["box","salesforce"],"strategy":"reuse"}`))
	if err != nil {
		t.Fatal(err)
	}
	packageBody, _ := io.ReadAll(packageResponse.Body)
	_ = packageResponse.Body.Close()
	if packageResponse.StatusCode != http.StatusCreated || !strings.Contains(string(packageBody), `"exists":true`) {
		t.Fatalf("package = %d: %s", packageResponse.StatusCode, packageBody)
	}
	validation := startMockRun(t, server.URL+"/api/runs")
	waitForMockRun(t, server.URL, validation.ID)
	events, err := http.Get(server.URL + "/api/runs/" + validation.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	eventBody, _ := io.ReadAll(events.Body)
	_ = events.Body.Close()
	for _, expected := range []string{"Box validation complete", "Salesforce validation complete", `"status":"completed"`} {
		if !strings.Contains(string(eventBody), expected) {
			t.Fatalf("validation events omitted %q: %s", expected, eventBody)
		}
	}
	deployment := startMockRun(t, server.URL+"/api/runs/"+validation.ID+"/deploy")
	completed := waitForMockRun(t, server.URL, deployment.ID)
	if completed.Status != runCompleted || len(completed.Providers) != 2 {
		t.Fatalf("deployment = %#v", completed)
	}
	deploymentEvents, err := http.Get(server.URL + "/api/runs/" + deployment.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	deploymentEventBody, _ := io.ReadAll(deploymentEvents.Body)
	_ = deploymentEvents.Body.Close()
	for _, expected := range []string{"Salesforce org is available", "Salesforce reports queued", "Salesforce reports in progress", "Managed package installed", "Salesforce configuration applied"} {
		if !strings.Contains(string(deploymentEventBody), expected) {
			t.Fatalf("deployment events omitted %q: %s", expected, deploymentEventBody)
		}
	}
}

func TestMockHandlerCanSimulateAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(NewMockHandlerWithOptions(MockOptions{ValidationFailureProvider: "salesforce"}))
	defer server.Close()
	packageResponse, err := http.Post(server.URL+"/api/packages", "application/json", bytes.NewBufferString(`{"name":"Northstar CLM rollout","templateId":"clm","components":["box","salesforce"],"strategy":"reuse"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = packageResponse.Body.Close()

	validation := startMockRun(t, server.URL+"/api/runs")
	failed := waitForMockRun(t, server.URL, validation.ID)
	if failed.Status != runFailed || len(failed.Providers) != 1 || failed.Providers[0].Name != "salesforce" {
		t.Fatalf("validation = %#v", failed)
	}
	events, err := http.Get(server.URL + "/api/runs/" + validation.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	eventBody, _ := io.ReadAll(events.Body)
	_ = events.Body.Close()
	if !strings.Contains(string(eventBody), `"component":"Authentication"`) || !strings.Contains(string(eventBody), `"progressState":"failed"`) || strings.Contains(string(eventBody), "Inspecting mock box configuration") {
		t.Fatalf("authentication failure did not stop configuration checks: %s", eventBody)
	}
	diagnostic, err := http.Get(server.URL + "/api/runs/" + validation.ID + "/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	diagnosticBody, _ := io.ReadAll(diagnostic.Body)
	_ = diagnostic.Body.Close()
	if diagnostic.StatusCode != http.StatusOK || !strings.Contains(string(diagnosticBody), `"code":"SALESFORCE_SESSION_EXPIRED"`) || !strings.Contains(string(diagnosticBody), "reconnect the selected Salesforce org") {
		t.Fatalf("authentication diagnostic = %d: %s", diagnostic.StatusCode, diagnosticBody)
	}
}

func TestMockHandlerCanSimulateStaleConnectionBeforeValidation(t *testing.T) {
	server := httptest.NewServer(NewMockHandlerWithOptions(MockOptions{ConnectionFailureProvider: "salesforce"}))
	defer server.Close()

	response, err := http.Post(server.URL+"/api/connections/salesforce/check", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "reconnect the selected Salesforce org") {
		t.Fatalf("check = %d: %s", response.StatusCode, body)
	}

	connections, err := http.Get(server.URL + "/api/connections")
	if err != nil {
		t.Fatal(err)
	}
	connectionBody, _ := io.ReadAll(connections.Body)
	_ = connections.Body.Close()
	if connections.StatusCode != http.StatusOK || strings.Count(string(connectionBody), `"verified":true`) != 1 || !strings.Contains(string(connectionBody), `"name":"Salesforce","configured":true,"verified":false`) {
		t.Fatalf("connections = %d: %s", connections.StatusCode, connectionBody)
	}
}

func startMockRun(t *testing.T, url string) runResponse {
	t.Helper()
	response, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start status = %d: %s", response.StatusCode, body)
	}
	var run runResponse
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	return run
}

func waitForMockRun(t *testing.T, baseURL, id string) runResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/api/runs/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var run runResponse
		decodeErr := json.NewDecoder(response.Body).Decode(&run)
		_ = response.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if run.Status == runCompleted || run.Status == runFailed {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", id)
	return runResponse{}
}
