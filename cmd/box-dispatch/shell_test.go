package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	deploymentaudit "github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
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

	updated, _ := model.requestDeployConfirmation()
	model = updated.(rootShellModel)
	if !model.confirmingDeploy || model.deployStarted {
		t.Fatal("deploy did not stop at the confirmation prompt")
	}
	if model.deployConfirmCursor != 1 {
		t.Fatalf("confirmation should default to the safe choice (Cancel), got cursor %d", model.deployConfirmCursor)
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

func TestComponentsAlwaysOfferAllSupportedPlatforms(t *testing.T) {
	// Even a minimal config that names only Box must still offer every
	// platform box-dispatch supports; BCL only enriches display copy.
	cfg := &config.RuntimeConfig{
		ActiveScenario: "clm",
		Scenarios:      map[string]config.Scenario{"clm": {Providers: []string{"box"}}},
		Providers: map[string]config.ProviderConfig{
			"box":                   {DisplayName: "Box", Role: "content"},
			"salesforce-agentforce": {DisplayName: "SF Custom", Role: "structured"},
		},
	}
	components := componentsFromRuntime(cfg)
	wantKeys := []string{"box", "salesforce", "databricks", "aws"}
	if len(components) != len(wantKeys) {
		t.Fatalf("got %d components, want %d: %+v", len(components), len(wantKeys), components)
	}
	for i, want := range wantKeys {
		if components[i].provider != want {
			t.Fatalf("component %d provider = %q, want %q", i, components[i].provider, want)
		}
	}
	// Config enrichment reaches providers via their BCL id (salesforce-agentforce).
	if components[1].name != "SF Custom" || components[1].role != "structured" {
		t.Fatalf("salesforce copy not enriched from config: %+v", components[1])
	}
	// databricks/aws have no config entry, so they keep built-in copy.
	if components[2].name == "" || components[3].name == "" {
		t.Fatalf("unconfigured platforms lost their default copy: %+v", components)
	}
	if !components[0].required || !components[0].selected {
		t.Fatal("box must be required and selected")
	}
}

func TestComponentsFallBackToDefaultsWithoutConfig(t *testing.T) {
	components := componentsFromRuntime(nil)
	wantKeys := []string{"box", "salesforce", "databricks", "aws"}
	if len(components) != len(wantKeys) {
		t.Fatalf("got %d components, want %d", len(components), len(wantKeys))
	}
	for i, want := range wantKeys {
		if components[i].provider != want {
			t.Fatalf("component %d = %q, want %q", i, components[i].provider, want)
		}
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

func TestTemplatesFillMissingRepositoryFromDefaults(t *testing.T) {
	// An imported/minimal config that names clm but omits the repository (and
	// other presentation fields) must still be packageable.
	cfg := &config.RuntimeConfig{
		ActiveScenario: "clm",
		Scenarios:      map[string]config.Scenario{"clm": {DisplayName: "Clm deployment", Providers: []string{"box"}}},
	}
	templates := templatesFromRuntime(cfg)
	if len(templates) == 0 || templates[0].id != "clm" {
		t.Fatalf("expected clm template: %+v", templates)
	}
	if templates[0].repository != "https://github.com/unofficialbox/box-bedrock-for-clm" {
		t.Fatalf("repository not filled from defaults: %+v", templates[0])
	}
	// The config's own display name is preserved; only empty fields are filled.
	if templates[0].name != "Clm deployment" {
		t.Fatalf("config display name overwritten: %+v", templates[0])
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
	// A minimal config that names only Box as a provider.
	seeded := `{"activeScenario":"clm","scenarios":{"clm":{"displayName":"CLM","providers":["box"]}},"providers":{"box":{"displayName":"Box Custom"}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "environment.json"), []byte(seeded), 0o600); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	// BUILD still offers all four supported platforms regardless of the config's
	// provider subset; only display copy is enriched.
	if len(model.components) != 4 {
		t.Fatalf("got %d components, want all 4 supported: %+v", len(model.components), model.components)
	}
	if model.components[0].name != "Box Custom" {
		t.Fatalf("box display copy not enriched from config: %+v", model.components[0])
	}
	// The template list still reflects the seeded scenarios.
	if model.templates[0].id != "clm" {
		t.Fatalf("template not sourced from seeded scenario: %+v", model.templates[0])
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

func TestWelcomePresentsBrandedLaunchExperience(t *testing.T) {
	model := newSetupOnlyShell()
	view := model.viewWelcome(112)
	for _, expected := range []string{"Community-built. Open source. Punk Rock. 🤘", "BOX ", "DISPATCH", "SELECT STACK", "PICK QUICKSTART", "🚀"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("welcome view does not contain %q", expected)
		}
	}
	// Product branding lives in the shared header rather than the welcome body.
	if header := model.header(112); !strings.Contains(header, "UNOFFICIALBOX.DEV") {
		t.Fatalf("header does not carry product branding: %q", header)
	}
}

func TestSpacebarSelectsHighlightedRowLikeEnter(t *testing.T) {
	// Spacebar must activate the highlighted row on the plain list screens, the
	// same as Enter, so arrows/space/enter behave consistently everywhere.
	model := newSetupOnlyShell()
	model.screen = screenWelcome
	model.cursor = 0
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(rootShellModel)
	if model.screen != screenComponents {
		t.Fatalf("space on welcome did not select the highlighted row: screen = %v", model.screen)
	}
}

func TestDeployConfirmButtonsAndKeys(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDeploy
	model.validationItems = []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true}}
	updated, _ := model.requestDeployConfirmation()
	model = updated.(rootShellModel)

	view := model.renderDeployConfirm(72)
	for _, want := range []string{"Deploy", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm view missing %q: %q", want, view)
		}
	}

	// Left moves focus onto Deploy; Enter then starts the deployment.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(rootShellModel)
	if model.deployConfirmCursor != 0 {
		t.Fatalf("left did not move focus to Deploy: cursor %d", model.deployConfirmCursor)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if !model.deployStarted || model.confirmingDeploy {
		t.Fatal("enter on the Deploy button did not start the deployment")
	}
}

func TestEnteringDeployOpensConfirmation(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenValidate
	model.validateDone = true
	model.validationItems = []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenDeploy || !model.confirmingDeploy {
		t.Fatal("entering Deploy did not open the confirmation prompt")
	}
}

func TestTeardownRequiresTypedConfirmationBeforeDeleting(t *testing.T) {
	model := newSetupOnlyShell()
	record := deploymentaudit.DeploymentRecord{
		DeploymentID: "20260101T000000Z",
		PackageRoot:  "/tmp/box-bedrock-for-clm",
		Providers: []deploymentaudit.ProviderRecord{{
			Provider:  "box",
			Resources: []lifecycle.ResourceReference{{Provider: "box", Kind: "folder", ID: "123", Name: "Workspace"}},
		}},
	}
	updated, _ := model.openTeardown(record)
	model = updated.(rootShellModel)
	if model.screen != screenTeardown {
		t.Fatalf("screen = %v, want teardown", model.screen)
	}
	if model.teardownStarted {
		t.Fatal("opening the reset preview must not start deleting")
	}

	// Enter opens the confirmation gate rather than running the reset.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if !model.confirmingTeardown || model.teardownConfirmForm == nil {
		t.Fatal("enter should open the destructive confirmation")
	}
	if model.teardownStarted {
		t.Fatal("the reset must not start before the confirmation is typed")
	}

	// Escape abandons the reset without deleting anything.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(rootShellModel)
	if model.confirmingTeardown || model.teardownStarted {
		t.Fatal("escape should cancel the reset")
	}
}

func TestTeardownConfirmationPhraseUsesPackageName(t *testing.T) {
	record := deploymentaudit.DeploymentRecord{DeploymentID: "id", PackageRoot: "/tmp/box-bedrock-for-clm"}
	if got := teardownConfirmationPhrase(record); got != "box-bedrock-for-clm" {
		t.Fatalf("phrase = %q, want the package directory name", got)
	}
	// Falls back to the deployment id when there is no usable package name.
	if got := teardownConfirmationPhrase(deploymentaudit.DeploymentRecord{DeploymentID: "id"}); got != "id" {
		t.Fatalf("fallback phrase = %q, want the deployment id", got)
	}
}

func TestTeardownWithNoRecordedResourcesIsInert(t *testing.T) {
	model := newSetupOnlyShell()
	updated, _ := model.openTeardown(deploymentaudit.DeploymentRecord{DeploymentID: "empty"})
	model = updated.(rootShellModel)
	if model.teardownError == "" {
		t.Fatal("a deployment with no resources should explain there is nothing to remove")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.confirmingTeardown || model.teardownStarted {
		t.Fatal("there is nothing to confirm or delete")
	}
}

func TestBoxCCGActionIsOfferedOnlyForBox(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "box"
	if !containsStr(model.providerActions(), "ccg") {
		t.Fatal("Box should offer the CCG connect action")
	}
	model.provider = "salesforce"
	if containsStr(model.providerActions(), "ccg") {
		t.Fatal("CCG is Box-only")
	}
}

func TestBoxCCGSavePersistsCredentials(t *testing.T) {
	// Isolate the state dir so the save lands somewhere disposable.
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	model := newSetupOnlyShell()
	model.provider = "box"
	// The CCG values live behind heap pointers so huh's writes survive bubbletea's
	// model copies; saveBoxCCG reads them back through those pointers.
	clientID, clientSecret, subjectType, subjectID := "cid", "csecret", "user", "385982796"
	model.ccgClientID = &clientID
	model.ccgClientSecret = &clientSecret
	model.ccgSubjectType = &subjectType
	model.ccgSubjectID = &subjectID
	updated, _ := model.saveBoxCCG()
	model = updated.(rootShellModel)
	if model.screen != screenProvider {
		t.Fatalf("after save screen = %v, want provider", model.screen)
	}

	saved, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HasBoxCCG() || saved.BoxCCGClientID != "cid" || saved.BoxCCGClientSecret != "csecret" ||
		saved.BoxCCGSubjectType != "user" || saved.BoxCCGSubjectID != "385982796" {
		t.Fatalf("CCG credentials not persisted: %+v", saved)
	}
	// A newly captured connection becomes the default box-dispatch deploys with.
	if saved.BoxDefaultConnection != boxconn.DispatchCCGName {
		t.Fatalf("new CCG connection should be the default, got %q", saved.BoxDefaultConnection)
	}
}

func containsStr(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestTruncateCellShortensToDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"exact", 5, "exact"},
		{"x", 0, ""},
		{"abc", 1, "…"},
	}
	for _, c := range cases {
		if got := truncateCell(c.in, c.n); got != c.want {
			t.Fatalf("truncateCell(%q,%d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestComponentTypeStripsDisplayName(t *testing.T) {
	if got := componentType("Folder Structure:CLM Contract Workspace"); got != "Folder Structure" {
		t.Fatalf("componentType = %q", got)
	}
	if got := componentType("Metadata Template"); got != "Metadata Template" {
		t.Fatalf("componentType passthrough = %q", got)
	}
}

func TestDeployedResourcesFlattensEveryProvider(t *testing.T) {
	m := rootShellModel{validationItems: []lifecycle.Item{
		{Provider: "box", Resources: []lifecycle.ResourceReference{
			{Provider: "box", Kind: "folder", Name: "Workspace", ID: "1"},
			{Provider: "box", Kind: "file", Name: "doc.pdf", ID: "2"},
		}},
		{Provider: "salesforce", Resources: []lifecycle.ResourceReference{
			{Provider: "salesforce", Kind: "apex_class", Name: "Router", ID: "3"},
		}},
		{Provider: "aws"}, // no resources
	}}
	if got := m.deployedResources(); len(got) != 3 {
		t.Fatalf("deployedResources = %d rows, want 3: %+v", len(got), got)
	}
}
