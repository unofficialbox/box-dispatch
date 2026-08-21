package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
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
