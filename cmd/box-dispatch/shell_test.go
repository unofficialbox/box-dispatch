package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
)

func updatedShell(t *testing.T, model rootShellModel, key tea.KeyType) rootShellModel {
	t.Helper()
	updated, _ := model.Update(tea.KeyMsg{Type: key})
	result, ok := updated.(rootShellModel)
	if !ok {
		t.Fatalf("Update returned %T, want rootShellModel", updated)
	}
	return result
}

func TestEnteringConnectChecksOnlySelectedProviders(t *testing.T) {
	model := newSetupOnlyShell()
	// Connect is entered from the component picker; selections live on answers.
	model.screen = screenComponents
	model.answers.components = []string{"box", "salesforce"}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(rootShellModel)

	if result.screen != screenDashboard {
		t.Fatalf("screen = %v, want dashboard", result.screen)
	}
	if cmd == nil {
		t.Fatal("entering Connect did not start the connectivity check")
	}
	if result.statuses["box"] != connectionChecking {
		t.Fatalf("Box status = %v, want checking", result.statuses["box"])
	}
	if len(result.queue) != 1 || result.queue[0] != "salesforce" {
		t.Fatalf("queue = %v, want only selected Salesforce after Box", result.queue)
	}
	if _, exists := result.statuses["databricks"]; exists {
		t.Fatal("unselected Databricks was scheduled for checking")
	}
	if _, exists := result.statuses["aws"]; exists {
		t.Fatal("unselected AWS was scheduled for checking")
	}
}

func TestConnectedServicesUnlockConfigurationAndPackage(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDashboard
	model.statuses["box"] = connectionConnected
	choice := model.templates[0]
	model.selected = &choice

	// Connect precedes template selection, which in turn unlocks configuration.
	model = updatedShell(t, model, tea.KeyRight)
	if model.screen != screenTemplates {
		t.Fatalf("screen = %v, want templates", model.screen)
	}

	model = updatedShell(t, model, tea.KeyRight)
	if model.screen != screenConfig {
		t.Fatalf("screen = %v, want config", model.screen)
	}

	// Focus 4 is the confirm row that starts packaging.
	model.setConfigFocus(4)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenPackage {
		t.Fatalf("screen = %v, want package", model.screen)
	}
	if cmd == nil {
		t.Fatal("entering Package did not start package creation")
	}
}

func TestConfigFocusReachesNameAndFolderBrowser(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenConfig

	model = updatedShell(t, model, tea.KeyTab)
	if model.configFocus != 1 || !model.packageInput.Focused() {
		t.Fatal("Tab did not focus the package-name input")
	}

	model.setConfigFocus(0)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	model = updated.(rootShellModel)
	if model.screen != screenDirectoryPicker || cmd == nil {
		t.Fatal("b did not open and initialize the directory browser")
	}
}

func TestConfigViewExplainsBothFieldsAndContinueAction(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenConfig
	model.width = 120
	model.height = 40
	view := model.View()
	for _, expected := range []string{"Parent directory", "Package directory name", "Create package", "b browse"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Config view does not contain %q", expected)
		}
	}
}

func TestValidateRunsAutomaticallyOnlyOnFirstEntry(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenPackage
	model.packageDone = true
	model.packagePath = t.TempDir()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenValidate || !model.validateStarted || !model.validateRunning || cmd == nil {
		t.Fatal("first Validate entry did not start validation")
	}

	model.validateRunning = false
	model.screen = screenPackage
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenValidate {
		t.Fatalf("screen = %v, want validate", model.screen)
	}
	if cmd != nil {
		t.Fatal("returning to Validate unexpectedly started validation again")
	}
}

func TestValidationTracksProgressPerProvider(t *testing.T) {
	model := newSetupOnlyShell()
	model.components[1].selected = true
	model.screen = screenValidate
	model.packagePath = t.TempDir()

	updated, cmd := model.startValidation()
	model = updated.(rootShellModel)
	if cmd == nil || model.currentValidation != "box" || model.validationProgress["box"] <= 0 {
		t.Fatal("validation did not start Box provider progress")
	}
	updated, _ = model.Update(providerValidationFinishedMsg{provider: "box", item: lifecycle.Item{Provider: "box", Status: lifecycle.StatusPresent}})
	model = updated.(rootShellModel)
	if model.validationProgress["box"] != 1 || model.currentValidation != "salesforce" {
		t.Fatalf("progress = %#v, current = %q", model.validationProgress, model.currentValidation)
	}
}

func TestConnectBlocksConfigurationUntilAllSelectedServicesConnect(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDashboard
	model.components[1].selected = true
	model.statuses["box"] = connectionConnected

	model = updatedShell(t, model, tea.KeyRight)
	if model.screen != screenDashboard {
		t.Fatalf("screen = %v, want dashboard", model.screen)
	}
	if model.message == "" {
		t.Fatal("blocked Configuration transition did not explain what remains")
	}
}

func TestOverallJourneyProgressIsAvailableOnEveryWorkflowStage(t *testing.T) {
	model := newSetupOnlyShell()
	stages := []shellScreen{screenComponents, screenTemplates, screenConfig, screenPackage, screenValidate, screenDeploy}
	for _, stage := range stages {
		model.screen = stage
		if model.overallJourneyProgress() <= 0 {
			t.Fatalf("stage %v has no overall journey progress", stage)
		}
		if !strings.Contains(model.stageHeader(), "OVERALL PROGRESS") {
			t.Fatalf("stage %v does not render overall progress", stage)
		}
	}
}

func TestDeployRequiresExplicitConfirmation(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDeploy
	model.validationItems = []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true}}

	updated, cmd := model.requestDeployConfirmation()
	model = updated.(rootShellModel)
	if cmd == nil || !model.confirmingDeploy || model.deployStarted {
		t.Fatal("deploy did not stop at the confirmation form")
	}
}

func TestProviderChecklistGroupsDeploymentComponents(t *testing.T) {
	item := lifecycle.Item{
		Present: []string{"CustomObject:Contract__c", "CustomField:Contract__c.Name__c"},
		Missing: []string{"CustomField:Contract__c.Status__c", "Layout:Contract__c-Contract Layout"},
	}
	view := renderComponentChecklist(item, "deploy", false, 1, "", 80)
	item.DeployableComponents = append([]string{}, item.Missing...)
	view = renderComponentChecklist(item, "deploy", false, 1, "", 80)
	for _, expected := range []string{"DEPLOYMENT CHECKLIST", "CustomField", "1 present · 1 to deploy", "Layout"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("checklist does not contain %q: %q", expected, view)
		}
	}
}

func TestComponentsFromRuntimeMapsBCLIdentifiers(t *testing.T) {
	cfg := &config.RuntimeConfig{
		ActiveScenario: "clm",
		Scenarios: map[string]config.Scenario{
			"clm": {DisplayName: "CLM", Providers: []string{"box", "salesforce-agentforce", "bedrock-agentcore"}},
		},
		Providers: map[string]config.ProviderConfig{
			"box":                   {DisplayName: "Box", Role: "content"},
			"salesforce-agentforce": {DisplayName: "Salesforce + Agentforce", Role: "structured"},
			"bedrock-agentcore":     {DisplayName: "AWS Bedrock AgentCore", Role: "runtime"},
		},
	}
	components := componentsFromRuntime(cfg)
	if len(components) != 3 {
		t.Fatalf("got %d components, want 3: %+v", len(components), components)
	}
	// BCL IDs are translated to internal keys the checker/lifecycle understand.
	wantKeys := []string{"box", "salesforce", "aws"}
	for i, want := range wantKeys {
		if components[i].provider != want {
			t.Fatalf("component %d provider = %q, want %q", i, components[i].provider, want)
		}
	}
	if components[1].name != "Salesforce + Agentforce" || components[1].role != "structured" {
		t.Fatalf("display copy not sourced from config: %+v", components[1])
	}
	if !components[0].required || !components[0].selected {
		t.Fatal("box must be required and selected")
	}
}

func TestComponentsFromRuntimeFallsBackForMissingCopy(t *testing.T) {
	cfg := &config.RuntimeConfig{
		ActiveScenario: "clm",
		Scenarios:      map[string]config.Scenario{"clm": {Providers: []string{"salesforce-agentforce"}}},
		Providers:      map[string]config.ProviderConfig{}, // no ProviderConfig entry
	}
	components := componentsFromRuntime(cfg)
	if len(components) != 1 || components[0].provider != "salesforce" {
		t.Fatalf("unexpected components: %+v", components)
	}
	// Falls back to built-in copy when the config omits displayName/role.
	if components[0].name != "Salesforce + Agentforce" || components[0].role == "" {
		t.Fatalf("fallback copy not applied: %+v", components[0])
	}
}

func TestTemplatesFromRuntimeOrdersActiveFirst(t *testing.T) {
	cfg := &config.RuntimeConfig{
		ActiveScenario: "clm",
		Scenarios: map[string]config.Scenario{
			"lifesciences":     {DisplayName: "Life Sciences", Sector: "REGULATED", Repository: "r-ls"},
			"clm":              {DisplayName: "CLM", Sector: "LEGAL", Repository: "r-clm"},
			"citizen-services": {DisplayName: "Citizen Services"},
		},
	}
	templates := templatesFromRuntime(cfg)
	gotOrder := []string{templates[0].id, templates[1].id, templates[2].id}
	wantOrder := []string{"clm", "citizen-services", "lifesciences"} // active first, then alpha
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("template order = %v, want %v", gotOrder, wantOrder)
		}
	}
	if templates[0].sector != "LEGAL" || templates[0].repository != "r-clm" {
		t.Fatalf("template fields not sourced from config: %+v", templates[0])
	}
}

func TestShellFallsBackToDefaultsWithoutConfig(t *testing.T) {
	// TestMain isolates XDG_CONFIG_HOME to an empty dir, so no runtime config exists.
	model := newSetupOnlyShell()
	if len(model.components) != 4 {
		t.Fatalf("got %d components, want 4 defaults", len(model.components))
	}
	if model.components[1].provider != "salesforce" || model.components[3].provider != "aws" {
		t.Fatalf("default internal keys wrong: %+v", model.components)
	}
	// The "new solution" starter is always appended last.
	last := model.templates[len(model.templates)-1]
	if last.id != "new" {
		t.Fatalf("last template = %q, want new starter", last.id)
	}
}

func TestShellSourcesScenariosFromSeededConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "box-dispatch", "default")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seeded := `{"activeScenario":"clm","scenarios":{"clm":{"displayName":"CLM","providers":["box","salesforce-agentforce"]}},"providers":{"box":{"displayName":"Box"},"salesforce-agentforce":{"displayName":"SF"}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "environment.json"), []byte(seeded), 0o600); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	if len(model.components) != 2 {
		t.Fatalf("got %d components, want 2 from seeded config: %+v", len(model.components), model.components)
	}
	if model.components[1].provider != "salesforce" {
		t.Fatalf("seeded provider not mapped to internal key: %+v", model.components[1])
	}
}

func TestProviderConnectionDetailsRendersDiscoveredIdentity(t *testing.T) {
	box := providerConnectionDetails(checker.ProviderResult{
		Name:      "box",
		Discovery: checker.ProviderDiscovery{Identity: "kadams@boxdemo.com", Account: "385982796", Enterprise: "5105484"},
	})
	for _, expected := range []string{"user kadams@boxdemo.com", "UID 385982796", "EID 5105484"} {
		if !strings.Contains(box, expected) {
			t.Fatalf("box detail %q does not contain %q", box, expected)
		}
	}

	salesforce := providerConnectionDetails(checker.ProviderResult{
		Name:      "salesforce",
		Discovery: checker.ProviderDiscovery{Identity: "kadams@agentforce.com", Profile: "agentforce"},
	})
	if !strings.Contains(salesforce, "alias agentforce") {
		t.Fatalf("salesforce detail missing alias: %q", salesforce)
	}
	// Empty fields are omitted rather than rendered as dangling labels.
	if strings.Contains(salesforce, "org ") {
		t.Fatalf("salesforce detail rendered an empty org: %q", salesforce)
	}
}

func TestWelcomePresentsBrandedMaritimeLaunchExperience(t *testing.T) {
	model := newSetupOnlyShell()
	view := model.viewWelcome(112)
	for _, expected := range []string{"BOX DEVELOPER COMMUNITY", "BUILD BEYOND", "THE BOX.", "SELECT STACK", "PICK QUICKSTART", "🚢", "⚓", "🌊"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("welcome view does not contain %q", expected)
		}
	}
	// Product branding lives in the shared header rather than the welcome body.
	if header := model.header(112); !strings.Contains(header, "UNOFFICIALBOX.DEV") {
		t.Fatalf("header does not carry product branding: %q", header)
	}
}

func TestEnteringDeployOpensConfirmation(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenValidate
	model.validateDone = true
	model.validationItems = []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true}}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenDeploy || !model.confirmingDeploy || cmd == nil {
		t.Fatal("entering Deploy did not open the Huh confirmation")
	}
}
