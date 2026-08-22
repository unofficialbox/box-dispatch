package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
)

func TestDeploymentsExposeSafeRunSummaries(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		Profile: "demo",
		DeploymentStore: func() ([]audit.DeploymentRecord, error) {
			return []audit.DeploymentRecord{{
				DeploymentID: "run-42", TemplateID: "clm", Strategy: "reuse-existing",
				SourcePath: "/private/audit.json", PackageRoot: "/private/package",
				CompletedAt: time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC),
				Providers:   []audit.ProviderRecord{{Provider: "salesforce", StatusAfter: lifecycle.StatusPresent, Detail: "raw CLI output"}},
			}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"/private", "raw CLI output", "SourcePath", "PackageRoot"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var summaries []deploymentSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "run-42" || summaries[0].Providers[0].Status != string(lifecycle.StatusPresent) {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestSalesforceRESTConnectionCheckAndScratchCreation(t *testing.T) {
	settings := config.ConnectionSettings{}
	created := make(chan struct{}, 1)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.InstanceURL != "https://scratch.example.com" || credential.AccessToken != "scratch-token" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dscratch", Username: "scratch@example.com", Status: "Active"}, nil
		},
		SalesforceCreate: func(_ context.Context, credential salesforceapi.Credential, request salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
			if credential.InstanceURL != "https://devhub.example.com" || credential.ClientID != "client-id" || request.Alias != "replacement" {
				t.Fatalf("credential=%#v request=%#v", credential, request)
			}
			created <- struct{}{}
			return salesforceapi.ScratchOrg{Alias: "replacement", Username: "new@example.com", OrgID: "00Dnew", InstanceURL: "https://new.example.com", AccessToken: "new-token", Status: "Active", ExpirationDate: "2026-09-21"}, nil
		},
	})

	connect := httptest.NewRecorder()
	handler.ServeHTTP(connect, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce/rest", strings.NewReader(`{"instanceUrl":"https://scratch.example.com","accessToken":"scratch-token","devHubUrl":"https://devhub.example.com","devHubAccessToken":"hub-token","clientId":"client-id","clientSecret":"secret"}`)))
	if connect.Code != http.StatusOK || strings.Contains(connect.Body.String(), "scratch-token") || strings.Contains(connect.Body.String(), "secret") {
		t.Fatalf("connect = %d %s", connect.Code, connect.Body.String())
	}

	check := httptest.NewRecorder()
	handler.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"available":true`) || settings.SalesforceOrgID != "00Dscratch" {
		t.Fatalf("check = %d %s settings=%#v", check.Code, check.Body.String(), settings)
	}

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/salesforce/scratch-orgs", strings.NewReader(`{"alias":"replacement","durationDays":30}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	var job scratchJob
	if err := json.Unmarshal(create.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-created:
	case <-time.After(time.Second):
		t.Fatal("scratch creation did not start")
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		status := httptest.NewRecorder()
		handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/salesforce/scratch-orgs/"+job.ID, nil))
		if strings.Contains(status.Body.String(), `"status":"active"`) {
			if settings.SalesforceAlias != "replacement" || settings.SalesforceAccessToken != "new-token" {
				t.Fatalf("settings = %#v", settings)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scratch creation did not complete")
}

func TestSalesforceRESTCheckRejectsExpiredScratchOrgBeforeCallingProvider(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceInstanceURL:    "https://expired.example.com",
		SalesforceAccessToken:    "expired-token",
		SalesforceOrgType:        "scratch",
		SalesforceExpirationDate: "2026-08-21",
	}
	called := false
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			called = true
			return salesforceapi.OrgStatus{}, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusGone || called || settings.SalesforceOrgStatus != "Expired" {
		t.Fatalf("status=%d called=%t settings=%#v body=%s", response.Code, called, settings, response.Body.String())
	}
}

func TestDeploymentDetailCountsWithoutDiagnostics(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		DeploymentStore: func() ([]audit.DeploymentRecord, error) {
			return []audit.DeploymentRecord{{
				DeploymentID: "run-42", SourcePath: "/private/audit.json", PackageRoot: "/private/package",
				Providers: []audit.ProviderRecord{{
					Provider: "box", StatusAfter: lifecycle.StatusMissing, Detail: "secret-adjacent diagnostic",
					Deployed: []string{"one"}, PresentAfter: []string{"one", "two"}, Remaining: []string{"three"},
					AdapterPending: []string{"four"}, Experimental: []string{"five"},
				}},
			}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments/run-42", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"/private", "secret-adjacent", "AdapterPending"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var detail deploymentDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	provider := detail.Providers[0]
	if provider.DeployedCount != 1 || provider.PresentCount != 2 || provider.RemainingCount != 1 || provider.ManualItemCount != 2 {
		t.Fatalf("provider detail = %#v", provider)
	}
}

func TestConnectionsRedactCredentials(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) {
			return config.ConnectionSettings{
				SalesforceAlias: "scratch-org", SalesforceOrgStatus: "Active", SalesforceExpirationDate: "2026-09-20",
				BoxCCGClientID: "client-id", BoxCCGClientSecret: "private-secret", BoxCCGSubjectType: "enterprise", BoxCCGSubjectID: "123",
				VerifiedConnections: map[string]config.VerifiedConnection{"box": {VerifiedAt: "2026-08-21", Selection: "ccg", Identity: "operator@example.com"}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"private-secret", "client-id", "operator@example.com", "123"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "client credentials") || !strings.Contains(body, "scratch-org") {
		t.Fatalf("response omitted safe connection state: %s", body)
	}
}

func TestBoxConnectionStoresCCGWithoutReturningSecrets(t *testing.T) {
	var settings config.ConnectionSettings
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/box", strings.NewReader(`{"clientId":"client-id","clientSecret":"very-secret","subjectType":"enterprise","subjectId":"12345"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if settings.BoxCCGClientID != "client-id" || settings.BoxCCGClientSecret != "very-secret" || settings.BoxCCGSubjectType != "enterprise" || settings.BoxCCGSubjectID != "12345" {
		t.Fatalf("settings = %#v", settings)
	}
	if strings.Contains(response.Body.String(), "very-secret") || strings.Contains(response.Body.String(), "client-id") || strings.Contains(response.Body.String(), "12345") {
		t.Fatalf("secret material leaked: %s", response.Body.String())
	}
}

func TestSalesforceConnectionSelectionOnlyAcceptsAuthenticatedAlias(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceAlias: "old-org", SalesforceOrgID: "00Dold",
		VerifiedConnections: map[string]config.VerifiedConnection{"salesforce": {VerifiedAt: "2026-08-21", Selection: "old-org"}},
	}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceTargets: func() ([]salesforceorg.Target, error) {
			return []salesforceorg.Target{
				{Alias: "dispatch-scratch", ConnectedStatus: "Connected", Status: "Active", ExpirationDate: "2026-09-15", DevHubID: "00Dhub"},
				{Alias: "disconnected", ConnectedStatus: "Disconnected"},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/options", nil))
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), "dispatch-scratch") || strings.Contains(options.Body.String(), "disconnected") {
		t.Fatalf("options = %d: %s", options.Code, options.Body.String())
	}

	selectRequest := httptest.NewRequest(http.MethodPut, "/api/connections/salesforce", strings.NewReader(`{"alias":"dispatch-scratch"}`))
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, selectRequest)
	if selected.Code != http.StatusOK {
		t.Fatalf("selection = %d: %s", selected.Code, selected.Body.String())
	}
	if settings.SalesforceAlias != "dispatch-scratch" || settings.SalesforceOrgID != "" || settings.SalesforceOrgType != "scratch" {
		t.Fatalf("saved settings = %#v", settings)
	}
	if _, found := settings.VerifiedConnections["salesforce"]; found {
		t.Fatalf("selection retained stale verification: %#v", settings.VerifiedConnections)
	}

	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce", strings.NewReader(`{"alias":"unknown"}`)))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("rejected selection = %d: %s", rejected.Code, rejected.Body.String())
	}
}

func TestHealthReturnsProfile(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{Profile: "demo", Now: func() time.Time { return time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC) }})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"profile":"demo"`) {
		t.Fatalf("health = %d %s", response.Code, response.Body.String())
	}
}

func TestPlanExposesProviderReadinessWithoutPackagePath(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{TemplateID: "clm", Template: "Contract Lifecycle Management", Repository: "https://example.test/clm", PackagePath: "/private/package", Components: []string{"box", "salesforce"}}, nil
		},
		ConnectionStore: func() (config.ConnectionSettings, error) {
			return config.ConnectionSettings{
				SalesforceAlias: "scratch-org",
				BoxCCGClientID:  "id", BoxCCGClientSecret: "secret", BoxCCGSubjectType: "enterprise", BoxCCGSubjectID: "123",
				VerifiedConnections: map[string]config.VerifiedConnection{"box": {VerifiedAt: "2026-08-21"}, "salesforce": {VerifiedAt: "2026-08-21"}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if strings.Contains(response.Body.String(), "/private/package") {
		t.Fatalf("plan response exposed package path: %s", response.Body.String())
	}
	var plan planResponse
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Exists || len(plan.Components) != 2 || !plan.Components[0].Ready || !plan.Components[1].Ready {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanUpdatePersistsOnlySupportedDraftFields(t *testing.T) {
	var saved config.SolutionPlan
	handler := NewHandlerWithOptions(ServerOptions{
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{PackagePath: "/kept-on-server"}, nil
		},
		PlanSaver:       func(plan config.SolutionPlan) error { saved = plan; return nil },
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})

	req := httptest.NewRequest(http.MethodPut, "/api/plan", strings.NewReader(`{"templateId":" clm ","template":" Contract Lifecycle Management ","repository":" https://example.test/clm ","components":[" BOX ","salesforce"]}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if saved.PackagePath != "/kept-on-server" || saved.TemplateID != "clm" || saved.Template != "Contract Lifecycle Management" || saved.Repository != "https://example.test/clm" || strings.Join(saved.Components, ",") != "box,salesforce" {
		t.Fatalf("saved = %#v", saved)
	}
}

func TestPlanUpdateRejectsUnsupportedProvider(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{PlanStore: func() (config.SolutionPlan, error) { return config.SolutionPlan{}, nil }})
	req := httptest.NewRequest(http.MethodPut, "/api/plan", strings.NewReader(`{"templateId":"clm","template":"CLM","repository":"https://example.test/clm","components":["box","databricks"]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "not available") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestPackageAssemblyUsesConfiguredTemplateAndKeepsWorkspacePrivate(t *testing.T) {
	var saved config.SolutionPlan
	var assembledTemplate packageTemplate
	var assembledComponents []string
	handler := NewHandlerWithOptions(ServerOptions{
		Templates: func() ([]packageTemplate, error) {
			return []packageTemplate{{ID: "clm", Name: "Contract Lifecycle Management", Description: "Contracts", repository: "https://example.test/clm"}}, nil
		},
		PackageAssembler: func(template packageTemplate, components []string, strategy string) (config.SolutionPlan, error) {
			assembledTemplate = template
			assembledComponents = append([]string(nil), components...)
			return config.SolutionPlan{TemplateID: template.ID, Template: template.Name, Repository: template.repository, Components: components, Strategy: strategy, PackagePath: "/private/web-package"}, nil
		},
		PlanSaver:       func(plan config.SolutionPlan) error { saved = plan; return nil },
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})

	request := httptest.NewRequest(http.MethodPost, "/api/packages", strings.NewReader(`{"templateId":"clm","components":["box","salesforce"]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if assembledTemplate.ID != "clm" || assembledTemplate.repository != "https://example.test/clm" || strings.Join(assembledComponents, ",") != "box,salesforce" {
		t.Fatalf("assembly = %#v %#v", assembledTemplate, assembledComponents)
	}
	if saved.PackagePath != "/private/web-package" {
		t.Fatalf("saved package path = %q", saved.PackagePath)
	}
	for _, forbidden := range []string{"/private/web-package"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestPackageAssemblyRejectsUnknownTemplateAndUnsupportedProviders(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		Templates: func() ([]packageTemplate, error) {
			return []packageTemplate{{ID: "clm", Name: "CLM", repository: "https://example.test/clm"}}, nil
		},
	})
	for _, body := range []string{
		`{"templateId":"unknown","components":["box"]}`,
		`{"templateId":"clm","components":["box","databricks"]}`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/packages", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400: %s", body, response.Code, response.Body.String())
		}
	}
}
