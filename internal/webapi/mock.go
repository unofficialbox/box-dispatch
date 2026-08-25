package webapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

// NewMockHandler returns the browser API backed entirely by deterministic
// in-memory state. It exercises the same HTTP and SSE contract as the live API
// without reading credentials, cloning packages, or calling providers.
func NewMockHandler() http.Handler {
	var mu sync.Mutex
	plan := config.SolutionPlan{}
	settings := mockConnectionSettings()
	deployments := []audit.DeploymentRecord{}
	validate := func(ctx context.Context, plan config.SolutionPlan, _ []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		items := make([]lifecycle.Item, 0, len(plan.Components))
		for _, provider := range plan.Components {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			component := mockProviderComponent(provider)
			emit(provider, lifecycle.ProgressUpdate{Message: "Inspecting mock " + provider + " configuration", State: lifecycle.ProgressRunning})
			emit(provider, lifecycle.ProgressUpdate{Message: "Ready to deploy", Component: component, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
			emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " validation complete", State: lifecycle.ProgressCompleted})
			items = append(items, lifecycle.Item{
				Provider: provider, Name: providerName(provider), Status: lifecycle.StatusMissing,
				Detail: "Mock validation found one deployable change.", Deployable: true,
				Missing: []string{component}, DeployableComponents: []string{component}, Planned: []string{component},
			})
			time.Sleep(30 * time.Millisecond)
		}
		return items, nil
	}
	deploy := func(ctx context.Context, plan config.SolutionPlan, items []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		startedAt := time.Now().UTC()
		result := append([]lifecycle.Item(nil), items...)
		for index := range result {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			provider := result[index].Provider
			component := mockProviderComponent(provider)
			emit(provider, lifecycle.ProgressUpdate{Message: "Applying mock " + provider + " configuration", State: lifecycle.ProgressRunning})
			emit(provider, lifecycle.ProgressUpdate{Message: "Deployed in mock backend", Component: component, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
			emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " configuration applied", State: lifecycle.ProgressCompleted})
			result[index].Status = lifecycle.StatusPresent
			result[index].Detail = providerName(provider) + " mock configuration applied successfully."
			result[index].Present = []string{component}
			result[index].Missing = nil
			result[index].DeployableComponents = nil
			result[index].Deployable = false
			time.Sleep(30 * time.Millisecond)
		}
		completedAt := time.Now().UTC()
		mu.Lock()
		deployments = append([]audit.DeploymentRecord{{
			DeploymentID: completedAt.Format("20060102T150405Z"), TemplateID: plan.TemplateID,
			Strategy: plan.Strategy, StartedAt: startedAt, CompletedAt: completedAt,
			Duration:  completedAt.Sub(startedAt).Round(time.Millisecond).String(),
			Providers: mockProviderRecords(result),
		}}, deployments...)
		mu.Unlock()
		return result, nil
	}
	runs := newRunManagerWithExecutors(validate, deploy, time.Now)
	return NewHandlerWithOptions(ServerOptions{
		Profile: "mock",
		PlanStore: func() (config.SolutionPlan, error) {
			mu.Lock()
			defer mu.Unlock()
			return plan, nil
		},
		PlanSaver: func(next config.SolutionPlan) error {
			mu.Lock()
			plan = next
			mu.Unlock()
			return nil
		},
		Templates: func() ([]packageTemplate, error) { return defaultPackageTemplates(), nil },
		PackageAssembler: func(template packageTemplate, components []string, strategy string) (config.SolutionPlan, error) {
			assembled := config.SolutionPlan{
				TemplateID: template.ID, Template: template.Name, Repository: template.repository,
				Components: append([]string(nil), components...), Strategy: strategy, PackagePath: "/mock/dispatch-package",
			}
			mu.Lock()
			plan = assembled
			mu.Unlock()
			return assembled, nil
		},
		ConnectionStore: func() (config.ConnectionSettings, error) {
			mu.Lock()
			defer mu.Unlock()
			return settings, nil
		},
		ConnectionSaver: func(next config.ConnectionSettings) error {
			mu.Lock()
			settings = next
			mu.Unlock()
			return nil
		},
		BoxCheck: func(context.Context, config.ConnectionSettings) (config.VerifiedConnection, error) {
			mu.Lock()
			defer mu.Unlock()
			return settings.VerifiedConnections["box"], nil
		},
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{Available: true, OrgID: "00D-mock", Username: "admin@example.test", Status: "Ready"}, nil
		},
		DeploymentStore: func() ([]audit.DeploymentRecord, error) {
			mu.Lock()
			defer mu.Unlock()
			return append([]audit.DeploymentRecord(nil), deployments...), nil
		},
		Runs: runs,
	})
}

func mockConnectionSettings() config.ConnectionSettings {
	verifiedAt := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	settings := config.ConnectionSettings{
		SalesforceAlias:       "admin@example.test",
		SalesforceInstanceURL: "https://example.my.salesforce.com",
		SalesforceAccessToken: "mock-access",
		BoxCCGClientID:        "mock-client",
		BoxCCGClientSecret:    "mock-secret",
		BoxCCGSubjectType:     "user",
		BoxCCGSubjectID:       "123456",
		BoxCCGAlias:           "box.user@example.test",
		BoxDefaultConnection:  "box-dispatch-ccg",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"box":        {VerifiedAt: verifiedAt, Selection: "box-dispatch-ccg", Identity: "box.user@example.test", Account: "123456", AuthType: "CCG"},
			"salesforce": {VerifiedAt: verifiedAt, Selection: "salesforce-mock", Identity: "admin@example.test", OrgID: "00D-mock", OrgStatus: "Ready", OrgType: "persistent", AuthType: "Salesforce OAuth"},
		},
	}
	return settings
}

func mockProviderComponent(provider string) string {
	if provider == "salesforce" {
		return "UIBundle:clmreactapp"
	}
	return "Sample Content:northstar-msa-redline-v3.pdf"
}

func mockProviderRecords(items []lifecycle.Item) []audit.ProviderRecord {
	records := make([]audit.ProviderRecord, 0, len(items))
	for _, item := range items {
		records = append(records, audit.ProviderRecord{
			Provider: item.Provider, StatusBefore: lifecycle.StatusMissing, StatusAfter: item.Status,
			Detail: item.Detail, Deployed: append([]string(nil), item.Present...), PresentAfter: append([]string(nil), item.Present...),
		})
	}
	return records
}
