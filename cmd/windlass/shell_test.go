package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unofficialbox/box-windlass/internal/lifecycle"
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
	model.screen = screenTemplates
	model.components[1].selected = true

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

	model = updatedShell(t, model, tea.KeyRight)
	if model.screen != screenConfig {
		t.Fatalf("screen = %v, want config", model.screen)
	}

	model.setConfigFocus(2)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
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
	view := renderComponentChecklist(item, 80)
	item.Deployable = true
	view = renderComponentChecklist(item, 80)
	for _, expected := range []string{"DEPLOYMENT CHECKLIST", "CustomField", "1 present · 1 to deploy", "Layout"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("checklist does not contain %q: %q", expected, view)
		}
	}
}

func TestWelcomePresentsBrandedMaritimeLaunchExperience(t *testing.T) {
	model := newSetupOnlyShell()
	view := model.viewWelcome(112)
	for _, expected := range []string{"UNOFFICIALBOX.DEV", "HEAVE YOUR SOLUTION", "BEGIN ASSEMBLY", "PICK QUICKSTART", "🚢", "⚓", "🌊"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("welcome view does not contain %q", expected)
		}
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
