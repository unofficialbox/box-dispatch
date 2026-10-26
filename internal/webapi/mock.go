package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxdiff"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
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
			changes := mockValidationChanges(provider)
			if plan.TemplateID == aiAgentPromotionTemplateID && provider == "box" {
				component = "AI Agent:Legal Assistant"
				changes = mockAIAgentPromotionChanges()
			}
			emit(provider, lifecycle.ProgressUpdate{Message: "Inspecting mock " + provider + " configuration", Component: component, State: lifecycle.ProgressRunning, Total: 1})
			time.Sleep(500 * time.Millisecond)
			emit(provider, lifecycle.ProgressUpdate{Message: "Ready to deploy", Component: component, State: lifecycle.ProgressCompleted, Current: 1, Total: 1})
			emit(provider, lifecycle.ProgressUpdate{Message: providerName(provider) + " validation complete", State: lifecycle.ProgressCompleted})
			items = append(items, lifecycle.Item{
				Provider: provider, Name: providerName(provider), Status: lifecycle.StatusMissing,
				Detail: "Mock validation found one deployable change.", Deployable: true,
				Missing: []string{component}, DeployableComponents: []string{component}, Planned: []string{component},
				Changes: changes,
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
		ContextFingerprint: func(plan config.SolutionPlan, settings config.ConnectionSettings) (string, error) {
			data, err := json.Marshal(plan)
			return string(data) + config.DeploymentTargetFingerprint(settings), err
		},
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
		BoxCompare: func(_ context.Context, left, right string) (boxdiff.Comparison, error) {
			started := time.Now().UTC()
			leftSnapshot := boxdiff.Snapshot{
				Environment: boxdiff.Environment{ConnectionID: left, Alias: "Acme Production", Identity: "box.user@example.test", EnterpriseID: "EID-mock", Domain: "acme.app.box.com", CapturedAt: started.Format(time.RFC3339Nano)},
				Templates: []boxdiff.MetadataTemplate{
					{Key: "contracts", DisplayName: "Contracts", Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum", DisplayName: "Status", Options: []string{"Draft", "Approved"}}}},
					{Key: "retention", DisplayName: "Retention", Fields: []boxdiff.MetadataTemplateField{{Key: "years", Type: "integer", DisplayName: "Years"}}},
				},
			}
			rightSnapshot := boxdiff.Snapshot{
				Environment: boxdiff.Environment{ConnectionID: right, Alias: "Acme Sandbox", Identity: "sandbox.user@example.test", EnterpriseID: "EID-sandbox", Domain: "acme-sandbox.app.box.com", CapturedAt: started.Add(time.Millisecond).Format(time.RFC3339Nano)},
				Templates: []boxdiff.MetadataTemplate{
					{Key: "contracts", DisplayName: "Contract records", Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum", DisplayName: "Status", Options: []string{"Draft", "Review", "Approved"}}}},
					{Key: "vendor", DisplayName: "Vendors", Fields: []boxdiff.MetadataTemplateField{{Key: "risk", Type: "enum", DisplayName: "Risk", Options: []string{"Low", "High"}}}},
				},
			}
			return boxdiff.CompareSnapshots(started, leftSnapshot, rightSnapshot, time.Now().UTC())
		},
		BoxFolderCompare: func(_ context.Context, left, right string) (boxdiff.FolderComparison, error) {
			started := time.Now().UTC()
			leftSnapshot := boxdiff.FolderSnapshot{
				Environment: boxdiff.Environment{ConnectionID: left, Alias: "Acme Production", Identity: "box.user@example.test", EnterpriseID: "EID-mock", Domain: "acme.app.box.com", CapturedAt: started.Format(time.RFC3339Nano)},
				Folders: []boxdiff.Folder{
					{Path: "/Contracts", Name: "Contracts", Depth: 1, Items: boxdiff.FolderItemCounts{Folders: 1, Files: 2}},
					{Path: "/Contracts/Archive", Name: "Archive", Depth: 2, Items: boxdiff.FolderItemCounts{Files: 1}},
					{Path: "/Legal", Name: "Legal", Depth: 1},
				},
			}
			rightSnapshot := boxdiff.FolderSnapshot{
				Environment: boxdiff.Environment{ConnectionID: right, Alias: "Acme Sandbox", Identity: "sandbox.user@example.test", EnterpriseID: "EID-sandbox", Domain: "acme-sandbox.app.box.com", CapturedAt: started.Add(time.Millisecond).Format(time.RFC3339Nano)},
				Folders: []boxdiff.Folder{
					{Path: "/Contracts", Name: "Contracts", Depth: 1, Items: boxdiff.FolderItemCounts{Folders: 1, Files: 3}},
					{Path: "/Contracts/Templates", Name: "Templates", Depth: 2, Items: boxdiff.FolderItemCounts{Files: 2}},
					{Path: "/Operations", Name: "Operations", Depth: 1},
				},
			}
			return boxdiff.CompareFolderSnapshots(started, leftSnapshot, rightSnapshot, time.Now().UTC())
		},
		BoxAIAgentCompare: func(_ context.Context, left, right string) (boxdiff.AIAgentComparison, error) {
			started := time.Now().UTC()
			leftSnapshot := boxdiff.AIAgentSnapshot{
				Environment: boxdiff.Environment{ConnectionID: left, Alias: "Acme Production", Identity: "box.user@example.test", EnterpriseID: "EID-mock", Domain: "acme.app.box.com", CapturedAt: started.Format(time.RFC3339Nano)},
				Agents: []boxdiff.AIAgent{
					{Key: "contract extractor", Name: "Contract Extractor", Kind: "Extract", AccessState: "enabled", Configuration: map[string]any{"extract": map[string]any{"description": "Extract contract terms", "long_text": map[string]any{"model": "model-production", "prompt_template": "Return contract terms"}}}},
					{Key: "legal assistant", Name: "Legal Assistant", Kind: "Ask", AccessState: "enabled", Configuration: map[string]any{"ask": map[string]any{"description": "Answer legal questions"}}},
				},
			}
			rightSnapshot := boxdiff.AIAgentSnapshot{
				Environment: boxdiff.Environment{ConnectionID: right, Alias: "Acme Sandbox", Identity: "sandbox.user@example.test", EnterpriseID: "EID-sandbox", Domain: "acme-sandbox.app.box.com", CapturedAt: started.Add(time.Millisecond).Format(time.RFC3339Nano)},
				Agents: []boxdiff.AIAgent{
					{Key: "contract extractor", Name: "Contract Extractor", Kind: "Extract", AccessState: "enabled", Configuration: map[string]any{"extract": map[string]any{"description": "Extract contract terms", "long_text": map[string]any{"model": "model-sandbox", "prompt_template": "Return contract terms and renewal dates"}}}},
					{Key: "summary writer", Name: "Summary Writer", Kind: "Text generation", AccessState: "enabled", Configuration: map[string]any{"textGen": map[string]any{"description": "Draft contract summaries"}}},
				},
			}
			return boxdiff.CompareAIAgentSnapshots(started, leftSnapshot, rightSnapshot, time.Now().UTC())
		},
		AIAgentPromotionAssembler: func(name string, comparison boxdiff.AIAgentComparison, agents []boxdiff.AIAgent, _ time.Time) (config.SolutionPlan, error) {
			if len(agents) == 0 {
				return config.SolutionPlan{}, errors.New("select at least one AI agent")
			}
			return config.SolutionPlan{
				Name: name, TemplateID: aiAgentPromotionTemplateID, Template: "AI agent promotion",
				Repository: "box://enterprise/" + comparison.Left.EnterpriseID + "/ai-agents",
				Components: []string{"box"}, Strategy: solution.StrategyReuse, PackagePath: "/mock/ai-agent-promotion",
			}, nil
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
		BoxConnections: []config.BoxAppConnection{
			{ID: "box-mock-production", Alias: "Acme Production", ClientID: "mock-client", ClientSecret: "mock-secret", SubjectType: "user", SubjectID: "123456", VerifiedAt: verifiedAt, Identity: "box.user@example.test", Account: "123456", Enterprise: "EID-mock", Hostname: "https://acme.app.box.com/"},
			{ID: "box-mock-sandbox", Alias: "Acme Sandbox", ClientID: "mock-client-sandbox", ClientSecret: "mock-secret-sandbox", SubjectType: "user", SubjectID: "654321", VerifiedAt: verifiedAt, Identity: "sandbox.user@example.test", Account: "654321", Enterprise: "EID-sandbox", Hostname: "https://acme-sandbox.app.box.com/"},
		},
		BoxSelectedConnectionID: "box-mock-production",
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

func mockAIAgentPromotionChanges() []salesforceapi.MetadataFileDiff {
	return []salesforceapi.MetadataFileDiff{{
		Component: "AI Agent:Legal Assistant", Path: "ai-agents/legal-assistant.json", Kind: "add", Previewable: true,
		After: `{
  "name": "Legal Assistant",
  "mode": "ask",
  "accessState": "enabled",
  "description": "Answer legal questions",
  "configuration": {
    "ask": {
      "description": "Answer legal questions"
    }
  }
}`,
	}}
}

func mockProviderRecords(items []lifecycle.Item) []audit.ProviderRecord {
	records := make([]audit.ProviderRecord, 0, len(items))
	for _, item := range items {
		record := audit.ProviderRecord{
			Provider: item.Provider, StatusBefore: lifecycle.StatusMissing, StatusAfter: item.Status,
			Detail: item.Detail, Deployed: append([]string(nil), item.Present...), PresentAfter: append([]string(nil), item.Present...),
			Changes: append([]salesforceapi.MetadataFileDiff(nil), item.Changes...),
		}
		if item.Provider == "box" {
			record.EnvironmentID = "EID-mock"
			record.EnvironmentDomain = "acme.app.box.com"
		}
		if item.Provider == "salesforce" {
			record.EnvironmentID = "00D-mock"
			record.EnvironmentDomain = "example.my.salesforce.com"
			record.Resources = []lifecycle.ResourceReference{{Provider: "salesforce", Component: "Salesforce org", Kind: "organization", Name: "admin@example.test", ID: "00D-mock", URL: "https://example.my.salesforce.com"}}
		}
		records = append(records, record)
	}
	return records
}
