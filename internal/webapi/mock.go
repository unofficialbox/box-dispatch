package webapi

import (
	"context"
	"errors"
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
	return NewMockHandlerWithOptions(MockOptions{})
}

type MockOptions struct {
	ValidationFailureProvider string
	ConnectionFailureProvider string
}

func NewMockHandlerWithOptions(options MockOptions) http.Handler {
	var mu sync.Mutex
	plan := config.SolutionPlan{}
	defaults := config.DeploymentDefaults{TemplateID: "clm", Template: "Contract Lifecycle Management", Repository: "https://github.com/unofficialbox/box-bedrock-for-clm", Strategy: "reuse", Components: []string{"box", "salesforce"}}
	settings := mockConnectionSettings()
	deployments := []audit.DeploymentRecord{}
	validate := func(ctx context.Context, plan config.SolutionPlan, _ []lifecycle.Item, emit func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		items := make([]lifecycle.Item, 0, len(plan.Components))
		for _, provider := range plan.Components {
			component := "Authentication"
			emit(provider, lifecycle.ProgressUpdate{Message: "Testing the selected " + providerName(provider) + " connection", Component: component, State: lifecycle.ProgressRunning, Current: 0, Total: 1})
			if provider == options.ValidationFailureProvider {
				detail := providerName(provider) + " authentication failed: session expired"
				if provider == "box" {
					detail = "Box OAuth session has expired"
				}
				emit(provider, lifecycle.ProgressUpdate{Message: detail, Component: component, State: lifecycle.ProgressFailed, Current: 1, Total: 1})
				return []lifecycle.Item{{Provider: provider, Name: providerName(provider), Status: lifecycle.StatusFailed, Detail: detail}}, nil
			}
			emit(provider, lifecycle.ProgressUpdate{Message: "Authentication verified", Component: component, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
		}
		for _, provider := range plan.Components {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			component := mockProviderComponent(provider)
			emit(provider, lifecycle.ProgressUpdate{Message: "Inspecting mock " + provider + " configuration", Component: component, State: lifecycle.ProgressRunning, Total: 1})
			time.Sleep(500 * time.Millisecond)
			emit(provider, lifecycle.ProgressUpdate{Message: "Ready to deploy", Component: component, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
			emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " validation complete", State: lifecycle.ProgressCompleted})
			items = append(items, lifecycle.Item{
				Provider: provider, Name: providerName(provider), Status: lifecycle.StatusMissing,
				Detail: "Mock validation found one deployable change.", Deployable: true,
				Missing: []string{component}, DeployableComponents: []string{component}, Planned: []string{component},
				Changes: mockValidationChanges(provider),
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
			if provider == "salesforce" {
				const availability = "Salesforce org availability"
				const managedPackage = "Managed Package:Box for Salesforce 5.43"
				emit(provider, lifecycle.ProgressUpdate{Message: "Checking the selected Salesforce org", Component: availability, State: lifecycle.ProgressRunning, Total: 1})
				time.Sleep(300 * time.Millisecond)
				emit(provider, lifecycle.ProgressUpdate{Message: "Salesforce org is available", Component: availability, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
				emit(provider, lifecycle.ProgressUpdate{Message: "Salesforce reports queued · request 0Hf-mock", Component: managedPackage, State: lifecycle.ProgressRunning, Total: 1})
				time.Sleep(650 * time.Millisecond)
				emit(provider, lifecycle.ProgressUpdate{Message: "Salesforce reports in progress · 1m elapsed", Component: managedPackage, State: lifecycle.ProgressRunning, Total: 1})
				time.Sleep(650 * time.Millisecond)
				emit(provider, lifecycle.ProgressUpdate{Message: "Managed package installed", Component: managedPackage, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
			}
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
			DeploymentID: completedAt.Format("20060102T150405Z"), Name: plan.Name, TemplateID: plan.TemplateID,
			Strategy: plan.Strategy, StartedAt: startedAt, CompletedAt: completedAt,
			Duration:        completedAt.Sub(startedAt).Round(time.Millisecond).String(),
			ChangesRecorded: true,
			Providers:       mockProviderRecords(result),
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
		DefaultsStore: func() (config.DeploymentDefaults, error) {
			mu.Lock()
			defer mu.Unlock()
			return defaults, nil
		},
		DefaultsSaver: func(next config.DeploymentDefaults) error {
			mu.Lock()
			defaults = next
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
			if options.ConnectionFailureProvider == "box" {
				return config.VerifiedConnection{}, errors.New("Box OAuth session has expired; reconnect the selected Box account")
			}
			return settings.VerifiedConnections["box"], nil
		},
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if options.ConnectionFailureProvider == "salesforce" {
				return salesforceapi.OrgStatus{}, errors.New("Salesforce session has expired; reconnect the selected Salesforce org")
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00D-mock", Username: "admin@example.test", Status: "Ready"}, nil
		},
		SalesforceDevHubCheck: func(context.Context, salesforceapi.Credential) (bool, error) { return true, nil },
		SalesforceCreate: func(_ context.Context, _ salesforceapi.Credential, request salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
			alias := request.Alias
			if alias == "" {
				alias = "mock-scratch"
			}
			return salesforceapi.ScratchOrg{ID: "2SR-mock", Alias: alias, Username: "scratch@example.test", OrgID: "00D-scratch", InstanceURL: "https://scratch.example.my.salesforce.com", AccessToken: "mock-scratch-access", Status: "Active", ExpirationDate: "2026-09-24"}, nil
		},
		SalesforcePackagePrepare: func(ctx context.Context, _ config.SolutionPlan, _ salesforceapi.Credential, report func(scratchPackageProgress)) (string, error) {
			report(scratchPackageProgress{Status: "checking", Message: "Checking Box for Salesforce 5.43"})
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
			report(scratchPackageProgress{Status: "installing", Message: "Salesforce reports in progress · 1m elapsed", RequestID: "0Hf-mock-preinstall"})
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(600 * time.Millisecond):
			}
			return "Box for Salesforce 5.43 is installed", nil
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
		SalesforceDevHubAlias: "Mock Dev Hub",
		SalesforceDevHubURL:   "https://devhub.example.my.salesforce.com",
		SalesforceDevHubToken: "mock-devhub-access",
		BoxCCGClientID:        "mock-client",
		BoxCCGClientSecret:    "mock-secret",
		BoxCCGSubjectType:     "user",
		BoxCCGSubjectID:       "123456",
		BoxCCGAlias:           "box.user@example.test",
		BoxDefaultConnection:  "box-dispatch-ccg",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"box":        {VerifiedAt: verifiedAt, Selection: "box-dispatch-ccg", Identity: "box.user@example.test", Account: "123456", Enterprise: "EID-mock", Host: "https://acme.app.box.com/", AuthType: "CCG"},
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

func mockValidationChanges(provider string) []salesforceapi.MetadataFileDiff {
	if provider != "salesforce" {
		return nil
	}
	return []salesforceapi.MetadataFileDiff{
		{
			Component:   "Settings:Communities",
			Path:        "settings/Communities.settings-meta.xml",
			Kind:        "update",
			Before:      "<CommunitiesSettings>\n  <enableNetworksEnabled>false</enableNetworksEnabled>\n</CommunitiesSettings>\n",
			After:       "<CommunitiesSettings>\n  <enableNetworksEnabled>true</enableNetworksEnabled>\n</CommunitiesSettings>\n",
			Previewable: true,
		},
		{
			Component:   "CustomObject:Contract__c",
			Path:        "objects/Contract__c/fields/Amount__c.field-meta.xml",
			Kind:        "add",
			Before:      "",
			After:       "<CustomField>\n  <fullName>Amount__c</fullName>\n  <label>Amount</label>\n  <type>Currency</type>\n</CustomField>\n",
			Previewable: true,
		},
	}
}

func mockProviderRecords(items []lifecycle.Item) []audit.ProviderRecord {
	records := make([]audit.ProviderRecord, 0, len(items))
	for _, item := range items {
		record := audit.ProviderRecord{
			Provider: item.Provider, StatusBefore: lifecycle.StatusMissing, StatusAfter: item.Status,
			Detail: item.Detail, Deployed: append([]string(nil), item.Present...), PresentAfter: append([]string(nil), item.Present...),
			Changes: append([]salesforceapi.MetadataFileDiff(nil), item.Changes...),
		}
		if item.Provider == "salesforce" {
			record.Resources = []lifecycle.ResourceReference{{Provider: "salesforce", Component: "Salesforce org", Kind: "organization", Name: "admin@example.test", ID: "00D-mock", URL: "https://example.my.salesforce.com"}}
		}
		records = append(records, record)
	}
	return records
}
