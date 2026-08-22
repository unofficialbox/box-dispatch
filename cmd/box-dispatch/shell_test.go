package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	deploymentaudit "github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
	"github.com/unofficialbox/box-dispatch/internal/workspace"
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

func TestShellDefaultsToReuseExistingDeployment(t *testing.T) {
	model := newSetupOnlyShell()
	if got := model.answers.deploymentStrategy; got != solution.StrategyReuse {
		t.Fatalf("deployment strategy = %q, want %q", got, solution.StrategyReuse)
	}
	if got := deploymentStrategyLabel(model.answers.deploymentStrategy); got != "REUSE EXISTING" {
		t.Fatalf("deployment strategy label = %q, want REUSE EXISTING", got)
	}
	model.cycleDeploymentStrategy(1)
	if got := model.answers.deploymentStrategy; got != solution.StrategyCreateNew {
		t.Fatalf("cycled deployment strategy = %q, want %q", got, solution.StrategyCreateNew)
	}
}

func TestEnteringConnectChecksOnlySelectedProviders(t *testing.T) {
	isolateShellRoot(t)
	model := newSetupOnlyShell()
	// Connect is entered from the component picker; selections live on answers.
	model.screen = screenComponents
	model.answers.components = []string{"box", "salesforce"}
	model.rebuildComponentForm()

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

func TestEnteringConnectReusesSavedVerifiedConnections(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{
		BoxCCGClientID: "client", BoxCCGClientSecret: "secret",
		BoxCCGSubjectType: "user", BoxCCGSubjectID: "123",
		BoxDefaultConnection: boxconn.DispatchCCGName,
		SalesforceAlias:      "dispatch-scratch",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"box": {
				VerifiedAt: "2026-08-12T12:00:00Z", Selection: boxconn.DispatchCCGName,
				Identity: "box@example.test", Enterprise: "5105484", AuthType: "CCG",
			},
			"salesforce": {
				VerifiedAt: "2026-08-12T12:00:00Z", Selection: "dispatch-scratch",
				Identity: "sf@example.test", Profile: "dispatch-scratch", OrgType: "scratch",
				OrgStatus: "Active", ExpiresAt: "2099-09-10", Options: []string{"dispatch-scratch", "production"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	model := newSetupOnlyShell()
	model.screen = screenComponents
	model.answers.components = []string{"box", "salesforce"}
	model.rebuildComponentForm()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(rootShellModel)
	if cmd != nil {
		t.Fatal("saved verified connections unexpectedly scheduled another check")
	}
	if !result.allSelectedConnected() || result.statuses["box"] != connectionConnected || result.statuses["salesforce"] != connectionConnected {
		t.Fatalf("restored statuses = %#v, want both connected", result.statuses)
	}
	if got := providerConnectionSummary(result.results["salesforce"]); !strings.Contains(got, "dispatch-scratch") {
		t.Fatalf("restored Salesforce summary = %q", got)
	}
	if !strings.Contains(result.message, "saved verified connections") {
		t.Fatalf("reuse message = %q", result.message)
	}
}

func TestChangedConnectionDoesNotReuseMismatchedVerification(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{
		SalesforceAlias: "new-org",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {VerifiedAt: "2026-08-12T12:00:00Z", Selection: "old-org", Profile: "old-org", OrgStatus: "Active"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	if got := model.statuses["salesforce"]; got == connectionConnected {
		t.Fatalf("mismatched saved connection restored as connected: %v", got)
	}
}

func TestExpiredSalesforceVerificationIsNotRestored(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{
		SalesforceAlias: "expired-scratch",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {
				VerifiedAt: "2026-07-01T12:00:00Z", Selection: "expired-scratch",
				Identity: "expired@example.test", Profile: "expired-scratch", OrgType: "scratch",
				OrgStatus: "Active", ExpiresAt: "2026-07-31",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	if got := model.statuses["salesforce"]; got == connectionConnected {
		t.Fatalf("expired saved Salesforce org restored as connected: %v", got)
	}
}

func TestStartingAnotherDeploymentClearsCompletedRunState(t *testing.T) {
	isolateShellRoot(t)
	model := newSetupOnlyShell()
	model.screen, model.cursor = screenWelcome, 0
	model.packagePath = "/tmp/previous-package"
	model.packageDone = true
	model.validateStarted, model.validateDone = true, true
	model.deployDone = true
	model.deploymentPhase = deploymentPhaseComplete
	model.validationItems = []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent}}
	model.deploymentBaseline = append([]lifecycle.Item(nil), model.validationItems...)
	model.deploymentAuditPath = "/tmp/previous-audit.json"
	model.statuses["box"] = connectionConnected
	model.componentForm.State = huh.StateCompleted

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(rootShellModel)
	if result.screen != screenComponents {
		t.Fatalf("screen = %v, want Choose", result.screen)
	}
	if result.packageDone || result.validateStarted || result.validateDone || result.deployDone || len(result.validationItems) != 0 || result.packagePath != "" || result.deploymentAuditPath != "" {
		t.Fatalf("new deployment retained completed run state: packageDone=%v validate=%v/%v deployDone=%v items=%d package=%q audit=%q",
			result.packageDone, result.validateStarted, result.validateDone, result.deployDone, len(result.validationItems), result.packagePath, result.deploymentAuditPath)
	}
	if result.deploymentPhase != deploymentPhaseReview {
		t.Fatalf("deployment phase = %q, want review", result.deploymentPhase)
	}
	if result.componentForm.State == huh.StateCompleted {
		t.Fatal("new deployment retained the completed Choose form")
	}
	if result.statuses["box"] != connectionConnected {
		t.Fatal("starting a new deployment discarded the valid Box connection")
	}
	choice := templateChoice{id: "test", name: "Second run", repository: "https://example.test/solution.git"}
	result.selected = &choice
	result.directoryInput.SetValue(t.TempDir())
	result.packageInput.SetValue("second-package")
	updated, cmd := result.startDeploymentPipeline()
	result = updated.(rootShellModel)
	if cmd == nil || !result.packageStarted || result.deploymentPhase != deploymentPhasePackage {
		t.Fatalf("second run skipped package assembly: phase=%q packageStarted=%v cmd=%v", result.deploymentPhase, result.packageStarted, cmd)
	}
}

func TestBuildPageUsesSalesforceProviderName(t *testing.T) {
	model := newSetupOnlyShell()
	names := make([]string, 0, len(model.components))
	for _, component := range model.components {
		names = append(names, component.name)
	}
	if !slices.Contains(names, "Salesforce") {
		t.Fatalf("Build component catalog is missing Salesforce: %v", names)
	}
	if slices.Contains(names, "Agentforce") {
		t.Fatalf("Build component catalog still contains Agentforce: %v", names)
	}
}

func TestSalesforceConnectSetsCLIDefault(t *testing.T) {
	got := strings.Join(providerCLIConnectCommand("salesforce"), " ")
	if !strings.Contains(got, "org login web --set-default") {
		t.Fatalf("Salesforce connect command = %q, want --set-default", got)
	}
}

func TestSuccessfulExternalLoginImmediatelyRechecksProvider(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider

	updated, cmd := model.Update(externalFinishedMsg{provider: "salesforce"})
	result := updated.(rootShellModel)
	if cmd == nil {
		t.Fatal("successful login did not schedule a connectivity check")
	}
	if result.statuses["salesforce"] != connectionChecking {
		t.Fatalf("Salesforce status = %v, want checking", result.statuses["salesforce"])
	}
}

func TestSuccessfulSalesforceCheckPersistsTargetOrg(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	t.Setenv("SF_ALIAS", "")

	model := newSetupOnlyShell()
	updated, _ := model.Update(checkFinishedMsg{
		provider: "salesforce",
		result: checker.ProviderResult{
			Name:           "salesforce",
			ConnectivityOK: true,
			Discovery: checker.ProviderDiscovery{
				Identity: "scratch-user@example.test", Profile: "dispatch-scratch", OrgID: "00Dtest",
				OrgType: "scratch", OrgStatus: "Active", ExpiresAt: "2026-09-09",
			},
		},
	})
	result := updated.(rootShellModel)
	if result.statuses["salesforce"] != connectionConnected {
		t.Fatalf("Salesforce status = %v, want connected", result.statuses["salesforce"])
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.SalesforceAlias != "dispatch-scratch" {
		t.Fatalf("saved Salesforce target = %q", settings.SalesforceAlias)
	}
	if settings.SalesforceOrgID != "00Dtest" || settings.SalesforceOrgType != "scratch" || settings.SalesforceOrgStatus != "Active" || settings.SalesforceExpirationDate != "2026-09-09" {
		t.Fatalf("saved Salesforce lifecycle metadata = %#v", settings)
	}
	snapshot, found := settings.VerifiedConnections["salesforce"]
	if !found || snapshot.Selection != "dispatch-scratch" || snapshot.OrgID != "00Dtest" || snapshot.VerifiedAt == "" {
		t.Fatalf("saved Salesforce verification = %#v", snapshot)
	}
	if os.Getenv("SF_ALIAS") != "dispatch-scratch" {
		t.Fatalf("SF_ALIAS = %q", os.Getenv("SF_ALIAS"))
	}
}

func TestSalesforceScratchCreationRequiresExplicitConfirmation(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider
	actions := model.providerActions()
	index := -1
	for i, action := range actions {
		if action == "scratch" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("Salesforce actions do not include scratch creation: %v", actions)
	}
	updated, cmd := model.runProviderAction(index)
	model = updated.(rootShellModel)
	if cmd == nil || model.screen != screenProvider {
		t.Fatalf("scratch action = screen %v cmd %v, want Dev Hub discovery", model.screen, cmd)
	}

	hubs := []salesforceorg.DevHub{
		{Alias: "dev", Username: "dev@example.com", OrgID: "00Ddev", ConnectedStatus: "Connected"},
		{Alias: "devhub", Username: "hub@example.com", OrgID: "00Dhub", ConnectedStatus: "Connected"},
	}
	updated, _ = model.Update(devHubListFinishedMsg{hubs: hubs})
	model = updated.(rootShellModel)
	if model.screen != screenDevHubs || len(model.devHubs) != 2 {
		t.Fatalf("Dev Hub discovery = screen %v hubs %v", model.screen, model.devHubs)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(rootShellModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenScratchConfirm || model.scratchConfirmCursor != 1 || model.selectedDevHub.Alias != "devhub" {
		t.Fatalf("Dev Hub selection = screen %v cursor %d hub %#v", model.screen, model.scratchConfirmCursor, model.selectedDevHub)
	}
	if !strings.HasPrefix(model.pendingScratchAlias, "box-dispatch-") {
		t.Fatalf("scratch alias = %q", model.pendingScratchAlias)
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.SalesforceDevHubAlias != "devhub" {
		t.Fatalf("saved Dev Hub = %q", settings.SalesforceDevHubAlias)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(rootShellModel)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if cmd == nil || model.screen != screenProvider || !model.salesforceCreating {
		t.Fatal("confirmed scratch creation did not schedule the Salesforce CLI command")
	}
}

func TestScratchCreationFailureOffersFullDiagnosticView(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height = 120, 40
	model.provider = "salesforce"
	model.screen = screenProvider
	model.results["salesforce"] = checker.ProviderResult{
		Name: "salesforce", Discovery: checker.ProviderDiscovery{Options: []string{"dev", "devhub"}},
	}
	failure := &salesforceorg.Failure{
		Summary:    "Salesforce Dev Hub \"devhub\" could not create the scratch org.",
		Diagnostic: `{"name":"NoDefaultDevHub","stack":"complete stack trace"}`,
	}
	updated, _ := model.Update(scratchOrgFinishedMsg{alias: "box-dispatch-test", err: failure})
	model = updated.(rootShellModel)
	if model.statuses["salesforce"] != connectionFailed || !strings.Contains(model.message, "failed") {
		t.Fatalf("creation failure not surfaced: status %v message %q", model.statuses["salesforce"], model.message)
	}
	if !containsStr(model.providerActions(), "salesforce-existing") {
		t.Fatalf("creation failure discarded authenticated profile options: %v", model.providerActions())
	}
	if strings.Contains(model.message, failure.Summary) {
		t.Fatalf("footer duplicates the panel error: %q", model.message)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(rootShellModel)
	if model.screen != screenDiagnostic || !strings.Contains(model.View(), "complete stack trace") {
		t.Fatalf("full diagnostic view was not opened: screen %v", model.screen)
	}
}

func TestSalesforceProviderActionsAreStreamlinedByStatus(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"

	if got := model.providerActions(); !slices.Equal(got, []string{"salesforce-existing", "scratch", "back"}) {
		t.Fatalf("disconnected actions = %v", got)
	}

	model.statuses["salesforce"] = connectionConnected
	if got := model.providerActions(); !slices.Equal(got, []string{"salesforce-done", "salesforce-existing", "forget-verification", "scratch"}) {
		t.Fatalf("connected actions = %v", got)
	}
}

func TestForgetVerificationKeepsConnectionSelection(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{
		SalesforceAlias: "dispatch-scratch",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {
				VerifiedAt: "2026-08-12T12:00:00Z", Selection: "dispatch-scratch",
				Identity: "user@example.test", Profile: "dispatch-scratch", OrgStatus: "Active",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider
	index := slices.Index(model.providerActions(), "forget-verification")
	if index < 0 {
		t.Fatalf("connected provider actions omit snapshot removal: %v", model.providerActions())
	}
	updated, cmd := model.runProviderAction(index)
	model = updated.(rootShellModel)
	if cmd != nil {
		t.Fatal("forgetting a snapshot unexpectedly scheduled provider work")
	}
	if model.statuses["salesforce"] != connectionPending {
		t.Fatalf("status = %v, want not checked", model.statuses["salesforce"])
	}
	if _, found := model.results["salesforce"]; found {
		t.Fatal("forgotten verification details remain in the shell")
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.SalesforceAlias != "dispatch-scratch" {
		t.Fatalf("forgetting verification removed the connection selection: %q", settings.SalesforceAlias)
	}
	if _, found := settings.VerifiedConnections["salesforce"]; found {
		t.Fatal("saved verification snapshot was not removed")
	}
	if !strings.Contains(model.message, "credentials and selections were kept") {
		t.Fatalf("removal guidance = %q", model.message)
	}
}

func TestChoosingSalesforceOrgImmediatelyRechecks(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{VerifiedConnections: map[string]config.VerifiedConnection{
		"salesforce": {VerifiedAt: "2026-08-12T12:00:00Z", Selection: "old-org", Profile: "old-org", OrgStatus: "Active"},
	}}); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenOptions
	model.results["salesforce"] = checker.ProviderResult{
		Name: "salesforce",
		Discovery: checker.ProviderDiscovery{
			Options: []string{"dispatch-dev", "dispatch-scratch"},
		},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if cmd == nil || model.screen != screenProvider || model.statuses["salesforce"] != connectionChecking {
		t.Fatalf("org selection = screen %v status %v cmd %v", model.screen, model.statuses["salesforce"], cmd)
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.SalesforceAlias != "dispatch-dev" {
		t.Fatalf("saved Salesforce alias = %q", settings.SalesforceAlias)
	}
	if _, found := settings.VerifiedConnections["salesforce"]; found {
		t.Fatal("choosing another Salesforce org retained the previous verification")
	}
}

func TestSalesforceOptionsCombineProfilesAndSignIn(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenOptions
	model.results["salesforce"] = checker.ProviderResult{
		Name: "salesforce",
		Discovery: checker.ProviderDiscovery{
			Options: []string{"dispatch-dev"},
		},
	}

	view := model.viewOptions(100)
	for _, expected := range []string{"Use an existing Salesforce org", "dispatch-dev", salesforceLoginOption} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Salesforce options do not contain %q:\n%s", expected, view)
		}
	}
}

func TestConnectedSalesforceViewShowsCurrentOrgSummary(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider
	model.statuses["salesforce"] = connectionConnected
	model.results["salesforce"] = checker.ProviderResult{
		Name:   "salesforce",
		Checks: []string{"salesforce tools discovered", "salesforce org connected"},
		Discovery: checker.ProviderDiscovery{
			Identity: "scratch@example.test", Profile: "dispatch-scratch", OrgType: "scratch",
			OrgStatus: "Active", ExpiresAt: "2026-09-10", Host: "https://example.test",
		},
	}

	view := model.viewProvider(100)
	for _, expected := range []string{"Current org", "dispatch-scratch", "expires 2026-09-10", "Continue with this org"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("connected Salesforce view does not contain %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "salesforce tools discovered") {
		t.Fatalf("connected Salesforce view leaked raw checker log:\n%s", view)
	}
}

func TestScratchFlowAuthenticatesDevHubOnlyWhenNeeded(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider

	updated, cmd := model.Update(devHubListFinishedMsg{})
	model = updated.(rootShellModel)
	if cmd == nil || model.screen != screenProvider || !strings.Contains(model.message, "Opening Salesforce login") {
		t.Fatalf("missing Dev Hub = screen %v message %q cmd %v", model.screen, model.message, cmd)
	}
}

func TestSalesforceProviderRecheckShortcut(t *testing.T) {
	model := newSetupOnlyShell()
	model.provider = "salesforce"
	model.screen = screenProvider

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(rootShellModel)
	if cmd == nil || model.statuses["salesforce"] != connectionChecking {
		t.Fatalf("recheck shortcut = status %v cmd %v", model.statuses["salesforce"], cmd)
	}
}

func TestFailedConnectionCheckOffersFullDiagnosticView(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height = 120, 40
	model.provider = "salesforce"
	model.screen = screenProvider
	updated, _ := model.Update(checkFinishedMsg{provider: "salesforce", result: checker.ProviderResult{
		Name: "salesforce", ConnectivityOK: false, Diagnostic: `{"stack":"connection stack"}`,
		Checks: []string{"Salesforce scratch org is deleted."},
	}})
	model = updated.(rootShellModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(rootShellModel)
	if model.screen != screenDiagnostic || !strings.Contains(model.View(), "connection stack") {
		t.Fatalf("connection diagnostic did not open: screen %v", model.screen)
	}
}

func TestFailedValidationBlocksDeployAndOpensDiagnostic(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height = 120, 40
	model.screen = screenValidate
	model.validateDone = true
	model.validationItems = []lifecycle.Item{{
		Provider: "salesforce", Status: lifecycle.StatusFailed,
		Detail: "Scratch org is deleted.", Diagnostic: `{"stack":"complete validation stack"}`,
	}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenValidate || !strings.Contains(model.message, "blocked") {
		t.Fatalf("failed validation advanced to deploy: screen %v message %q", model.screen, model.message)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(rootShellModel)
	if model.screen != screenDiagnostic || !strings.Contains(model.View(), "complete validation stack") {
		t.Fatalf("validation diagnostic did not open: screen %v", model.screen)
	}
}

func TestConnectedServicesUnlockConfigurationAndReview(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDashboard
	model.statuses["box"] = connectionConnected
	choice := model.templates[0]
	model.selected = &choice

	// Choose already captured the template, so Connect unlocks configuration directly.
	model = updatedShell(t, model, tea.KeyRight)
	if model.screen != screenConfig {
		t.Fatalf("screen = %v, want config", model.screen)
	}

	// Focus 4 opens Review without creating the package yet.
	model.setConfigFocus(4)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenPackage {
		t.Fatalf("screen = %v, want review", model.screen)
	}
	if cmd != nil || model.packageStarted || model.deploymentPhase != deploymentPhaseReview {
		t.Fatal("entering Review unexpectedly started package creation")
	}
}

func isolateShellRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

func TestBoxConfigurationHidesUnsupportedCapabilitiesByDefault(t *testing.T) {
	root := isolateShellRoot(t)
	model := newSetupOnlyShell()
	model.answers.templateID = "clm"
	model.prepareBoxComponentSelection()
	view := model.viewBoxComponents(100)
	if strings.Contains(view, "Automate Workflow") {
		t.Fatalf("validate-only Automate workflow appeared as configurable:\n%s", view)
	}
	for _, expected := range []string{"Folder Structure", "Metadata Template", "Doc Gen Template", "AI Agent", "Box Hub"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("deployable capability %q missing from configuration:\n%s", expected, view)
		}
	}
	settings, err := shellstate.LoadUISettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"box_form", "box_app", "https_connector", "automate_workflow"} {
		if settings.BoxComponentVisibility[id] {
			t.Fatalf("%s should default hidden in BCL", id)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".dispatch", "ui-settings.bcl")); err != nil {
		t.Fatalf("BCL UI settings were not generated: %v", err)
	}
}

func TestBCLCanShowUnsupportedCapabilitiesAsLockedReferences(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveUISettings(config.UISettings{BoxComponentVisibility: map[string]bool{
		"automate_workflow": true,
		"box_form":          true,
	}}); err != nil {
		t.Fatal(err)
	}

	model := newSetupOnlyShell()
	model.answers.templateID = "clm"
	model.prepareBoxComponentSelection()
	view := model.viewBoxComponents(100)
	for _, expected := range []string{"Automate Workflow", "PARTIAL API", "Box Form", "NO PUBLIC API"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("visible reference %q missing from configuration:\n%s", expected, view)
		}
	}
	for i, capability := range model.boxCapabilities {
		if capability.ComponentType == "Automate Workflow" {
			model.cursor = i
			break
		}
	}
	updated, _ := model.updateBoxComponents(tea.KeyMsg{Type: tea.KeySpace})
	result := updated.(rootShellModel)
	if result.boxComponentMode != "defaults" || result.boxComponentValues["automate_workflow"] {
		t.Fatal("reference-only Automate workflow became selected")
	}
	if !strings.Contains(result.message, "reference-only") {
		t.Fatalf("locked row message = %q", result.message)
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
	for _, expected := range []string{"Parent directory", "Package directory name", "Review deployment", "b browse"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Config view does not contain %q", expected)
		}
	}
}

func TestDeploymentPipelineReusesPackageAndStartsValidation(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDeploy
	model.packageDone = true
	model.packagePath = t.TempDir()
	model.deploymentPhase = deploymentPhaseReview

	updated, cmd := model.startDeploymentPipeline()
	model = updated.(rootShellModel)
	if model.screen != screenDeploy || model.deploymentPhase != deploymentPhaseValidate || !model.validateStarted || !model.validateRunning || cmd == nil {
		t.Fatal("unified Deploy screen did not start validation for the assembled package")
	}
}

func TestChooseCombinesQuickstartAndProviderSelection(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenComponents
	_ = model.componentForm.Init()
	view := model.viewComponents(112)
	for _, expected := range []string{"Choose a quickstart and providers", "Choose a solution quickstart", "Choose platform components"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("combined Choose view does not contain %q:\n%s", expected, view)
		}
	}
	if got := strings.Count(view, "(Coming soon)"); got != 2 {
		t.Fatalf("Choose view shows %d coming-soon provider labels, want 2:\n%s", got, view)
	}
}

func TestChooseSpaceSelectsAndTabSwitchesPanels(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenComponents
	_ = model.componentForm.Init()
	quickstarts, ok := model.componentForm.GetFocusedField().(*huh.MultiSelect[string])
	if !ok || quickstarts.GetKey() != "template" {
		t.Fatalf("initial Choose focus = %T, want quickstart selector", model.componentForm.GetFocusedField())
	}

	selectedQuickstart := model.answers.templateID
	// Arrows move the highlight without changing the selected quickstart.
	model = updatedShell(t, model, tea.KeyDown)
	if model.answers.templateID != selectedQuickstart {
		t.Fatalf("Down changed quickstart from %q to %q before Space", selectedQuickstart, model.answers.templateID)
	}
	model = updatedShell(t, model, tea.KeySpace)
	if model.answers.templateID == selectedQuickstart {
		t.Fatalf("Space did not select the highlighted quickstart after %q", selectedQuickstart)
	}
	view := model.viewComponents(112)
	for _, quickstart := range model.templates {
		if !strings.Contains(view, quickstart.name) {
			t.Fatalf("selecting a quickstart hid %q:\n%s", quickstart.name, view)
		}
	}

	// Tab explicitly switches panels and cycles back from components.
	model = updatedShell(t, model, tea.KeyTab)
	components, ok := model.componentForm.GetFocusedField().(*huh.MultiSelect[string])
	if !ok || components.GetKey() != "components" {
		t.Fatalf("Tab focused %T/%q, want provider selector", model.componentForm.GetFocusedField(), model.componentForm.GetFocusedField().GetKey())
	}

	model = updatedShell(t, model, tea.KeyDown)
	model = updatedShell(t, model, tea.KeySpace)
	if !slices.Contains(model.answers.components, "salesforce") {
		t.Fatalf("Space did not toggle Salesforce: %v", model.answers.components)
	}
	model = updatedShell(t, model, tea.KeyTab)
	if model.componentForm.GetFocusedField().GetKey() != "template" {
		t.Fatalf("second Tab did not cycle to quickstarts: %q", model.componentForm.GetFocusedField().GetKey())
	}

	footer := model.footer()
	for _, expected := range []string{"rows", "switch panel", "next/continue", "select/toggle"} {
		if !strings.Contains(footer, expected) {
			t.Fatalf("Choose footer omitted %q:\n%s", expected, footer)
		}
	}
}

func TestChooseComingSoonProvidersAreReadOnly(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenComponents
	_ = model.componentForm.Init()

	// Move from quickstarts to providers, then from Box through Salesforce to
	// Databricks. Space must leave the roadmap-only provider unselected.
	model = updatedShell(t, model, tea.KeyTab)
	model = updatedShell(t, model, tea.KeyCtrlA)
	if slices.Contains(model.answers.components, "databricks") || slices.Contains(model.answers.components, "aws") {
		t.Fatalf("select-all enabled coming-soon providers: %v", model.answers.components)
	}
	model = updatedShell(t, model, tea.KeyDown)
	model = updatedShell(t, model, tea.KeyDown)
	model = updatedShell(t, model, tea.KeySpace)
	if slices.Contains(model.answers.components, "databricks") || !strings.Contains(model.message, "Databricks is coming soon") {
		t.Fatalf("Databricks was selectable or lacked guidance: components=%v message=%q", model.answers.components, model.message)
	}

	model = updatedShell(t, model, tea.KeyDown)
	model = updatedShell(t, model, tea.KeySpace)
	if slices.Contains(model.answers.components, "aws") || !strings.Contains(model.message, "AWS Bedrock AgentCore is coming soon") {
		t.Fatalf("AWS was selectable or lacked guidance: components=%v message=%q", model.answers.components, model.message)
	}

	// Defense in depth: stale or accessible-form values are removed before the
	// provider plan is used.
	model.answers.components = append(model.answers.components, "databricks", "aws")
	if err := model.syncComponentAnswers(); err != nil {
		t.Fatal(err)
	}
	if got := model.selectedProviders(); slices.Contains(got, "databricks") || slices.Contains(got, "aws") {
		t.Fatalf("coming-soon providers survived selection normalization: %v", got)
	}

	scoped := newSetupOnlyShell("aws")
	if got := scoped.selectedProviders(); !slices.Equal(got, []string{"box"}) {
		t.Fatalf("scoped coming-soon provider became active: %v", got)
	}
}

func TestDefaultWorkflowMovesThroughFiveStages(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenComponents

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenDashboard || model.selected == nil {
		t.Fatalf("Choose did not enter Connect with a selected quickstart: screen=%v selected=%v", model.screen, model.selected)
	}

	for _, provider := range model.selectedProviders() {
		model.statuses[provider] = connectionConnected
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(rootShellModel)
	if model.screen != screenConfig {
		t.Fatalf("Connect did not enter Configure: screen=%v", model.screen)
	}

	model.directoryInput.SetValue(t.TempDir())
	model.packageInput.SetValue("five-stage-package")
	model.setConfigFocus(4)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenPackage || model.deploymentPhase != deploymentPhaseReview || model.packageStarted {
		t.Fatalf("Configure did not enter a non-mutating Review: screen=%v phase=%q packageStarted=%v", model.screen, model.deploymentPhase, model.packageStarted)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenDeploy || !model.confirmingDeploy {
		t.Fatalf("Review did not open the Deploy confirmation: screen=%v confirming=%v", model.screen, model.confirmingDeploy)
	}
	model.deployConfirmCursor = 0
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenDeploy || model.deploymentPhase != deploymentPhasePackage || !model.packageStarted || cmd == nil {
		t.Fatalf("Deploy did not start the unified pipeline: screen=%v phase=%q packageStarted=%v", model.screen, model.deploymentPhase, model.packageStarted)
	}
}

func TestPackageCompletionAutomaticallyStartsValidation(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDeploy
	model.deploymentPhase = deploymentPhasePackage
	model.packageStarted = true
	root := t.TempDir()

	updated, cmd := model.Update(packageFinishedMsg{manifest: workspace.PackageManifest{Destination: root}})
	model = updated.(rootShellModel)
	if model.deploymentPhase != deploymentPhaseValidate || !model.validateStarted || !model.validateRunning || cmd == nil {
		t.Fatalf("package completion did not continue into validation: phase=%q started=%v running=%v", model.deploymentPhase, model.validateStarted, model.validateRunning)
	}
}

func TestValidationCompletionAutomaticallyFinishesNoopDeployment(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen = screenDeploy
	model.deploymentPhase = deploymentPhaseValidate
	model.validateRunning = true
	model.validationItems = []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent}}
	model.validationQueue = nil

	updated, cmd := model.startNextValidation()
	model = updated.(rootShellModel)
	if cmd != nil || !model.validateDone || !model.deployDone || model.deploymentPhase != deploymentPhaseComplete {
		t.Fatalf("successful validation did not continue through a no-op deployment: phase=%q validateDone=%v deployDone=%v", model.deploymentPhase, model.validateDone, model.deployDone)
	}
}

func TestDeploymentHistoryIsTheOnlyResetEntryPoint(t *testing.T) {
	model := newSetupOnlyShell()
	if len(welcomeOptions) != 2 {
		t.Fatalf("welcome options = %v, want only Start and History", welcomeOptions)
	}
	model.screen = screenHistory
	model.deploymentHistory = []deploymentaudit.DeploymentRecord{{
		DeploymentID: "20260811T000000Z",
		PackageRoot:  "/tmp/five-stage-package",
		Providers: []deploymentaudit.ProviderRecord{{
			Provider:  "box",
			Resources: []lifecycle.ResourceReference{{Provider: "box", Kind: "folder", ID: "123", Name: "Workspace"}},
		}},
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(rootShellModel)
	if model.screen != screenTeardown || model.teardownRecord == nil {
		t.Fatalf("history did not open the reset preview: screen=%v record=%v", model.screen, model.teardownRecord)
	}
}

func TestSpinnerStopsSchedulingTicksWhenWorkFinishes(t *testing.T) {
	model := newSetupOnlyShell()
	updated, cmd := model.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Fatal("idle shell scheduled another spinner tick")
	}
	model = updated.(rootShellModel)
	model.validateRunning = true
	_, cmd = model.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("active validation did not keep the spinner running")
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

func TestWorkflowHeaderUsesStepsWithoutArtificialProgress(t *testing.T) {
	model := newSetupOnlyShell()
	stages := []shellScreen{screenComponents, screenTemplates, screenConfig, screenPackage, screenValidate, screenDeploy}
	for _, stage := range stages {
		model.screen = stage
		header := model.stageHeader()
		if header == "" {
			t.Fatalf("stage %v has no workflow header", stage)
		}
		if strings.Contains(header, "OVERALL PROGRESS") || strings.Contains(header, "%") {
			t.Fatalf("stage %v renders navigation position as progress: %q", stage, header)
		}
		for _, expected := range []string{"CHOOSE", "CONNECT", "CONFIGURE", "REVIEW", "DEPLOY"} {
			if !strings.Contains(header, expected) {
				t.Fatalf("stage %v header omitted %q: %q", stage, expected, header)
			}
		}
		for _, obsolete := range []string{"BUILD", "TEMPLATE", "PACKAGE", "VALIDATE"} {
			if strings.Contains(header, obsolete) {
				t.Fatalf("stage %v header retained obsolete stage %q: %q", stage, obsolete, header)
			}
		}
	}
}

func TestChoosePanelExplainsDecisionWithoutDuplicatingControls(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.width, m.height = screenComponents, 120, 40
	_ = m.componentForm.Init()
	view := m.View()
	if !strings.Contains(view, "Start from a proven architecture") {
		t.Fatalf("Choose panel omitted decision guidance:\n%s", view)
	}
	if strings.Contains(view, "press Space") || strings.Contains(view, "Highlight with arrows") {
		t.Fatalf("Choose panel duplicated keyboard guidance:\n%s", view)
	}
	if !strings.Contains(view, "space select/toggle") {
		t.Fatalf("Choose footer omitted the interaction guidance:\n%s", view)
	}
}

func TestDispatchThemeUsesReferencePalette(t *testing.T) {
	colors := map[string]struct {
		got  lipgloss.Color
		want string
	}{
		"navy":             {navy, "#113053"},
		"blue":             {cyan, "#0061D3"},
		"light blue":       {ice, "#91C1FC"},
		"community coral":  {coral, "#FF6658"},
		"warning yellow":   {gold, "#F4B21A"},
		"success green":    {green, "#25C180"},
		"body blue":        {muted, "#4B6E98"},
		"standard outline": {white, "#D0D0D0"},
	}
	for name, color := range colors {
		if got := fmt.Sprint(color.got); got != color.want {
			t.Errorf("%s = %s, want %s", name, got, color.want)
		}
	}
	if progressGradientStart != "#FD56FF" || progressGradientEnd != "#307CF2" {
		t.Fatalf("progress gradient = %s -> %s, want #FD56FF -> #307CF2", progressGradientStart, progressGradientEnd)
	}
}

func TestDeployProgressReadsAsNestedTasks(t *testing.T) {
	m := newSetupOnlyShell()
	m.deploymentPhase = deploymentPhaseApply
	m.height = 40
	block := m.deployPipelineStrip()
	if !strings.Contains(block, "DEPLOY PROGRESS") || !strings.Contains(block, "Assemble") || !strings.Contains(block, "Apply") {
		t.Fatalf("nested deploy progress omitted its hierarchy or tasks:\n%s", block)
	}
	if lipgloss.Height(block) != 2 {
		t.Fatalf("deploy progress height = %d, want a two-row labeled block:\n%s", lipgloss.Height(block), block)
	}
	if !strings.Contains(block, "┃") {
		t.Fatalf("deploy progress omitted the inset hierarchy rail:\n%s", block)
	}
	if strings.Contains(block, "ASSEMBLE") || strings.Contains(block, "APPLY") {
		t.Fatalf("child tasks compete with the all-caps primary workflow:\n%s", block)
	}
	m.height = 24
	if compact := m.deployPipelineStrip(); lipgloss.Height(compact) != 1 {
		t.Fatalf("short-terminal deploy progress height = %d, want one compact row:\n%s", lipgloss.Height(compact), compact)
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

func TestComponentsAlwaysShowTheFullPlatformRoadmap(t *testing.T) {
	// Even a minimal config that names only Box must still show the full
	// platform roadmap; BCL only enriches display copy.
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
	if !components[2].comingSoon || !components[3].comingSoon {
		t.Fatalf("Databricks and AWS must remain read-only roadmap providers: %+v", components)
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
		Discovery: checker.ProviderDiscovery{AuthType: "CCG", Identity: "kadams@boxdemo.com", Account: "385982796", Enterprise: "5105484"},
	})
	wantBox := "auth CCG\nuser kadams@boxdemo.com\nUID 385982796\nEID 5105484"
	if box != wantBox {
		t.Fatalf("box detail = %q, want %q", box, wantBox)
	}

	salesforce := providerConnectionDetails(checker.ProviderResult{
		Name: "salesforce",
		Discovery: checker.ProviderDiscovery{
			Identity: "kadams@agentforce.com", Profile: "agentforce", OrgType: "scratch",
			OrgStatus: "Active", ExpiresAt: "2026-09-09", OrgID: "00Dtest",
		},
	})
	wantSalesforce := "user kadams@agentforce.com\nalias agentforce\ntype scratch\nstatus Active\nexpires 2026-09-09\norg ID 00Dtest"
	if salesforce != wantSalesforce {
		t.Fatalf("salesforce detail = %q, want %q", salesforce, wantSalesforce)
	}
	// Empty fields are omitted rather than rendered as dangling labels.
	if strings.Contains(salesforce, "\norg ") && !strings.Contains(salesforce, "\norg ID ") {
		t.Fatalf("salesforce detail rendered an empty org: %q", salesforce)
	}

	for _, test := range []struct {
		name   string
		result checker.ProviderResult
		want   string
	}{
		{
			name: "databricks",
			result: checker.ProviderResult{Name: "databricks", Discovery: checker.ProviderDiscovery{
				Identity: "data@example.com", Profile: "clm", Host: "https://workspace.example.com",
			}},
			want: "user data@example.com\nprofile clm\nworkspace https://workspace.example.com",
		},
		{
			name: "aws",
			result: checker.ProviderResult{Name: "aws", Discovery: checker.ProviderDiscovery{
				Account: "123456789012", Profile: "demo", Region: "us-east-1",
			}},
			want: "account 123456789012\nprofile demo\nregion us-east-1",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := providerConnectionDetails(test.result); got != test.want {
				t.Fatalf("detail = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderProviderConnectionDetailsDoesNotPadLinesToLongestValue(t *testing.T) {
	rendered := renderProviderConnectionDetails(checker.ProviderResult{
		Name: "box",
		Discovery: checker.ProviderDiscovery{
			AuthType: "CCG", Identity: "kadams@boxdemo.com", Account: "385982796", Enterprise: "5105484",
		},
	})
	lines := strings.Split(rendered, "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d detail lines, want 4: %q", len(lines), rendered)
	}
	if lipgloss.Width(lines[0]) == lipgloss.Width(lines[1]) {
		t.Fatalf("short and long detail lines were padded to the same width: %q", rendered)
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("detail line has background-breaking trailing padding: %q", line)
		}
	}
}

func TestProviderConnectionSummaryShowsOnlyEssentialIdentity(t *testing.T) {
	box := providerConnectionSummary(checker.ProviderResult{
		Name: "box",
		Discovery: checker.ProviderDiscovery{
			AuthType: "CCG", Identity: "kadams@boxdemo.com", Account: "385982796", Enterprise: "5105484",
		},
	})
	if box != "user kadams@boxdemo.com\nenterprise 5105484" {
		t.Fatalf("Box summary = %q", box)
	}
	if strings.Contains(box, "CCG") || strings.Contains(box, "385982796") {
		t.Fatalf("Box summary contains advanced details: %q", box)
	}

	salesforce := providerConnectionSummary(checker.ProviderResult{
		Name: "salesforce",
		Discovery: checker.ProviderDiscovery{
			Identity: "scratch@example.test", Profile: "dispatch-scratch", OrgType: "scratch",
			ExpiresAt: "2026-09-10", OrgID: "00Dtest", Host: "https://example.test",
		},
	})
	if salesforce != "org dispatch-scratch\nexpires 2026-09-10" {
		t.Fatalf("Salesforce summary = %q", salesforce)
	}
	if strings.Contains(salesforce, "00Dtest") || strings.Contains(salesforce, "https://") {
		t.Fatalf("Salesforce summary contains advanced details: %q", salesforce)
	}
}

func TestPrimaryScreensDoNotExposeImplementationOrDuplicateControls(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height = 112, 50

	if view := model.viewComponents(112); strings.Contains(view, "Huh") {
		t.Fatalf("component screen exposes its form implementation:\n%s", view)
	}
	if view := model.viewTemplates(112); strings.Contains(view, "Huh") {
		t.Fatalf("template screen exposes its form implementation:\n%s", view)
	}
	if view := model.viewBoxComponents(112); strings.Contains(view, "Enter save") || strings.Contains(view, "a enable all") {
		t.Fatalf("Box component body duplicates footer controls:\n%s", view)
	}
	if view := model.viewDirectoryPicker(112); strings.Contains(view, "Space choose highlighted") || strings.Contains(view, "Esc cancel") {
		t.Fatalf("directory picker body duplicates footer controls:\n%s", view)
	}
}

func TestWelcomePresentsBrandedLaunchExperience(t *testing.T) {
	model := newSetupOnlyShell()
	model.height = 48 // tall enough for the big banner
	view := model.viewWelcome(112)
	// Wide, tall terminals render DISPATCH as an ANSI-shadow banner (block glyphs)
	// with a large chevron accent, so assert the branding, the wordmark banner,
	// the menu, the route strip, and the punk-rock mark.
	for _, expected := range []string{"COMMUNITY-BUILT", "🤘", "██", "Start new deployment", "CHOOSE", "CONFIGURE", "REVIEW", "DEPLOY"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("welcome view does not contain %q", expected)
		}
	}
	if strings.Contains(view, "Reset demo environment") {
		t.Fatal("destructive reset remains on the default home menu")
	}
	// Narrow or short terminals fall back to a one-line DISPATCH headline with the
	// small » accent.
	narrow := model.viewWelcome(70)
	for _, expected := range []string{"DISPATCH", "»"} {
		if !strings.Contains(narrow, expected) {
			t.Fatalf("narrow welcome fallback missing %q", expected)
		}
	}
	// Product branding lives in the shared header rather than the welcome body.
	if header := model.header(112); !strings.Contains(header, "UNOFFICIALBOX.DEV") {
		t.Fatalf("header does not carry product branding: %q", header)
	}
}

func TestExpandedHelpIsDiscoverableAndReturnsToCaller(t *testing.T) {
	model := newSetupOnlyShell()
	model.screen, model.width, model.height = screenDashboard, 100, 30
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model = updated.(rootShellModel)
	if model.screen != screenHelp || model.helpReturn != screenDashboard {
		t.Fatalf("? did not open help from Connect: screen=%v return=%v", model.screen, model.helpReturn)
	}
	view := model.View()
	allHelp := strings.Join(model.expandedHelpLines(92), "\n")
	for _, expected := range []string{"Keyboard and accessibility help", "WORKFLOW"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expanded help omitted visible heading %q:\n%s", expected, view)
		}
	}
	for _, expected := range []string{"ACCESSIBLE OUTPUT", "NO_COLOR"} {
		if !strings.Contains(allHelp, expected) {
			t.Fatalf("expanded help content omitted %q:\n%s", expected, allHelp)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if returned := updated.(rootShellModel); returned.screen != screenDashboard {
		t.Fatalf("help returned to screen %v, want Connect", returned.screen)
	}
}

func TestAccessibleFormPreferenceLoadsFromBCL(t *testing.T) {
	isolateShellRoot(t)
	if err := shellstate.SaveUISettings(config.UISettings{AccessibleForms: true}); err != nil {
		t.Fatal(err)
	}
	model := newSetupOnlyShell()
	if !model.accessibleForms {
		t.Fatal("metadata.accessibleForms did not enable accessible form mode")
	}
}

func TestAccessibleChooseCompletionContinuesIntoConnect(t *testing.T) {
	model := newSetupOnlyShell()
	model.accessibleForms = true
	model.screen = screenComponents
	updated, cmd := model.Update(accessibleFormFinishedMsg{kind: accessibleChooseForm})
	model = updated.(rootShellModel)
	if model.screen != screenDashboard || model.selected == nil || cmd == nil {
		t.Fatalf("accessible Choose did not continue into Connect: screen=%v selected=%v", model.screen, model.selected)
	}
}

func TestCoreViewsFitSupportedTerminalSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {100, 30}, {120, 40}} {
		for _, screen := range []shellScreen{screenWelcome, screenHelp, screenValidate, screenDeploy} {
			model := newSetupOnlyShell()
			model.width, model.height, model.screen = size.width, size.height, screen
			if screen == screenHelp {
				model.helpReturn = screenWelcome
			}
			if screen == screenValidate || screen == screenDeploy {
				model.validateDone = true
				model.validationItems = []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent, Detail: "Box configuration is present."}}
				model.validationProgress = map[string]float64{"box": 1}
			}
			if lines := strings.Count(model.View(), "\n") + 1; lines > size.height {
				t.Fatalf("%dx%d screen %v rendered %d lines", size.width, size.height, screen, lines)
			}
		}
	}
}

func TestExpandedHelpScrollStopsAtBothBoundaries(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height, model.screen = 80, 24, screenHelp
	for range 100 {
		model = updatedShell(t, model, tea.KeyDown)
	}
	limit := model.helpScrollLimit()
	if model.helpScroll != limit || limit == 0 {
		t.Fatalf("help bottom offset = %d, limit = %d", model.helpScroll, limit)
	}
	model = updatedShell(t, model, tea.KeyDown)
	if model.helpScroll != limit {
		t.Fatalf("help wrapped after its final row: %d", model.helpScroll)
	}
	for range 100 {
		model = updatedShell(t, model, tea.KeyUp)
	}
	if model.helpScroll != 0 {
		t.Fatalf("help top offset = %d, want 0", model.helpScroll)
	}
}

func TestDiagnosticScrollStopsAtBothBoundaries(t *testing.T) {
	model := newSetupOnlyShell()
	model.width, model.height, model.screen = 80, 24, screenDiagnostic
	model.diagnosticBody = strings.Repeat("diagnostic line with wrapped content\n", 40)
	for range 100 {
		model = updatedShell(t, model, tea.KeyDown)
	}
	width := min(max(model.width-8, 64), 112)
	limit := max(len(model.diagnosticLines(width))-model.diagnosticCapacity(), 0)
	if model.diagnosticScroll != limit || limit == 0 {
		t.Fatalf("diagnostic bottom offset = %d, limit = %d", model.diagnosticScroll, limit)
	}
	model = updatedShell(t, model, tea.KeyDown)
	if model.diagnosticScroll != limit {
		t.Fatalf("diagnostic wrapped after its final row: %d", model.diagnosticScroll)
	}
	for range 100 {
		model = updatedShell(t, model, tea.KeyUp)
	}
	if model.diagnosticScroll != 0 {
		t.Fatalf("diagnostic top offset = %d, want 0", model.diagnosticScroll)
	}
}

func TestTeardownAndDeployedAssetsStopAtBoundaries(t *testing.T) {
	resources := make([]lifecycle.ResourceReference, 40)
	for i := range resources {
		resources[i] = lifecycle.ResourceReference{Provider: "box", Kind: "file", Name: fmt.Sprintf("file-%02d", i), ID: fmt.Sprint(i)}
	}

	teardown := newSetupOnlyShell()
	teardown.width, teardown.height, teardown.screen = 80, 24, screenTeardown
	teardown.teardownRecord = &deploymentaudit.DeploymentRecord{Providers: []deploymentaudit.ProviderRecord{{Provider: "box", Resources: resources}}}
	for range 100 {
		teardown = updatedShell(t, teardown, tea.KeyDown)
	}
	teardownLimit := max(len(teardown.teardownBodyRows(teardown.width))-teardown.teardownVisibleRows(), 0)
	if teardown.teardownScroll != teardownLimit || teardownLimit == 0 {
		t.Fatalf("teardown bottom offset = %d, limit = %d", teardown.teardownScroll, teardownLimit)
	}
	teardown = updatedShell(t, teardown, tea.KeyDown)
	if teardown.teardownScroll != teardownLimit {
		t.Fatalf("teardown wrapped after its final row: %d", teardown.teardownScroll)
	}

	deployed := newSetupOnlyShell()
	deployed.width, deployed.height, deployed.screen, deployed.deployDone, deployed.deployShowDetails = 80, 24, screenDeploy, true, true
	deployed.validationItems = []lifecycle.Item{{Provider: "box", Resources: resources}}
	for range 100 {
		deployed = updatedShell(t, deployed, tea.KeyDown)
	}
	assetLimit := max(len(resources)-deployed.deployTableCapacity(), 0)
	if deployed.deployAssetsScroll != assetLimit || assetLimit == 0 {
		t.Fatalf("deployed-assets bottom offset = %d, limit = %d", deployed.deployAssetsScroll, assetLimit)
	}
	deployed = updatedShell(t, deployed, tea.KeyDown)
	if deployed.deployAssetsScroll != assetLimit {
		t.Fatalf("deployed assets wrapped after their final row: %d", deployed.deployAssetsScroll)
	}
}

func TestDeployViewFitsAndScrollReverses(t *testing.T) {
	// A tall deployment must not overflow the terminal, and scrolling up must
	// undo scrolling down — the two together are what made the table unscrollable
	// upward before (the frame overflowed and the top rendered off-screen).
	m := newSetupOnlyShell()
	m.screen, m.deployDone, m.deployShowDetails, m.width, m.height = screenDeploy, true, true, 120, 44
	provs := []string{"box", "salesforce", "databricks", "aws"}
	items := make([]lifecycle.Item, 0, len(provs))
	for _, p := range provs {
		res := make([]lifecycle.ResourceReference, 30)
		for i := range res {
			res[i] = lifecycle.ResourceReference{Provider: p, Component: "Sample Content:f", Kind: "file", Name: "f.pdf", ID: "1"}
		}
		items = append(items, lifecycle.Item{Provider: p, Status: lifecycle.StatusMissing, Deployable: true, Resources: res})
	}
	m.validationItems = items

	if lines := strings.Count(m.View(), "\n") + 1; lines > m.height {
		t.Fatalf("deploy view overflows terminal: %d lines > height %d", lines, m.height)
	}

	var model tea.Model = m
	for i := 0; i < 5; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	down := model.(rootShellModel).deployAssetsScroll
	if down == 0 {
		t.Fatal("scrolling down did not advance the window")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if up := model.(rootShellModel).deployAssetsScroll; up != down-1 {
		t.Fatalf("scroll up did not reverse down: down=%d up=%d", down, up)
	}
}

func TestDeployCompletionOffersBoxAndSelectedScratchOrg(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{SalesforceAlias: "dispatch-scratch", SalesforceOrgType: "scratch"}); err != nil {
		t.Fatal(err)
	}

	m := newSetupOnlyShell()
	for i := range m.components {
		m.components[i].selected = m.components[i].provider == "box" || m.components[i].provider == "salesforce"
	}
	m.screen, m.deployDone, m.width, m.height = screenDeploy, true, 120, 50
	m.deploymentAuditPath = "/tmp/deployment.json"
	m.results["box"] = checker.ProviderResult{Discovery: checker.ProviderDiscovery{Enterprise: "5105484"}}
	action := m.footer()
	for _, want := range []string{"open Box (EID 5105484)", "open Salesforce (dispatch-scratch)", "home"} {
		if !strings.Contains(action, want) {
			t.Fatalf("deployment completion action omitted %q:\n%s", want, action)
		}
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil || updated.(rootShellModel).screen != screenDeploy {
		t.Fatal("Salesforce open action did not return an executable command on the deployment screen")
	}
}

func TestDeployCompletionHidesSalesforceOpenForPersistentOrg(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := shellstate.SaveConnectionSettings(config.ConnectionSettings{SalesforceAlias: "production", SalesforceOrgType: "persistent"}); err != nil {
		t.Fatal(err)
	}
	m := newSetupOnlyShell()
	for i := range m.components {
		m.components[i].selected = m.components[i].provider == "box" || m.components[i].provider == "salesforce"
	}
	m.screen, m.deployDone = screenDeploy, true
	if action := m.footer(); strings.Contains(action, "open Salesforce") {
		t.Fatalf("persistent org received scratch-org open action:\n%s", action)
	}
}

func TestDeployCompletionDefaultsToFocusedSummary(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.deployDone, m.deploymentPhase = screenDeploy, true, deploymentPhaseComplete
	m.width, m.height = 120, 44
	m.deploymentAuditPath = "/tmp/private/deployment-audit.json"
	m.deploymentStartedAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	m.deploymentCompletedAt = m.deploymentStartedAt.Add(42 * time.Second)
	m.validationItems = []lifecycle.Item{{
		Provider: "box",
		Status:   lifecycle.StatusPresent,
		Present:  []string{"Folder Structure:CLM Workspace"},
		Resources: []lifecycle.ResourceReference{{
			Provider: "box", Component: "Sample Content:msa.pdf", Kind: "file", Name: "msa.pdf", ID: "999",
		}},
	}}

	view := m.View()
	for _, want := range []string{"DEPLOY PROGRESS", "Assemble", "Validate", "Apply", "Finish", "Deployment complete", "1 created", "1 already present", "42s", "Audit saved", "v details"} {
		if !strings.Contains(view, want) {
			t.Fatalf("focused completion omitted %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"DEPLOYMENT CHECKLIST", "STATUS", "/tmp/private/deployment-audit.json", "msa.pdf"} {
		if strings.Contains(view, hidden) {
			t.Fatalf("focused completion exposed detailed content %q:\n%s", hidden, view)
		}
	}
	if got := strings.Count(view, "Deployment complete"); got != 1 {
		t.Fatalf("completion outcome appears %d times, want one:\n%s", got, view)
	}
	for _, size := range []struct{ width, height int }{{80, 24}, {100, 30}, {120, 40}} {
		resized := m
		resized.width, resized.height = size.width, size.height
		if lines := strings.Count(resized.View(), "\n") + 1; lines > size.height {
			t.Fatalf("focused completion overflows %dx%d: %d lines", size.width, size.height, lines)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	detailed := updated.(rootShellModel)
	if !detailed.deployShowDetails {
		t.Fatal("v did not open deployment details")
	}
	detailView := detailed.View()
	for _, want := range []string{"STATUS", "msa.pdf", "/tmp/private/deployment-audit.json", "v summary"} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("detailed completion omitted %q:\n%s", want, detailView)
		}
	}
}

func TestPackagePhaseKeepsChangingTaskOutOfFooter(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.deploymentPhase, m.packageStarted = screenDeploy, deploymentPhasePackage, true
	m.width, m.height = 120, 40
	m.message = "Pulling the selected solution components from GitHub..."

	view := m.View()
	for _, want := range []string{"Assembling package", "CURRENT TASK", "Pulling the selected solution components from GitHub..."} {
		if !strings.Contains(view, want) {
			t.Fatalf("package phase omitted %q:\n%s", want, view)
		}
	}
	footer := m.footer()
	if strings.Contains(footer, m.message) {
		t.Fatalf("package status leaked into the contextual footer:\n%s", footer)
	}
}

func TestSubSecondDeploymentDurationIsNotShown(t *testing.T) {
	m := newSetupOnlyShell()
	m.deploymentStartedAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	m.deploymentCompletedAt = m.deploymentStartedAt.Add(400 * time.Millisecond)
	if got := m.deploymentDurationSuffix(); got != "" {
		t.Fatalf("sub-second duration = %q, want hidden", got)
	}
}

func TestDeployFocusExpandsOnlyActiveProviderWithoutSyntheticPercentage(t *testing.T) {
	m := newSetupOnlyShell()
	m.progress.Width = 24
	completed := lifecycle.Item{Provider: "box", Status: lifecycle.StatusPresent}
	active := lifecycle.Item{
		Provider: "salesforce",
		Status:   lifecycle.StatusMissing,
		Planned:  []string{"Managed Package:Box for Salesforce", "ApexClass:CLMController"},
	}
	boxRow := m.focusedProviderRow("box", 1, &completed, false, "validate", 80)
	salesforceRow := m.focusedProviderRow("salesforce", 0.39, &active, true, "validate", 80)

	if !strings.Contains(boxRow, "COMPLETE") || strings.Contains(boxRow, "DEPLOYMENT CHECKLIST") {
		t.Fatalf("completed provider did not collapse to a summary:\n%s", boxRow)
	}
	for _, want := range []string{"Salesforce", "IN PROGRESS", "Checking 2 configuration components"} {
		if !strings.Contains(salesforceRow, want) {
			t.Fatalf("active provider omitted %q:\n%s", want, salesforceRow)
		}
	}
	if strings.Contains(salesforceRow, "%") || strings.Contains(salesforceRow, "DEPLOYMENT CHECKLIST") {
		t.Fatalf("focused provider exposed synthetic precision or checklist detail:\n%s", salesforceRow)
	}
}

func TestActiveDeployKeepsOneVisualFocusAtSupportedSizes(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.deploymentPhase, m.deployStarted = screenDeploy, deploymentPhaseApply, true
	m.currentDeployment = "box"
	m.deploymentProgress = map[string]float64{"box": 0.42, "salesforce": 0}
	m.validationItems = []lifecycle.Item{
		{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true, DeployableComponents: []string{"Folder:Workspace", "File:README.md"}},
		{Provider: "salesforce", Status: lifecycle.StatusMissing, Deployable: true, DeployableComponents: []string{"ApexClass:CLMController"}},
	}
	m.activityLog = []string{"Preparing Box", "Creating workspace", "Uploading README.md"}

	for _, size := range []struct{ width, height int }{{80, 24}, {100, 30}, {120, 40}} {
		resized := m
		resized.width, resized.height = size.width, size.height
		view := resized.View()
		if lines := strings.Count(view, "\n") + 1; lines > size.height {
			t.Fatalf("active deployment overflows %dx%d: %d lines", size.width, size.height, lines)
		}
		for _, want := range []string{"Box", "IN PROGRESS", "CURRENT TASK", "Uploading README.md", "e activity"} {
			if !strings.Contains(view, want) {
				t.Fatalf("active deployment at %dx%d omitted %q:\n%s", size.width, size.height, want, view)
			}
		}
		for _, hidden := range []string{"Preparing Box", "Creating workspace", "Working", "e to expand"} {
			if strings.Contains(view, hidden) {
				t.Fatalf("active deployment at %dx%d exposed redundant content %q:\n%s", size.width, size.height, hidden, view)
			}
		}
	}
}

func TestDeployConfirmationUsesSingleKeyboardGuidanceLocation(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.confirmingDeploy, m.deploymentPhase = screenDeploy, true, deploymentPhaseReview
	m.deployConfirmTitle, m.deployConfirmDesc, m.deployConfirmAffirm = "Deploy this solution?", "Apply the reviewed plan.", "Deploy"
	m.width, m.height = 120, 40
	view := m.View()
	if got := strings.Count(view, "enter/space"); got != 1 {
		t.Fatalf("confirmation keyboard guidance appears %d times, want once:\n%s", got, view)
	}
}

func TestBrowserOpenCommands(t *testing.T) {
	tests := map[string][]string{
		"darwin":  {"open", boxAdminConsoleURL},
		"linux":   {"xdg-open", boxAdminConsoleURL},
		"windows": {"rundll32", "url.dll,FileProtocolHandler", boxAdminConsoleURL},
	}
	for goos, want := range tests {
		cmd := browserOpenCommand(goos, boxAdminConsoleURL)
		if cmd == nil || !slices.Equal(cmd.Args, want) {
			t.Fatalf("%s open command = %#v, want %#v", goos, cmd, want)
		}
	}
	if cmd := browserOpenCommand("plan9", boxAdminConsoleURL); cmd != nil {
		t.Fatalf("unsupported operating system returned command %#v", cmd.Args)
	}
}

func TestValidationAndDeployChecklistsStayInsideTerminal(t *testing.T) {
	m := newSetupOnlyShell()
	m.components[1].selected = true
	m.width, m.height = 120, 44
	m.validateDone = true
	m.validationProgress = map[string]float64{"box": 1, "salesforce": 1}

	missing := make([]string, 36)
	order := make([]string, 36)
	for i := range missing {
		order[i] = fmt.Sprintf("MetadataType%02d", i)
		missing[i] = order[i] + ":Example"
	}
	m.validationItems = []lifecycle.Item{
		{Provider: "box", Status: lifecycle.StatusPresent, Detail: "Box configuration is present."},
		{
			Provider: "salesforce", Status: lifecycle.StatusMissing, Detail: "36 components need deployment.", Deployable: true,
			Missing: missing, DeployableComponents: append([]string(nil), missing...), ComponentOrder: order,
		},
	}

	assertFitsAndScrolls := func(screen shellScreen, confirm bool) {
		t.Helper()
		m.screen, m.confirmingDeploy, m.deployShowDetails, m.lifecycleScroll = screen, confirm, screen == screenDeploy, 0
		view := m.View()
		if lines := strings.Count(view, "\n") + 1; lines > m.height {
			t.Fatalf("screen %v overflows terminal: %d lines > height %d", screen, lines, m.height)
		}
		if !strings.Contains(view, "↑/↓ scroll") {
			t.Fatalf("screen %v did not expose the bounded checklist viewport", screen)
		}

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		down := updated.(rootShellModel)
		if down.lifecycleScroll == 0 {
			t.Fatalf("screen %v did not scroll checklist down", screen)
		}
		updated, _ = down.Update(tea.KeyMsg{Type: tea.KeyUp})
		if up := updated.(rootShellModel).lifecycleScroll; up != down.lifecycleScroll-1 {
			t.Fatalf("screen %v scroll up did not reverse down: down=%d up=%d", screen, down.lifecycleScroll, up)
		}
	}

	assertFitsAndScrolls(screenValidate, false)
	assertFitsAndScrolls(screenDeploy, true)
}

func TestSalesforceValidationChecklistStopsAtLastRow(t *testing.T) {
	m := newSetupOnlyShell()
	m.components[1].selected = true
	m.screen = screenValidate
	m.width, m.height = 120, 36
	m.validateDone = true
	m.validationProgress = map[string]float64{"box": 1, "salesforce": 1}

	missing := make([]string, 36)
	order := make([]string, 36)
	for i := range missing {
		order[i] = fmt.Sprintf("MetadataType%02d", i)
		missing[i] = order[i] + ":Example"
	}
	m.validationItems = []lifecycle.Item{
		{Provider: "box", Status: lifecycle.StatusPresent, Detail: "Box configuration is present."},
		{
			Provider: "salesforce", Status: lifecycle.StatusMissing, Detail: "36 components need deployment.", Deployable: true,
			Missing: missing, DeployableComponents: append([]string(nil), missing...), ComponentOrder: order,
		},
	}

	var model tea.Model = m
	for range 100 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	atEnd := model.(rootShellModel)
	if atEnd.lifecycleScroll == 0 {
		t.Fatal("Salesforce checklist returned to its first row after repeated Down presses")
	}
	endView := atEnd.View()
	if !strings.Contains(endView, "MetadataType35") || strings.Contains(endView, "MetadataType00") {
		t.Fatalf("checklist did not stop on its final rows:\n%s", endView)
	}
	if !strings.Contains(endView, "end of checklist") {
		t.Fatalf("final viewport does not identify the checklist boundary:\n%s", endView)
	}

	model, _ = atEnd.Update(tea.KeyMsg{Type: tea.KeyDown})
	afterExtraDown := model.(rootShellModel)
	if afterExtraDown.lifecycleScroll != atEnd.lifecycleScroll || afterExtraDown.View() != endView {
		t.Fatalf("Down at the final row moved or wrapped the checklist: before=%d after=%d", atEnd.lifecycleScroll, afterExtraDown.lifecycleScroll)
	}
	if !afterExtraDown.lifecycleFollowTail {
		t.Fatal("reaching the final row did not resume live-tail behavior")
	}
}

func TestClampLifecycleScrollNeverWraps(t *testing.T) {
	if got := clampLifecycleScroll(7, 1, 7); got != 7 {
		t.Fatalf("Down at end = %d, want 7", got)
	}
	if got := clampLifecycleScroll(0, -1, 7); got != 0 {
		t.Fatalf("Up at start = %d, want 0", got)
	}
	if got := clampLifecycleScroll(99, 1, 7); got != 7 {
		t.Fatalf("resized out-of-range offset = %d, want 7", got)
	}
}

func TestLiveValidationFollowsTailUntilOperatorScrollsUp(t *testing.T) {
	m := newSetupOnlyShell()
	m.components[1].selected = true
	m.screen = screenValidate
	m.width, m.height = 120, 34
	m.validateRunning = true
	m.lifecycleFollowTail = true
	m.currentValidation = "salesforce"
	m.validationProgress = map[string]float64{"box": 1, "salesforce": 0.05}

	missing := make([]string, 36)
	order := make([]string, 36)
	for i := range missing {
		order[i] = fmt.Sprintf("MetadataType%02d", i)
		missing[i] = order[i] + ":Example"
	}
	m.validationItems = []lifecycle.Item{
		{Provider: "box", Status: lifecycle.StatusPresent, Detail: "Box configuration is present."},
		{
			Provider: "salesforce", Status: lifecycle.StatusMissing, Detail: "Validating Salesforce metadata.", Deployable: true,
			Missing: missing, DeployableComponents: append([]string(nil), missing...), ComponentOrder: order,
		},
	}
	m.followLifecycleTail()
	limit, ok := m.lifecycleScrollLimit()
	initialOffset := m.lifecycleScroll
	if !ok || initialOffset >= limit {
		t.Fatalf("initial active-row follow = %d, limit = %d, ok = %v", initialOffset, limit, ok)
	}

	updated, _ := m.Update(activityMsg{provider: "salesforce", line: "Reading Salesforce metadata"})
	tailing := updated.(rootShellModel)
	if tailing.lifecycleScroll >= limit {
		t.Fatalf("early activity jumped to the pending tail: scroll=%d limit=%d", tailing.lifecycleScroll, limit)
	}
	for range 12 {
		updated, _ = tailing.Update(providerValidationProgressMsg{provider: "salesforce"})
		tailing = updated.(rootShellModel)
	}
	if tailing.lifecycleScroll <= initialOffset || tailing.lifecycleScroll >= limit {
		t.Fatalf("active-row follow did not advance with progress: initial=%d current=%d limit=%d", initialOffset, tailing.lifecycleScroll, limit)
	}

	updated, _ = tailing.Update(tea.KeyMsg{Type: tea.KeyUp})
	paused := updated.(rootShellModel)
	if paused.lifecycleFollowTail || paused.lifecycleScroll >= tailing.lifecycleScroll {
		t.Fatalf("Up did not pause live tail: before=%d after=%d follow=%v", tailing.lifecycleScroll, paused.lifecycleScroll, paused.lifecycleFollowTail)
	}
	pausedOffset := paused.lifecycleScroll
	updated, _ = paused.Update(activityMsg{provider: "salesforce", line: "Comparing packaged components"})
	paused = updated.(rootShellModel)
	if paused.lifecycleScroll != pausedOffset {
		t.Fatalf("new activity moved a manually paused checklist: before=%d after=%d", pausedOffset, paused.lifecycleScroll)
	}

	for range 100 {
		updated, _ = paused.Update(tea.KeyMsg{Type: tea.KeyDown})
		paused = updated.(rootShellModel)
	}
	limit, _ = paused.lifecycleScrollLimit()
	if paused.lifecycleScroll != limit || !paused.lifecycleFollowTail {
		t.Fatalf("returning to bottom did not resume tail: scroll=%d limit=%d follow=%v", paused.lifecycleScroll, limit, paused.lifecycleFollowTail)
	}
}

func TestSalesforceChecklistOrderIsStableWithoutConfiguredOrder(t *testing.T) {
	item := lifecycle.Item{
		Provider:             "salesforce",
		Status:               lifecycle.StatusMissing,
		Missing:              []string{"Layout:Demo", "ApexClass:Demo", "CustomObject:Demo", "PermissionSet:Demo", "CustomField:Demo.Field__c"},
		DeployableComponents: []string{"Layout:Demo", "ApexClass:Demo", "CustomObject:Demo", "PermissionSet:Demo", "CustomField:Demo.Field__c"},
	}
	wantOrder := []string{"ApexClass", "CustomField", "CustomObject", "Layout", "PermissionSet"}
	first := renderComponentChecklist(item, "validate", false, 1, "", 100)
	previous := -1
	for _, name := range wantOrder {
		index := strings.Index(first, name)
		if index <= previous {
			t.Fatalf("Salesforce checklist is not in deterministic fallback order: %q", first)
		}
		previous = index
	}
	for range 50 {
		if got := renderComponentChecklist(item, "validate", false, 1, "", 100); got != first {
			t.Fatalf("Salesforce checklist order changed between renders:\nfirst: %q\nnext:  %q", first, got)
		}
	}
}

func TestSalesforceManagedPackageIsFirstDeploymentChecklistRow(t *testing.T) {
	item := lifecycle.Item{
		Provider:             "salesforce",
		Status:               lifecycle.StatusMissing,
		ComponentOrder:       []string{"Managed Package"},
		Missing:              []string{"ApexClass:Demo", "Managed Package:Box for Salesforce 5.43", "CustomObject:Demo__c"},
		DeployableComponents: []string{"ApexClass:Demo", "Managed Package:Box for Salesforce 5.43", "CustomObject:Demo__c"},
	}
	checklist := renderComponentChecklist(item, "deploy", false, 0, "", 100)
	packageIndex := strings.Index(checklist, "Managed Package")
	apexIndex := strings.Index(checklist, "ApexClass")
	objectIndex := strings.Index(checklist, "CustomObject")
	if packageIndex < 0 || packageIndex > apexIndex || packageIndex > objectIndex {
		t.Fatalf("managed package is not the first deployment prerequisite:\n%s", checklist)
	}
}

func TestDeployedAssetsTableHeader(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.deployDone, m.width, m.height = screenDeploy, true, 120, 50
	m.validationItems = []lifecycle.Item{{Provider: "box", Resources: []lifecycle.ResourceReference{
		{Provider: "box", Component: "Sample Content:msa.pdf", Kind: "file", Name: "msa.pdf", ID: "999"},
	}}}
	table := m.renderDeployedAssetsTable(112, m.deployTableCapacity())
	if strings.Contains(table, "KIND") {
		t.Error("column header should be TYPE, not KIND")
	}
	if !strings.Contains(table, "TYPE") {
		t.Error("expected TYPE column header")
	}
	if !strings.Contains(table, "msa.pdf") {
		t.Errorf("file name missing from table:\n%s", table)
	}
}

func TestActivityFeedStreamsAndExpands(t *testing.T) {
	m := newSetupOnlyShell()
	m.screen, m.deployStarted, m.width, m.height = screenDeploy, true, 120, 50
	m.activityCh = make(chan tea.Msg, 8)

	// Each streamed step appends to the log and becomes the latest line.
	var model tea.Model = m
	for _, step := range []string{"Ensuring the Box workspace folder tree", "Uploading sample content msa.pdf", "Creating AI agent Risk Triage"} {
		model, _ = model.Update(activityMsg{provider: "box", line: step})
	}
	rm := model.(rootShellModel)
	if len(rm.activityLog) != 3 {
		t.Fatalf("activityLog = %d, want 3", len(rm.activityLog))
	}

	// The footer must not echo a step line while the feed is live (that was the
	// duplicate output at the bottom of the deploy/reset screens).
	rm.message = "Creating AI agent Risk Triage"
	if strings.Contains(rm.footer(), "Creating AI agent Risk Triage") {
		t.Fatalf("footer duplicated the activity line while the feed is active:\n%s", rm.footer())
	}

	// Collapsed: shows only the current task. Interaction guidance stays in the footer.
	collapsed := rm.renderActivity(100)
	if !strings.Contains(collapsed, "CURRENT TASK") || !strings.Contains(collapsed, "Creating AI agent Risk Triage") {
		t.Fatalf("collapsed feed missing its label or latest line:\n%s", collapsed)
	}
	for _, hidden := range []string{"Ensuring the Box workspace folder tree", "Uploading sample content msa.pdf", "e to expand", "Working"} {
		if strings.Contains(collapsed, hidden) {
			t.Fatalf("collapsed feed retained redundant content %q:\n%s", hidden, collapsed)
		}
	}
	if footer := rm.footer(); !strings.Contains(footer, "e activity") {
		t.Fatalf("collapsed activity action missing from footer:\n%s", footer)
	}

	// `e` expands to the full recent history.
	model, _ = rm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	rm = model.(rootShellModel)
	if !rm.activityExpanded {
		t.Fatal("e did not expand the activity feed")
	}
	if expanded := rm.renderActivity(100); !strings.Contains(expanded, "ACTIVITY · 3 STEPS") || !strings.Contains(expanded, "Ensuring the Box workspace folder tree") || strings.Contains(expanded, "e to collapse") {
		t.Fatalf("expanded feed has the wrong hierarchy or duplicated guidance:\n%s", expanded)
	}
	if footer := rm.footer(); !strings.Contains(footer, "e collapse activity") {
		t.Fatalf("expanded activity action missing from footer:\n%s", footer)
	}
}

func TestComponentCountUsesNaturalGrammar(t *testing.T) {
	if got := componentCount(1); got != "1 configuration component" {
		t.Fatalf("singular count = %q", got)
	}
	if got := componentCount(18); got != "18 configuration components" {
		t.Fatalf("plural count = %q", got)
	}
}

func TestTeardownResultRowsSummary(t *testing.T) {
	m := newSetupOnlyShell()
	m.width, m.height = 120, 50
	m.teardownResults = []lifecycle.TeardownResult{
		{Provider: "box", Outcomes: []lifecycle.TeardownOutcome{
			{Resource: lifecycle.ResourceReference{Provider: "box", Kind: "folder", Name: "Workspace", ID: "1"}, Deleted: true},
			{Resource: lifecycle.ResourceReference{Provider: "box", Kind: "automate_workflow", Name: "Intake", ID: "2"}, Unmanaged: true},
			{Resource: lifecycle.ResourceReference{Provider: "box", Kind: "hub", Name: "CLM", ID: "3"}, Error: "boom"},
		}},
	}
	rows := strings.Join(m.teardownResultRows(112), "\n")
	for _, want := range []string{"1 deleted", "2 remaining", "STATUS", "TYPE", "✓ deleted", "○ manual", "× failed", "boom"} {
		if !strings.Contains(rows, want) {
			t.Errorf("teardown result table missing %q:\n%s", want, rows)
		}
	}
}

func TestChromeFramingAndStepperFit(t *testing.T) {
	for _, w := range []int{80, 100, 112, 140} {
		m := newSetupOnlyShell()
		m.screen, m.width, m.height = screenDeploy, w, 50
		content := min(max(w-8, 64), 112)

		// The stepper track must never exceed the content width (else it wraps and
		// breaks the frame).
		if got := lipgloss.Width(m.stepper()); got > content {
			t.Errorf("width=%d: stepper %d cols > content %d", w, got, content)
		}
		// Header is a row plus a hairline rule spanning the content width.
		hl := strings.Split(m.header(content), "\n")
		if len(hl) != 2 || lipgloss.Width(hl[1]) != content {
			t.Errorf("width=%d: header not framed to content width: %d lines, rule %d cols", w, len(hl), lipgloss.Width(hl[len(hl)-1]))
		}
		if !strings.Contains(m.header(content), "UNOFFICIALBOX.DEV") {
			t.Errorf("width=%d: header lost its wordmark", w)
		}
		// Footer carries a matching top rule.
		if fl := strings.Split(m.footer(), "\n"); lipgloss.Width(fl[0]) != content {
			t.Errorf("width=%d: footer rule %d cols, want %d", w, lipgloss.Width(fl[0]), content)
		}
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
	actions := model.providerActions()
	if !containsStr(actions, "ccg") {
		t.Fatal("Box should offer the CCG connect action")
	}
	if containsStr(actions, "connect") || containsStr(actions, "switch") {
		t.Fatalf("Box should not offer removed Box CLI actions: %v", actions)
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
	updated, cmd := model.saveBoxCCG()
	model = updated.(rootShellModel)
	if model.screen != screenProvider {
		t.Fatalf("after save screen = %v, want provider", model.screen)
	}
	if cmd == nil || model.statuses["box"] != connectionChecking {
		t.Fatalf("new Box connection was not verified immediately: status=%v cmd=%v", model.statuses["box"], cmd)
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

func TestDeployedAssetsIncludesExistingConfiguration(t *testing.T) {
	m := rootShellModel{validationItems: []lifecycle.Item{
		{
			Provider: "box",
			Resources: []lifecycle.ResourceReference{
				{Provider: "box", Component: "Workspace:CLM", Kind: "folder", Name: "CLM", ID: "1"},
			},
			// Configuration already in the tenant: created earlier, so the deploy
			// records no ID, but it must still show as an existing row.
			Present: []string{"Metadata Template:clmDocument", "Doc Gen Template:contract.docx"},
		},
	}}
	assets := m.deployedAssets()
	if len(assets) != 3 {
		t.Fatalf("deployedAssets = %d, want 3 (1 created + 2 existing): %+v", len(assets), assets)
	}
	var created, existing int
	kinds := map[string]string{}
	for _, a := range assets {
		if a.created {
			created++
		} else {
			existing++
		}
		kinds[a.ref.Component] = a.ref.Kind
	}
	if created != 1 || existing != 2 {
		t.Fatalf("created=%d existing=%d, want 1 and 2", created, existing)
	}
	if kinds["Metadata Template:clmDocument"] != "metadata_template" {
		t.Fatalf("existing metadata kind = %q, want metadata_template", kinds["Metadata Template:clmDocument"])
	}
}

func TestDeployedAssetsDoesNotDuplicateCreatedComponents(t *testing.T) {
	// A component recorded as created must not also appear as an existing row,
	// even if it lingers in the Present list.
	m := rootShellModel{validationItems: []lifecycle.Item{{
		Provider:  "box",
		Resources: []lifecycle.ResourceReference{{Provider: "box", Component: "Box Hub:CLM", Kind: "hub", Name: "CLM", ID: "9"}},
		Present:   []string{"Box Hub:CLM"},
	}}}
	if got := m.deployedAssets(); len(got) != 1 || !got[0].created {
		t.Fatalf("deployedAssets = %+v, want a single created row", got)
	}
}
