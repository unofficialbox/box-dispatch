package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	deploymentaudit "github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
	"github.com/unofficialbox/box-dispatch/internal/workspace"
)

// welcomeOptions is the home menu; the key handler and the view read the same
// slice so the cursor range never drifts from what is rendered.
var welcomeOptions = []string{"Start new deployment", "Show deployment history", "Reset demo environment"}

type shellScreen int

const (
	screenWelcome shellScreen = iota
	screenHistory
	screenComponents
	screenTemplates
	screenDashboard
	screenProvider
	screenOptions
	screenDatabricksHost
	screenConfig
	screenBoxComponents
	screenDirectoryPicker
	screenPackage
	screenValidate
	screenDeploy
	screenTeardown
	screenBoxCCG
	screenBoxSwitch
)

type connectionStatus int

const (
	connectionPending connectionStatus = iota
	connectionChecking
	connectionConnected
	connectionFailed
)

type componentChoice struct {
	provider string
	name     string
	role     string
	selected bool
	required bool
}

type templateChoice struct {
	id          string
	name        string
	sector      string
	description string
	repository  string
}

type checkFinishedMsg struct {
	provider string
	result   checker.ProviderResult
	err      error
}

type externalFinishedMsg struct {
	provider string
	err      error
}

type packageFinishedMsg struct {
	manifest workspace.PackageManifest
	err      error
}

type providerValidationFinishedMsg struct {
	provider string
	item     lifecycle.Item
	err      error
}

type providerValidationProgressMsg struct {
	provider string
}

type providerDeployFinishedMsg struct {
	provider string
	item     lifecycle.Item
	err      error
}

type providerDeployProgressMsg struct {
	provider string
}

type providerTeardownFinishedMsg struct {
	provider string
	result   lifecycle.TeardownResult
	err      error
}

type providerTeardownProgressMsg struct {
	provider string
}

type wizardAnswers struct {
	components         []string
	templateID         string
	directory          string
	packageName        string
	deploymentStrategy string
}

type contextualHelp []key.Binding

func (k contextualHelp) ShortHelp() []key.Binding  { return []key.Binding(k) }
func (k contextualHelp) FullHelp() [][]key.Binding { return [][]key.Binding{[]key.Binding(k)} }

var (
	navy       = lipgloss.Color("#0B172A")
	cyan       = lipgloss.Color("#0866D9")
	ice        = lipgloss.Color("#D8EAFF")
	coral      = lipgloss.Color("#FF6658")
	gold       = lipgloss.Color("#F7C95C")
	green      = lipgloss.Color("#67C587")
	muted      = lipgloss.Color("#8A9AAF")
	white      = lipgloss.Color("#FFFEFA")
	panel      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#52637A")).Padding(1, 2)
	activePane = panel.Copy().BorderForeground(coral).Background(navy)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(white)
	dimStyle   = lipgloss.NewStyle().Foreground(muted)
	accent     = lipgloss.NewStyle().Bold(true).Foreground(cyan)
)

func dispatchHuhTheme() *huh.Theme {
	theme := huh.ThemeCharm()
	focusedButton := theme.Focused.FocusedButton.Foreground(white).Background(cyan).Bold(true)
	blurredButton := theme.Focused.BlurredButton.Foreground(ice).Background(lipgloss.Color("#172235"))
	theme.Focused.FocusedButton = focusedButton
	theme.Focused.BlurredButton = blurredButton
	theme.Focused.Next = focusedButton
	theme.Blurred.FocusedButton = focusedButton
	theme.Blurred.BlurredButton = blurredButton
	return theme
}

type rootShellModel struct {
	screen                shellScreen
	width                 int
	height                int
	cursor                int
	templateCursor        int
	optionCursor          int
	provider              string
	components            []componentChoice
	templates             []templateChoice
	selected              *templateChoice
	statuses              map[string]connectionStatus
	results               map[string]checker.ProviderResult
	queue                 []string
	spinner               spinner.Model
	hostInput             textinput.Model
	progress              progress.Model
	help                  help.Model
	answers               *wizardAnswers
	componentForm         *huh.Form
	templateForm          *huh.Form
	directoryInput        textinput.Model
	packageInput          textinput.Model
	directoryPicker       filepicker.Model
	configFocus           int
	boxCapabilities       []solution.Capability
	boxComponentMode      string
	boxComponentValues    map[string]bool
	packagePath           string
	packageStarted        bool
	packageDone           bool
	validateStarted       bool
	validateRunning       bool
	validateDone          bool
	validationQueue       []string
	validationProgress    map[string]float64
	currentValidation     string
	deployStarted         bool
	deployDone            bool
	deployApproved        *bool
	confirmingDeploy      bool
	deployConfirmForm     *huh.Form
	deploymentQueue       []lifecycle.Item
	deploymentProgress    map[string]float64
	currentDeployment     string
	deploymentBaseline    []lifecycle.Item
	deploymentStartedAt   time.Time
	deploymentCompletedAt time.Time
	deploymentAuditPath   string
	deploymentHistory     []deploymentaudit.DeploymentRecord
	historyError          string
	validationItems       []lifecycle.Item
	message               string

	// Teardown ("reset the demo environment") state. The record is the resource
	// inventory the reset deletes from; nothing outside it is ever touched.
	teardownRecord       *deploymentaudit.DeploymentRecord
	teardownProviders    []string
	teardownQueue        []string
	teardownResults      []lifecycle.TeardownResult
	teardownProgress     map[string]float64
	currentTeardown      string
	teardownStarted      bool
	teardownDone         bool
	confirmingTeardown   bool
	teardownConfirmForm  *huh.Form
	teardownConfirmation *string
	teardownError        string

	// Box CCG credential entry.
	boxCCGForm      *huh.Form
	ccgClientID     string
	ccgClientSecret string
	ccgSubjectType  string
	ccgSubjectID    string

	// Box connection switcher.
	boxConnections []boxconn.Connection
}

func newSetupOnlyShell(scopedProvider ...string) rootShellModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(cyan)
	host := textinput.New()
	host.Placeholder = "https://dbc-....cloud.databricks.com"
	host.Prompt = "  Workspace URL  "
	host.CharLimit = 300
	bar := progress.New(
		progress.WithGradient("#0866D9", "#FF6658"),
		progress.WithWidth(52),
	)
	helpModel := help.New()
	helpModel.ShortSeparator = "  •  "
	helpModel.Styles.ShortKey = lipgloss.NewStyle().Bold(true).Foreground(cyan)
	helpModel.Styles.ShortDesc = lipgloss.NewStyle().Foreground(muted)
	helpModel.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("#52637A"))

	// BCL runtime config is the source of truth for scenarios and providers.
	// When it is absent (setup has not run) the shell falls back to built-in copy.
	runtime, _ := config.LoadRuntimeConfig()
	components := componentsFromRuntime(runtime)
	if len(components) == 0 {
		components = defaultComponents()
	}
	provider := ""
	if len(scopedProvider) > 0 {
		provider = scopedProvider[0]
	}
	if provider != "" {
		for i := range components {
			components[i].selected = components[i].provider == "box" || components[i].provider == provider
		}
	}

	selectedComponents := make([]string, 0, len(components))
	for _, component := range components {
		if component.selected {
			selectedComponents = append(selectedComponents, component.provider)
		}
	}

	templates := templatesFromRuntime(runtime)
	if len(templates) == 0 {
		templates = defaultTemplates()
	}
	// The "new solution" starter is a built-in affordance, not a configured scenario.
	templates = append(templates, newSolutionStarter())

	activeTemplateID := "clm"
	if runtime != nil && runtime.ActiveScenario != "" {
		if _, ok := runtime.Scenarios[runtime.ActiveScenario]; ok {
			activeTemplateID = runtime.ActiveScenario
		}
	}

	cwd, _ := os.Getwd()
	answers := &wizardAnswers{
		components:         selectedComponents,
		templateID:         activeTemplateID,
		directory:          filepath.Dir(cwd),
		packageName:        packageNameForTemplate(templates, activeTemplateID),
		deploymentStrategy: solution.StrategyCreateNew,
	}
	m := rootShellModel{
		screen:             screenWelcome,
		components:         components,
		templates:          templates,
		statuses:           map[string]connectionStatus{},
		results:            map[string]checker.ProviderResult{},
		validationProgress: map[string]float64{},
		deploymentProgress: map[string]float64{},
		spinner:            spin,
		hostInput:          host,
		progress:           bar,
		help:               helpModel,
		answers:            answers,
	}
	m.rebuildComponentForm()
	m.rebuildTemplateForm()
	m.prepareConfigInputs()
	m.prepareBoxComponentSelection()
	return m
}

func newDispatchShell() rootShellModel { return newSetupOnlyShell() }

// componentsFromRuntime builds the component picker from the active BCL scenario,
// translating BCL provider IDs to internal keys and filling display copy from the
// provider config (falling back to built-in copy for anything the config omits).
// componentsFromRuntime returns the platforms box-dispatch supports for the
// BUILD picker. Availability is fixed (the tool checks and deploys exactly these
// providers) and independent of the active scenario, which is chosen later at the
// TEMPLATE step; the BCL providers map only enriches each platform's display copy.
func componentsFromRuntime(cfg *config.RuntimeConfig) []componentChoice {
	components := defaultComponents()
	if cfg == nil {
		return components
	}
	for i := range components {
		bclID := config.BCLProviderID(components[i].provider)
		pc, ok := cfg.Providers[bclID]
		if !ok {
			continue
		}
		if pc.DisplayName != "" {
			components[i].name = pc.DisplayName
		}
		if pc.Role != "" {
			components[i].role = pc.Role
		}
	}
	return components
}

// templatesFromRuntime builds the template list from the BCL scenarios, ordered
// with the active scenario first and the rest alphabetically for determinism.
func templatesFromRuntime(cfg *config.RuntimeConfig) []templateChoice {
	if cfg == nil || len(cfg.Scenarios) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.Scenarios))
	for id := range cfg.Scenarios {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		switch {
		case a == cfg.ActiveScenario:
			return -1
		case b == cfg.ActiveScenario:
			return 1
		default:
			return strings.Compare(a, b)
		}
	})
	// Built-in copy fills fields a minimal or imported config omits — notably
	// the repository, without which packaging cannot clone the template.
	fallback := map[string]templateChoice{}
	for _, t := range defaultTemplates() {
		fallback[t.id] = t
	}
	templates := make([]templateChoice, 0, len(ids))
	for _, id := range ids {
		scenario := cfg.Scenarios[id]
		fb := fallback[id]
		name := firstNonEmptyString(scenario.DisplayName, fb.name, id)
		templates = append(templates, templateChoice{
			id:          id,
			name:        name,
			sector:      firstNonEmptyString(scenario.Sector, fb.sector),
			description: firstNonEmptyString(scenario.Description, fb.description),
			repository:  firstNonEmptyString(scenario.Repository, fb.repository),
		})
	}
	return templates
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultComponents() []componentChoice {
	return []componentChoice{
		{provider: "box", name: "Box", role: "Content, unstructured data, and AI", selected: true, required: true},
		{provider: "salesforce", name: "Agentforce", role: "Structured data, human experience, and agents"},
		{provider: "databricks", name: "Databricks", role: "Analytics, models, and data intelligence"},
		{provider: "aws", name: "AWS Bedrock AgentCore", role: "Agent runtime and orchestration"},
	}
}

func defaultTemplates() []templateChoice {
	return []templateChoice{
		{id: "clm", name: "Contract Lifecycle Management", sector: "LEGAL OPERATIONS", description: "Content-centric contract workflows with Box and intelligent agents.", repository: "https://github.com/unofficialbox/box-bedrock-for-clm"},
		{id: "lifesciences", name: "Life Sciences", sector: "REGULATED CONTENT", description: "Accelerate document-heavy life sciences processes and insight.", repository: "https://github.com/unofficialbox/box-bedrock-for-lifesciences"},
		{id: "citizen-services", name: "Citizen Services", sector: "PUBLIC SECTOR", description: "Modernize constituent intake, case content, and service delivery.", repository: "https://github.com/unofficialbox/box-bedrock-for-citizen-services"},
	}
}

// newSolutionStarter is the built-in "create your own" affordance appended after
// the BCL-configured scenarios.
func newSolutionStarter() templateChoice {
	return templateChoice{id: "new", name: "Create a New Solution", sector: "STARTER", description: "Begin with the Box Dispatch reference architecture and shape your own solution.", repository: "https://github.com/unofficialbox/box-bedrock-template"}
}

// packageNameForTemplate derives the default package directory name from a
// template's repository basename, falling back to the CLM default.
func packageNameForTemplate(templates []templateChoice, id string) string {
	for _, t := range templates {
		if t.id == id && t.repository != "" {
			return filepath.Base(t.repository)
		}
	}
	return "box-bedrock-for-clm"
}

func (m *rootShellModel) rebuildComponentForm() {
	options := make([]huh.Option[string], 0, len(m.components))
	for _, component := range m.components {
		label := component.name
		if component.required {
			label += "  ·  REQUIRED"
		}
		options = append(options, huh.NewOption(label, component.provider).Selected(slices.Contains(m.answers.components, component.provider)))
	}
	m.componentForm = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Key("components").
				Title("Select platform components").
				Description("Box is required; partner platforms are optional.").
				Options(options...).
				Value(&m.answers.components).
				Validate(func(values []string) error {
					if !slices.Contains(values, "box") {
						return errors.New("Box is required for every Box Dispatch solution")
					}
					return nil
				}),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithWidth(76)
}

func (m *rootShellModel) rebuildTemplateForm() {
	options := make([]huh.Option[string], 0, len(m.templates))
	for _, template := range m.templates {
		options = append(options, huh.NewOption(template.name, template.id))
	}
	m.templateForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("template").
				Title("Choose an industry quickstart").
				Description("Start from a proven architecture or the Box Dispatch solution template.").
				Options(options...).
				Value(&m.answers.templateID),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithWidth(76)
}

func (m *rootShellModel) prepareConfigInputs() {
	directory := textinput.New()
	directory.Prompt = "  "
	directory.SetValue(m.answers.directory)
	directory.CharLimit = 500
	directory.Width = 64

	name := textinput.New()
	name.Prompt = "  "
	name.SetValue(m.answers.packageName)
	name.CharLimit = 120
	name.Width = 48

	picker := filepicker.New()
	picker.CurrentDirectory = m.answers.directory
	picker.DirAllowed = true
	picker.FileAllowed = false
	picker.ShowHidden = false
	picker.ShowPermissions = false
	picker.ShowSize = false
	picker.AutoHeight = false
	picker.Height = 14
	picker.KeyMap.Select.SetKeys(" ")
	picker.KeyMap.Select.SetHelp("space", "choose folder")

	m.directoryInput = directory
	m.packageInput = name
	m.directoryPicker = picker
	m.setConfigFocus(0)
}

func (m *rootShellModel) setConfigFocus(index int) {
	m.configFocus = index
	m.directoryInput.Blur()
	m.packageInput.Blur()
	if index == 0 {
		m.directoryInput.Focus()
	} else if index == 1 {
		m.packageInput.Focus()
	}
}

func (m *rootShellModel) prepareBoxComponentSelection() {
	m.boxComponentMode = "defaults"
	m.boxComponentValues = map[string]bool{}
	m.boxCapabilities = nil
	manifest, err := solution.LoadBundled(m.answers.templateID)
	if err != nil {
		return
	}
	m.boxCapabilities = append([]solution.Capability(nil), manifest.Box.Capabilities...)
	for _, capability := range m.boxCapabilities {
		enabled := capability.EnabledByDefault == nil || *capability.EnabledByDefault
		m.boxComponentValues[manifest.CapabilityID(capability)] = enabled
	}
}

func (m rootShellModel) boxComponentSelection() solution.ComponentSelection {
	values := map[string]bool{}
	for id, enabled := range m.boxComponentValues {
		values[id] = enabled
	}
	return solution.ComponentSelection{Mode: m.boxComponentMode, Selections: values}
}

func (m rootShellModel) openDirectoryPicker() (tea.Model, tea.Cmd) {
	directory := strings.TrimSpace(m.directoryInput.Value())
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		m.message = "Enter an existing parent directory before browsing."
		return m, nil
	}
	m.answers.directory = directory
	m.directoryPicker.CurrentDirectory = directory
	m.screen = screenDirectoryPicker
	m.message = "Enter/→ opens a folder · ← goes up · Space chooses the highlighted folder · c chooses this folder"
	return m, m.directoryPicker.Init()
}

func (m rootShellModel) updateDirectoryPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			m.screen, m.message = screenWelcome, ""
			return m, nil
		case "esc":
			m.screen = screenConfig
			m.message = "Folder selection cancelled."
			return m, nil
		case "c":
			return m.chooseDirectory(m.directoryPicker.CurrentDirectory)
		}
	}
	if selected, path := m.directoryPicker.DidSelectFile(msg); selected {
		return m.chooseDirectory(path)
	}
	var cmd tea.Cmd
	m.directoryPicker, cmd = m.directoryPicker.Update(msg)
	return m, cmd
}

func (m rootShellModel) chooseDirectory(path string) (tea.Model, tea.Cmd) {
	path = filepath.Clean(path)
	m.answers.directory = path
	m.directoryInput.SetValue(path)
	m.screen = screenConfig
	m.setConfigFocus(1)
	m.message = "Folder selected. Edit the package name, then continue."
	return m, textinput.Blink
}

func (m *rootShellModel) syncComponentAnswers() error {
	if !slices.Contains(m.answers.components, "box") {
		return errors.New("Box is required for every Box Dispatch solution")
	}
	for i := range m.components {
		m.components[i].selected = slices.Contains(m.answers.components, m.components[i].provider)
	}
	return nil
}

func (m rootShellModel) selectTemplateAndConfigure() (tea.Model, tea.Cmd) {
	for i := range m.templates {
		if m.templates[i].id == m.answers.templateID {
			choice := m.templates[i]
			m.selected = &choice
			m.templateCursor = i
			break
		}
	}
	if m.selected == nil {
		choice := m.templates[0]
		m.selected = &choice
		m.answers.templateID = choice.id
	}
	m.answers.packageName = "box-bedrock-for-" + m.selected.id
	if m.selected.id == "new" {
		m.answers.packageName = "box-bedrock-for-my-solution"
	}
	m.prepareConfigInputs()
	m.prepareBoxComponentSelection()
	m.savePlan()
	m.screen, m.cursor, m.configFocus = screenConfig, 0, 0
	return m, textinput.Blink
}

func (m rootShellModel) startPackage() (tea.Model, tea.Cmd) {
	if m.selected == nil {
		m.message = "Choose a solution template before packaging."
		return m, nil
	}
	if strings.TrimSpace(m.selected.repository) == "" {
		m.message = "This solution has no source repository configured; set a repository for scenario " + m.selected.id + " in the runtime config before packaging."
		return m, nil
	}
	m.answers.directory = strings.TrimSpace(m.directoryInput.Value())
	m.answers.packageName = strings.TrimSpace(m.packageInput.Value())
	info, err := os.Stat(m.answers.directory)
	if err != nil || !info.IsDir() {
		m.message = "Choose an existing parent directory."
		return m, nil
	}
	name := m.answers.packageName
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		m.message = "Enter a valid package directory name without slashes."
		m.setConfigFocus(1)
		return m, nil
	}
	m.packagePath = filepath.Join(m.answers.directory, strings.TrimSpace(m.answers.packageName))
	m.packageStarted = true
	m.packageDone = false
	m.screen = screenPackage
	m.message = "Pulling the selected solution components from GitHub..."
	req := workspace.PackageRequest{
		Repository: m.selected.repository, Destination: m.packagePath,
		TemplateID: m.selected.id, Components: m.selectedProviders(), BoxComponents: m.boxComponentSelection(),
		BoxStrategy: m.answers.deploymentStrategy,
	}
	return m, tea.Batch(m.spinner.Tick, packageCmd(req))
}

func packageCmd(req workspace.PackageRequest) tea.Cmd {
	return func() tea.Msg {
		manifest, err := workspace.Build(req)
		return packageFinishedMsg{manifest: manifest, err: err}
	}
}

func (m rootShellModel) enterValidate() (tea.Model, tea.Cmd) {
	m.screen = screenValidate
	if m.validateStarted {
		return m, nil
	}
	return m.startValidation()
}

func (m rootShellModel) startValidation() (tea.Model, tea.Cmd) {
	m.validateStarted = true
	m.validateRunning = true
	m.validateDone = false
	m.validationItems = nil
	m.validationQueue = append([]string(nil), m.selectedProviders()...)
	m.validationProgress = map[string]float64{}
	for _, provider := range m.validationQueue {
		m.validationProgress[provider] = 0
		item, err := lifecycle.PlanProvider(m.packagePath, provider)
		if err != nil {
			item = lifecycle.Item{
				Provider: provider,
				Name:     providerLabel(provider) + " configuration",
				Status:   lifecycle.StatusFailed,
				Detail:   "Unable to prepare validation checklist: " + err.Error(),
			}
		}
		m.validationItems = append(m.validationItems, item)
	}
	return m.startNextValidation()
}

func (m rootShellModel) startDeploy() (tea.Model, tea.Cmd) {
	m.deployStarted = true
	m.deployDone = false
	m.deploymentBaseline = append([]lifecycle.Item(nil), m.validationItems...)
	m.deploymentStartedAt = time.Now().UTC()
	m.deploymentCompletedAt = time.Time{}
	m.deploymentAuditPath = ""
	m.deploymentQueue = append([]lifecycle.Item(nil), m.validationItems...)
	m.deploymentProgress = map[string]float64{}
	for _, item := range m.deploymentQueue {
		m.deploymentProgress[item.Provider] = 0
	}
	return m.startNextDeployment()
}

func (m rootShellModel) requestDeployConfirmation() (tea.Model, tea.Cmd) {
	deployable := 0
	providers := []string{}
	for _, item := range m.validationItems {
		if item.Status == lifecycle.StatusMissing && item.Deployable {
			deployable++
			providers = append(providers, providerLabel(item.Provider))
		}
	}
	approved := false
	m.deployApproved = &approved
	m.confirmingDeploy = true
	title := fmt.Sprintf("Deploy %d provider configuration set(s)?", deployable)
	description := "Box Dispatch will deploy missing configuration to: " + strings.Join(providers, ", ") + ". Existing components will be skipped."
	affirmative := "Deploy"
	if deployable == 0 {
		title = "Complete with no deployment changes?"
		description = "Validation found no supported missing configuration. No provider commands will run."
		affirmative = "Complete"
	}
	m.deployConfirmForm = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative(affirmative).
				Negative("Cancel").
				Value(m.deployApproved),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithWidth(76)
	m.message = "Review and confirm the deployment plan."
	return m, m.deployConfirmForm.Init()
}

func (m rootShellModel) startNextValidation() (tea.Model, tea.Cmd) {
	if len(m.validationQueue) == 0 {
		m.validateRunning = false
		m.validateDone = true
		m.currentValidation = ""
		m.message = "Validation complete. Review existing and missing configuration before deployment."
		return m, nil
	}
	provider := m.validationQueue[0]
	m.validationQueue = m.validationQueue[1:]
	m.currentValidation = provider
	m.validationProgress[provider] = 0.05
	m.message = "Validating " + providerLabel(provider) + " configuration..."
	root := m.packagePath
	return m, tea.Batch(m.spinner.Tick, validationProgressCmd(provider), func() tea.Msg {
		item, err := lifecycle.ValidateProvider(root, provider)
		return providerValidationFinishedMsg{provider: provider, item: item, err: err}
	})
}

func validationProgressCmd(provider string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return providerValidationProgressMsg{provider: provider}
	})
}

func (m rootShellModel) startNextDeployment() (tea.Model, tea.Cmd) {
	for len(m.deploymentQueue) > 0 {
		item := m.deploymentQueue[0]
		m.deploymentQueue = m.deploymentQueue[1:]
		if item.Status != lifecycle.StatusMissing || !item.Deployable {
			m.deploymentProgress[item.Provider] = 1
			continue
		}
		m.currentDeployment = item.Provider
		m.deploymentProgress[item.Provider] = 0.05
		m.message = "Deploying " + providerLabel(item.Provider) + " configuration..."
		root := m.packagePath
		return m, tea.Batch(m.spinner.Tick, deploymentProgressCmd(item.Provider), func() tea.Msg {
			result, err := lifecycle.DeployProvider(root, item)
			return providerDeployFinishedMsg{provider: item.Provider, item: result, err: err}
		})
	}
	m.deployStarted = false
	m.deployDone = true
	m.deploymentCompletedAt = time.Now().UTC()
	m.currentDeployment = ""
	m.message = "Deployment run complete. Review provider results below."
	// Persist the audit immediately: it is the resource inventory a later reset
	// deletes from, so it must not depend on the operator pressing a key.
	if path, err := deploymentaudit.ExportDeployment(m.packagePath, m.deploymentBaseline, m.validationItems, m.deploymentStartedAt, m.deploymentCompletedAt); err == nil {
		m.deploymentAuditPath = path
	}
	return m, nil
}

func deploymentProgressCmd(provider string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return providerDeployProgressMsg{provider: provider}
	})
}

// openTeardown loads a recorded deployment and shows what a reset would remove.
// Nothing is deleted until the confirmation is typed.
func (m rootShellModel) openTeardown(record deploymentaudit.DeploymentRecord) (tea.Model, tea.Cmd) {
	m.teardownRecord = &record
	m.teardownError = ""
	m.teardownStarted = false
	m.teardownDone = false
	m.teardownResults = nil
	m.teardownProgress = map[string]float64{}
	m.currentTeardown = ""
	m.teardownProviders = nil
	for _, provider := range record.Providers {
		if len(provider.Resources) > 0 {
			m.teardownProviders = append(m.teardownProviders, provider.Provider)
		}
	}
	m.screen, m.cursor = screenTeardown, 0
	if len(m.teardownProviders) == 0 {
		m.teardownError = "This deployment recorded no resources, so there is nothing to remove."
		m.message = "Nothing to reset."
		return m, nil
	}
	m.message = "Review what will be removed, then confirm."
	return m, nil
}

// requestTeardownConfirmation gates the reset behind typing the package name,
// so a destructive run cannot happen on a stray keypress.
func (m rootShellModel) requestTeardownConfirmation() (tea.Model, tea.Cmd) {
	if m.teardownRecord == nil || len(m.teardownProviders) == 0 {
		return m, nil
	}
	expected := teardownConfirmationPhrase(*m.teardownRecord)
	typed := ""
	m.teardownConfirmation = &typed
	m.confirmingTeardown = true
	m.teardownConfirmForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Permanently delete %d recorded resources?", teardownResourceCount(*m.teardownRecord))).
				Description("This cannot be undone. Type " + expected + " to confirm.").
				Value(m.teardownConfirmation).
				Validate(func(value string) error {
					if strings.TrimSpace(value) != expected {
						return fmt.Errorf("type %s to confirm", expected)
					}
					return nil
				}),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithWidth(76)
	m.message = "Type the package name to confirm the reset."
	return m, m.teardownConfirmForm.Init()
}

// teardownConfirmationPhrase is the package directory name, which is specific to
// the environment being reset.
func teardownConfirmationPhrase(record deploymentaudit.DeploymentRecord) string {
	if name := strings.TrimSpace(filepath.Base(record.PackageRoot)); name != "" && name != "." && name != string(filepath.Separator) {
		return name
	}
	return record.DeploymentID
}

func teardownResourceCount(record deploymentaudit.DeploymentRecord) int {
	return len(record.DeployedResources())
}

func (m rootShellModel) startTeardown() (tea.Model, tea.Cmd) {
	m.teardownStarted = true
	m.teardownDone = false
	m.teardownResults = nil
	m.teardownQueue = append([]string(nil), m.teardownProviders...)
	m.teardownProgress = map[string]float64{}
	for _, provider := range m.teardownQueue {
		m.teardownProgress[provider] = 0
	}
	m.message = "Removing deployed resources..."
	return m.startNextTeardown()
}

func (m rootShellModel) startNextTeardown() (tea.Model, tea.Cmd) {
	for len(m.teardownQueue) > 0 {
		provider := m.teardownQueue[0]
		m.teardownQueue = m.teardownQueue[1:]
		m.currentTeardown = provider
		root := m.teardownRecord.PackageRoot
		resources := m.teardownRecord.DeployedResources()
		return m, tea.Batch(m.spinner.Tick, teardownProgressCmd(provider), func() tea.Msg {
			result, err := lifecycle.DestroyProvider(root, provider, resources)
			return providerTeardownFinishedMsg{provider: provider, result: result, err: err}
		})
	}
	m.teardownStarted = false
	m.teardownDone = true
	m.currentTeardown = ""
	m.message = "Reset complete. Review the results below."
	return m, nil
}

func teardownProgressCmd(provider string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return providerTeardownProgressMsg{provider: provider}
	})
}

func (m rootShellModel) Init() tea.Cmd {
	if m.componentForm == nil {
		return nil
	}
	return m.componentForm.Init()
}

func (m rootShellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = max(msg.Width-8, 20)
		m.progress.Width = min(max(msg.Width-34, 20), 52)
		if m.componentForm != nil {
			m.componentForm.WithWidth(min(max(msg.Width-16, 44), 90))
		}
		if m.templateForm != nil {
			m.templateForm.WithWidth(min(max(msg.Width-16, 44), 90))
		}
		if m.teardownConfirmForm != nil {
			m.teardownConfirmForm.WithWidth(min(max(msg.Width-16, 44), 90))
		}
		if m.boxCCGForm != nil {
			m.boxCCGForm.WithWidth(min(max(msg.Width-16, 44), 90))
		}
		if m.deployConfirmForm != nil {
			m.deployConfirmForm.WithWidth(min(max(msg.Width-16, 44), 90))
			if m.screen == screenDeploy && m.confirmingDeploy {
				form, cmd := m.deployConfirmForm.Update(msg)
				m.deployConfirmForm = form.(*huh.Form)
				return m, cmd
			}
		}
		m.directoryInput.Width = min(max(msg.Width-34, 30), 72)
		m.packageInput.Width = min(max(msg.Width-34, 30), 56)
		m.directoryPicker.Height = min(max(msg.Height-18, 5), 16)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case checkFinishedMsg:
		if msg.err != nil {
			m.statuses[msg.provider] = connectionFailed
			m.message = msg.err.Error()
		} else {
			m.results[msg.provider] = msg.result
			if msg.result.ConnectivityOK {
				m.statuses[msg.provider] = connectionConnected
				m.message = providerLabel(msg.provider) + " connected"
			} else {
				m.statuses[msg.provider] = connectionFailed
				m.message = providerLabel(msg.provider) + " needs attention"
			}
		}
		return m.startNextCheck()
	case externalFinishedMsg:
		if msg.err != nil {
			m.message = "Connection command failed: " + msg.err.Error()
		} else {
			m.message = "Authentication finished. Choose Check connection when ready."
		}
		return m, nil
	case packageFinishedMsg:
		m.packageStarted = false
		if msg.err != nil {
			m.message = "Package failed: " + msg.err.Error()
			return m, nil
		}
		m.packageDone = true
		m.packagePath = msg.manifest.Destination
		m.message = "Package created. Press → to validate provider configuration."
		m.savePlan()
		return m, nil
	case providerValidationFinishedMsg:
		m.validationProgress[msg.provider] = 1
		if msg.err != nil {
			msg.item = lifecycle.Item{Provider: msg.provider, Name: providerLabel(msg.provider) + " configuration", Status: lifecycle.StatusFailed, Detail: msg.err.Error()}
		}
		updated := false
		for i := range m.validationItems {
			if m.validationItems[i].Provider == msg.provider {
				m.validationItems[i] = msg.item
				updated = true
				break
			}
		}
		if !updated {
			m.validationItems = append(m.validationItems, msg.item)
		}
		return m.startNextValidation()
	case providerValidationProgressMsg:
		if m.validateRunning && m.currentValidation == msg.provider && m.validationProgress[msg.provider] < 0.92 {
			m.validationProgress[msg.provider] = math.Min(m.validationProgress[msg.provider]+0.035, 0.92)
			return m, validationProgressCmd(msg.provider)
		}
		return m, nil
	case providerDeployFinishedMsg:
		m.deploymentProgress[msg.provider] = 1
		if msg.err != nil {
			msg.item.Status = lifecycle.StatusFailed
			msg.item.Detail = msg.err.Error()
		}
		for i := range m.validationItems {
			if m.validationItems[i].Provider == msg.provider {
				m.validationItems[i] = msg.item
				break
			}
		}
		return m.startNextDeployment()
	case providerDeployProgressMsg:
		if m.deployStarted && m.currentDeployment == msg.provider && m.deploymentProgress[msg.provider] < 0.92 {
			m.deploymentProgress[msg.provider] = math.Min(m.deploymentProgress[msg.provider]+0.035, 0.92)
			return m, deploymentProgressCmd(msg.provider)
		}
		return m, nil

	case providerTeardownFinishedMsg:
		m.teardownProgress[msg.provider] = 1
		result := msg.result
		if msg.err != nil {
			result.Provider = msg.provider
			result.Detail = msg.err.Error()
		}
		m.teardownResults = append(m.teardownResults, result)
		return m.startNextTeardown()

	case providerTeardownProgressMsg:
		if m.teardownStarted && m.currentTeardown == msg.provider && m.teardownProgress[msg.provider] < 0.92 {
			m.teardownProgress[msg.provider] = math.Min(m.teardownProgress[msg.provider]+0.035, 0.92)
			return m, teardownProgressCmd(msg.provider)
		}
		return m, nil
	}

	if m.screen == screenDatabricksHost {
		return m.updateDatabricksHost(msg)
	}
	if m.screen == screenDirectoryPicker {
		return m.updateDirectoryPicker(msg)
	}
	if m.screen == screenBoxComponents {
		return m.updateBoxComponents(msg)
	}
	if m.screen == screenBoxCCG && m.boxCCGForm != nil {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
			m.screen, m.cursor = screenProvider, 0
			m.message = "Box CCG setup cancelled."
			return m, nil
		}
		form, cmd := m.boxCCGForm.Update(msg)
		m.boxCCGForm = form.(*huh.Form)
		if m.boxCCGForm.State == huh.StateCompleted {
			return m.saveBoxCCG()
		}
		if m.boxCCGForm.State == huh.StateAborted {
			m.screen, m.cursor = screenProvider, 0
			m.message = "Box CCG setup cancelled."
			return m, nil
		}
		return m, cmd
	}
	if m.screen == screenTeardown && m.confirmingTeardown && m.teardownConfirmForm != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "q":
				m.confirmingTeardown = false
				m.message = "Reset cancelled. Nothing was removed."
				return m, nil
			}
		}
		form, cmd := m.teardownConfirmForm.Update(msg)
		m.teardownConfirmForm = form.(*huh.Form)
		if m.teardownConfirmForm.State == huh.StateCompleted {
			m.confirmingTeardown = false
			// The form only completes when the typed phrase validated.
			return m.startTeardown()
		}
		return m, cmd
	}
	if m.screen == screenDeploy && m.confirmingDeploy && m.deployConfirmForm != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "q":
				m.confirmingDeploy = false
				m.screen, m.cursor, m.message = screenWelcome, 0, ""
				return m, nil
			case "esc":
				m.confirmingDeploy = false
				m.message = "Deployment cancelled. No provider configuration was changed."
				return m, nil
			}
		}
		form, cmd := m.deployConfirmForm.Update(msg)
		m.deployConfirmForm = form.(*huh.Form)
		if m.deployConfirmForm.State == huh.StateCompleted {
			m.confirmingDeploy = false
			if m.deployApproved != nil && *m.deployApproved {
				return m.startDeploy()
			}
			m.message = "Deployment cancelled. No provider configuration was changed."
		}
		return m, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if key.String() == "q" && !(m.screen == screenConfig && m.configFocus < 2) {
		if m.screen == screenWelcome {
			return m, tea.Quit
		}
		m.screen, m.cursor, m.message = screenWelcome, 0, ""
		return m, nil
	}
	switch m.screen {
	case screenWelcome:
		m.moveCursor(key, len(welcomeOptions))
		if key.String() == "enter" || key.String() == "right" {
			if m.cursor == 0 {
				m.screen = screenComponents
				return m, m.componentForm.Init()
			}
			resetting := m.cursor == 2
			history, err := deploymentaudit.ListDeployments()
			m.deploymentHistory = history
			m.historyError = ""
			if err != nil {
				m.historyError = err.Error()
			} else if len(history) == 0 {
				m.historyError = "No deployment has been recorded yet."
			}
			// "Reset demo environment" opens the same history list; the reset
			// acts on whichever deployment is selected there.
			m.screen, m.cursor = screenHistory, 0
			if resetting {
				m.message = "Choose the deployment to reset, then press enter."
			} else {
				m.message = ""
			}
			return m, nil
		}
	case screenHistory:
		m.moveCursor(key, len(m.deploymentHistory))
		if key.String() == "left" || key.String() == "esc" {
			m.screen, m.cursor = screenWelcome, 1
			return m, nil
		}
		// Enter on a recorded deployment opens the reset preview for it.
		if (key.String() == "enter" || key.String() == "right") && m.cursor < len(m.deploymentHistory) {
			return m.openTeardown(m.deploymentHistory[m.cursor])
		}
	case screenTeardown:
		if key.String() == "left" || key.String() == "esc" || key.String() == "q" {
			if m.teardownStarted {
				return m, nil
			}
			m.screen, m.cursor = screenHistory, 0
			m.message = ""
			return m, nil
		}
		if (key.String() == "enter" || key.String() == "right") && !m.teardownStarted && !m.teardownDone && m.teardownError == "" {
			return m.requestTeardownConfirmation()
		}
		if (key.String() == "enter" || key.String() == "right") && m.teardownDone {
			m.screen, m.cursor = screenWelcome, 0
			m.message = ""
			return m, nil
		}
	case screenComponents:
		if key.String() == "left" {
			m.screen, m.cursor = screenWelcome, 0
			return m, nil
		}
		if key.String() == "right" {
			if err := m.syncComponentAnswers(); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.screen, m.cursor = screenDashboard, 0
			return m.beginChecks(m.selectedProviders())
		}
		form, cmd := m.componentForm.Update(msg)
		m.componentForm = form.(*huh.Form)
		if m.componentForm.State == huh.StateCompleted {
			if err := m.syncComponentAnswers(); err != nil {
				m.message = err.Error()
				m.rebuildComponentForm()
				return m, m.componentForm.Init()
			}
			m.screen, m.cursor = screenDashboard, 0
			model, checkCmd := m.beginChecks(m.selectedProviders())
			return model, tea.Batch(cmd, checkCmd)
		}
		return m, cmd
	case screenTemplates:
		if key.String() == "left" {
			m.screen, m.cursor = screenDashboard, 0
			return m, nil
		}
		if key.String() == "right" {
			return m.selectTemplateAndConfigure()
		}
		form, cmd := m.templateForm.Update(msg)
		m.templateForm = form.(*huh.Form)
		if m.templateForm.State == huh.StateCompleted {
			model, nextCmd := m.selectTemplateAndConfigure()
			return model, tea.Batch(cmd, nextCmd)
		}
		return m, cmd
	case screenDashboard:
		items := m.dashboardItems()
		m.moveCursor(key, len(items))
		if key.String() == "left" {
			m.rebuildComponentForm()
			m.screen, m.cursor = screenComponents, 0
			return m, m.componentForm.Init()
		}
		if key.String() == "right" {
			if m.allSelectedConnected() {
				m.rebuildTemplateForm()
				m.screen, m.cursor = screenTemplates, m.templateCursor
				return m, m.templateForm.Init()
			} else {
				m.message = "Connect every selected service before continuing."
			}
			return m, nil
		}
		if key.String() == "enter" {
			if m.cursor < len(m.selectedProviders()) {
				m.provider = m.selectedProviders()[m.cursor]
				m.screen, m.cursor = screenProvider, 0
			} else if m.cursor == len(m.selectedProviders()) {
				return m.beginChecks(m.selectedProviders())
			} else if m.cursor == len(m.selectedProviders())+1 {
				m.screen, m.cursor = screenComponents, 0
			} else if m.allSelectedConnected() {
				m.rebuildTemplateForm()
				m.screen, m.cursor = screenTemplates, m.templateCursor
				return m, m.templateForm.Init()
			} else {
				m.message = "Connect every selected service before continuing."
			}
		}
	case screenProvider:
		m.moveCursor(key, len(m.providerActions()))
		if key.String() == "left" {
			m.screen, m.cursor = screenDashboard, 0
			return m, nil
		}
		if key.String() == "enter" || key.String() == "right" {
			return m.runProviderAction(m.cursor)
		}
	case screenBoxSwitch:
		m.moveCursor(key, len(m.boxConnections))
		if key.String() == "left" || key.String() == "esc" {
			m.screen, m.cursor = screenProvider, 0
			m.message = ""
			return m, nil
		}
		if (key.String() == "enter" || key.String() == "right") && m.cursor < len(m.boxConnections) {
			return m.setBoxDefault(m.boxConnections[m.cursor])
		}
	case screenOptions:
		options := m.results[m.provider].Discovery.Options
		m.moveCursor(key, len(options))
		m.optionCursor = m.cursor
		if (key.String() == "enter" || key.String() == "right") && len(options) > 0 {
			m.saveProviderOption(options[m.cursor])
			m.screen, m.cursor = screenProvider, 0
			m.message = "Selection saved. Choose Check connection when ready."
		}
		if key.String() == "left" {
			m.screen, m.cursor = screenProvider, 0
		}
	case screenConfig:
		if key.String() == "esc" || (key.String() == "left" && m.configFocus == 4) {
			m.rebuildTemplateForm()
			m.screen, m.cursor = screenTemplates, m.templateCursor
			return m, m.templateForm.Init()
		}
		switch key.String() {
		case "up", "shift+tab":
			m.setConfigFocus((m.configFocus + 4) % 5)
			return m, nil
		case "down", "tab":
			m.setConfigFocus((m.configFocus + 1) % 5)
			return m, nil
		case "b":
			if m.configFocus != 0 {
				break
			}
			return m.openDirectoryPicker()
		case "enter":
			if m.configFocus == 0 {
				return m.openDirectoryPicker()
			}
			if m.configFocus == 1 {
				m.setConfigFocus(2)
				return m, nil
			}
			if m.configFocus == 2 {
				m.cycleDeploymentStrategy(1)
				return m, nil
			}
			if m.configFocus == 3 {
				m.screen, m.cursor = screenBoxComponents, 0
				return m, nil
			}
			if m.configFocus == 4 {
				return m.startPackage()
			}
		case "right":
			if m.configFocus == 2 {
				m.cycleDeploymentStrategy(1)
				return m, nil
			}
			if m.configFocus == 4 {
				return m.startPackage()
			}
		case "left":
			if m.configFocus == 2 {
				m.cycleDeploymentStrategy(-1)
				return m, nil
			}
		case "space":
			if m.configFocus == 2 {
				m.cycleDeploymentStrategy(1)
				return m, nil
			}
		}
		var cmd tea.Cmd
		if m.configFocus == 0 {
			m.directoryInput, cmd = m.directoryInput.Update(msg)
			m.answers.directory = strings.TrimSpace(m.directoryInput.Value())
		} else if m.configFocus == 1 {
			m.packageInput, cmd = m.packageInput.Update(msg)
			m.answers.packageName = strings.TrimSpace(m.packageInput.Value())
		}
		return m, cmd
	case screenPackage:
		if key.String() == "left" {
			m.prepareConfigInputs()
			m.screen = screenConfig
			m.message = "Choose a destination or edit the package name."
			return m, textinput.Blink
		}
		if key.String() == "right" || (key.String() == "enter" && m.packageDone) {
			if !m.packageDone {
				m.message = "Create the package before validation."
				return m, nil
			}
			return m.enterValidate()
		}
		if key.String() == "enter" && !m.packageStarted {
			return m.startPackage()
		}
	case screenValidate:
		if key.String() == "left" {
			m.screen = screenPackage
			return m, nil
		}
		if key.String() == "right" || (key.String() == "enter" && m.validateDone) {
			if !m.validateDone {
				m.message = "Complete validation before deployment."
				return m, nil
			}
			m.screen = screenDeploy
			return m.requestDeployConfirmation()
		}
		if key.String() == "r" && !m.validateRunning {
			return m.startValidation()
		}
		if key.String() == "enter" && !m.validateRunning && !m.validateDone {
			return m.startValidation()
		}
	case screenDeploy:
		if key.String() == "left" {
			m.screen = screenValidate
			return m, nil
		}
		if (key.String() == "enter" || key.String() == "right") && m.deployDone {
			if m.deploymentAuditPath != "" {
				m.screen, m.cursor = screenWelcome, 0
				m.message = ""
				return m, nil
			}
			path, err := deploymentaudit.ExportDeployment(m.packagePath, m.deploymentBaseline, m.validationItems, m.deploymentStartedAt, m.deploymentCompletedAt)
			if err != nil {
				m.message = "Audit export failed: " + err.Error()
				return m, nil
			}
			m.deploymentAuditPath = path
			m.message = "Deployment audit exported: " + path
			return m, nil
		}
		if (key.String() == "enter" || key.String() == "right") && !m.deployStarted {
			return m.requestDeployConfirmation()
		}
	}
	return m, nil
}

func (m *rootShellModel) moveCursor(key tea.KeyMsg, count int) {
	if count == 0 {
		return
	}
	switch key.String() {
	case "up", "k":
		m.cursor = (m.cursor - 1 + count) % count
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % count
	}
}

func (m rootShellModel) beginChecks(providers []string) (tea.Model, tea.Cmd) {
	if len(providers) == 0 {
		return m, nil
	}
	m.queue = append([]string(nil), providers...)
	return m.startNextCheck()
}

func (m rootShellModel) startNextCheck() (tea.Model, tea.Cmd) {
	if len(m.queue) == 0 {
		if m.allSelectedConnected() {
			m.message = "All selected services connected. Press → to continue."
		}
		return m, nil
	}
	provider := m.queue[0]
	m.queue = m.queue[1:]
	m.statuses[provider] = connectionChecking
	m.message = "Checking " + providerLabel(provider) + "..."
	return m, tea.Batch(m.spinner.Tick, checkProviderCmd(provider))
}

func checkProviderCmd(provider string) tea.Cmd {
	return func() tea.Msg {
		// The checker reads provider credentials from the environment, so apply
		// the profile the user chose in the shell before running it. Without
		// this, choosing a Salesforce alias (or AWS profile) never reaches the
		// connectivity check and the provider never shows as connected.
		applyConnectionEnv()
		report, err := checker.Check(checker.CheckConfig{Providers: []string{provider}})
		if err != nil {
			return checkFinishedMsg{provider: provider, err: err}
		}
		if len(report.Providers) == 0 {
			return checkFinishedMsg{provider: provider, err: fmt.Errorf("no result returned")}
		}
		return checkFinishedMsg{provider: provider, result: report.Providers[0]}
	}
}

// applyConnectionEnv exports the connection choices saved by the shell so the
// checker (which is environment-driven) targets them. Only non-empty values are
// set, so a real environment variable still wins when nothing was chosen.
func applyConnectionEnv() {
	settings, _ := shellstate.LoadConnectionSettings()
	for key, value := range map[string]string{
		"SF_ALIAS":           settings.SalesforceAlias,
		"AWS_PROFILE":        settings.AWSProfile,
		"AWS_REGION":         settings.AWSRegion,
		"DATABRICKS_HOST":    settings.DatabricksHost,
		"DATABRICKS_PROFILE": settings.DatabricksProfile,
	} {
		if strings.TrimSpace(value) != "" {
			_ = os.Setenv(key, value)
		}
	}
}

func (m rootShellModel) runProviderAction(index int) (tea.Model, tea.Cmd) {
	actions := m.providerActions()
	if index >= len(actions) {
		return m, nil
	}
	switch actions[index] {
	case "check":
		return m.beginChecks([]string{m.provider})
	case "choose":
		m.screen, m.cursor = screenOptions, 0
		return m, nil
	case "ccg":
		return m.openBoxCCGForm()
	case "switch":
		return m.openBoxSwitch()
	case "connect":
		if m.provider == "databricks" {
			m.screen = screenDatabricksHost
			m.hostInput.Focus()
			return m, textinput.Blink
		}
		commands := map[string][]string{
			"box":        {"box", "login"},
			"salesforce": {"sf", "org", "login", "web"},
			"aws":        {"aws", "configure", "sso"},
		}
		parts := commands[m.provider]
		cmd := exec.Command(parts[0], parts[1:]...)
		provider := m.provider
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return externalFinishedMsg{provider: provider, err: err} })
	case "back":
		m.screen, m.cursor = screenDashboard, 0
	}
	return m, nil
}

func (m rootShellModel) updateDatabricksHost(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			m.screen, m.cursor = screenProvider, 0
			m.hostInput.Blur()
			return m, nil
		case "enter":
			host := strings.TrimRight(strings.TrimSpace(m.hostInput.Value()), "/")
			if host == "" {
				m.message = "Enter the full Databricks workspace URL."
				return m, nil
			}
			settings, _ := shellstate.LoadConnectionSettings()
			settings.DatabricksHost = host
			if settings.DatabricksProfile == "" {
				settings.DatabricksProfile = "box-dispatch"
			}
			_ = shellstate.SaveConnectionSettings(settings)
			m.screen = screenProvider
			m.hostInput.Blur()
			profile := settings.DatabricksProfile
			cmd := exec.Command("databricks", "auth", "login", "--host", host, "--profile", profile)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return externalFinishedMsg{provider: "databricks", err: err} })
		}
	}
	var cmd tea.Cmd
	m.hostInput, cmd = m.hostInput.Update(msg)
	return m, cmd
}

func (m rootShellModel) providerActions() []string {
	actions := []string{"check"}
	if len(m.results[m.provider].Discovery.Options) > 1 {
		actions = append(actions, "choose")
	}
	if m.provider == "box" {
		actions = append(actions, "ccg", "switch")
	}
	actions = append(actions, "connect", "back")
	return actions
}

// openBoxCCGForm captures a Box Client Credentials Grant app and its subject, so
// box-dispatch can mint a token that carries the enterprise app's scopes (the
// CLI's OAuth token lacks some, e.g. Doc Gen).
func (m rootShellModel) openBoxCCGForm() (tea.Model, tea.Cmd) {
	settings, _ := shellstate.LoadConnectionSettings()
	m.ccgClientID = settings.BoxCCGClientID
	m.ccgClientSecret = settings.BoxCCGClientSecret
	m.ccgSubjectType = settings.BoxCCGSubjectType
	if m.ccgSubjectType == "" {
		m.ccgSubjectType = "user"
	}
	m.ccgSubjectID = settings.BoxCCGSubjectID
	m.boxCCGForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Client ID").Value(&m.ccgClientID).Validate(requiredField("Client ID")),
			huh.NewInput().Title("Client Secret").Password(true).Value(&m.ccgClientSecret).Validate(requiredField("Client Secret")),
			huh.NewSelect[string]().Title("Subject type").
				Options(
					huh.NewOption("user — created resources owned by the user", "user"),
					huh.NewOption("enterprise — acts as the service account", "enterprise"),
				).Value(&m.ccgSubjectType),
			huh.NewInput().Title("Subject ID").
				Description("Box user ID when subject is user, enterprise ID when enterprise").
				Value(&m.ccgSubjectID).Validate(requiredField("Subject ID")),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(true).WithWidth(76)
	m.screen = screenBoxCCG
	m.message = "Enter the CCG app credentials. Esc cancels."
	return m, m.boxCCGForm.Init()
}

func requiredField(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

// saveBoxCCG persists the captured credentials and returns to the Box provider
// screen. The record is written 0600, but the secret is on-disk plaintext.
func (m rootShellModel) saveBoxCCG() (tea.Model, tea.Cmd) {
	settings, _ := shellstate.LoadConnectionSettings()
	settings.BoxCCGClientID = strings.TrimSpace(m.ccgClientID)
	settings.BoxCCGClientSecret = strings.TrimSpace(m.ccgClientSecret)
	settings.BoxCCGSubjectType = strings.TrimSpace(m.ccgSubjectType)
	settings.BoxCCGSubjectID = strings.TrimSpace(m.ccgSubjectID)
	if err := shellstate.SaveConnectionSettings(settings); err != nil {
		m.message = "Could not save Box CCG credentials: " + err.Error()
	} else {
		m.message = "Box CCG credentials saved. Choose Check connection to verify."
	}
	m.screen, m.cursor = screenProvider, 0
	m.boxCCGForm = nil
	return m, nil
}

// openBoxSwitch lists every Box connection box-dispatch can use — the box CLI
// environments (each OAuth2, CCG or JWT) plus the box-dispatch CCG app — so the
// user can pin which one deploys run against.
func (m rootShellModel) openBoxSwitch() (tea.Model, tea.Cmd) {
	m.boxConnections = boxconn.List()
	m.screen, m.cursor = screenBoxSwitch, 0
	if len(m.boxConnections) == 0 {
		m.message = "No Box connection found. Run box login or add a CCG app first."
	} else {
		m.message = "Enter pins the highlighted connection as the default. Esc cancels."
	}
	return m, nil
}

// setBoxDefault pins the chosen connection as box-dispatch's default. A CLI
// connection also becomes the CLI's current environment (the CLI has no
// per-command selection), so the two never disagree.
func (m rootShellModel) setBoxDefault(conn boxconn.Connection) (tea.Model, tea.Cmd) {
	if conn.Source == boxconn.SourceCLI {
		if err := boxconn.SetCLICurrent(conn.Name); err != nil {
			m.message = "Could not switch the Box CLI to " + conn.Name + ": " + err.Error()
			return m, nil
		}
	}
	settings, _ := shellstate.LoadConnectionSettings()
	settings.BoxDefaultConnection = conn.Name
	if err := shellstate.SaveConnectionSettings(settings); err != nil {
		m.message = "Could not save the default Box connection: " + err.Error()
		return m, nil
	}
	m.boxConnections = boxconn.List()
	m.message = conn.Name + " (" + conn.AuthType + ") is now the default Box connection."
	return m, nil
}

func (m rootShellModel) viewBoxSwitch(width int) string {
	rows := []string{}
	if len(m.boxConnections) == 0 {
		rows = append(rows, dimStyle.Render("No Box connection is available."))
	}
	for i, conn := range m.boxConnections {
		tags := []string{}
		if conn.Default {
			tags = append(tags, lipgloss.NewStyle().Bold(true).Foreground(green).Render("[default]"))
		}
		if conn.Current {
			tags = append(tags, dimStyle.Render("[CLI current]"))
		}
		if conn.Source == boxconn.SourceDispatch {
			tags = append(tags, dimStyle.Render("[box-dispatch]"))
		}
		line := fmt.Sprintf("%-28s %-8s", conn.Name, conn.AuthType)
		if len(tags) > 0 {
			line += "  " + strings.Join(tags, " ")
		}
		if conn.Detail != "" {
			line += "\n" + dimStyle.Render("   "+conn.Detail)
		}
		style := panel.Copy().Width(width - 4)
		if i == m.cursor {
			style = activePane.Copy().Width(width - 4)
		}
		rows = append(rows, style.Render(line))
	}
	return m.stageHeader() + "\n\n" +
		titleStyle.Render("Switch Box connection") + "\n" +
		dimStyle.Render("The default is the connection deploys authenticate with. Selecting a CLI environment also makes it the CLI's current environment.") + "\n\n" +
		strings.Join(rows, "\n")
}

func (m rootShellModel) viewBoxCCG(width int) string {
	form := ""
	if m.boxCCGForm != nil {
		form = m.boxCCGForm.View()
	}
	return m.stageHeader() + "\n\n" +
		titleStyle.Render("Connect Box with a CCG app") + "\n" +
		dimStyle.Render("Client Credentials Grant, used as the chosen subject. Stored in .dispatch/connection-settings.bcl (0600); the secret is kept as on-disk plaintext.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(form)
}

func (m rootShellModel) saveProviderOption(value string) {
	settings, _ := shellstate.LoadConnectionSettings()
	switch m.provider {
	case "salesforce":
		settings.SalesforceAlias = value
	case "databricks":
		settings.DatabricksProfile = value
	case "aws":
		settings.AWSProfile = value
	}
	_ = shellstate.SaveConnectionSettings(settings)
}

func (m rootShellModel) savePlan() {
	if m.selected == nil {
		return
	}
	_ = shellstate.SaveSolutionPlan(config.SolutionPlan{
		Components: m.selectedProviders(), TemplateID: m.selected.id,
		Template: m.selected.name, Repository: m.selected.repository, PackagePath: m.packagePath,
	})
}

func (m rootShellModel) selectedProviders() []string {
	providers := make([]string, 0, len(m.components))
	for _, item := range m.components {
		if item.selected {
			providers = append(providers, item.provider)
		}
	}
	return providers
}

func (m rootShellModel) dashboardItems() []string {
	items := append([]string(nil), m.selectedProviders()...)
	return append(items, "check-all", "revise", "continue")
}

func (m rootShellModel) allSelectedConnected() bool {
	providers := m.selectedProviders()
	if len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		if m.statuses[provider] != connectionConnected {
			return false
		}
	}
	return true
}

func (m rootShellModel) View() string {
	if m.width == 0 {
		return ""
	}
	contentWidth := min(max(m.width-8, 64), 112)
	body := ""
	switch m.screen {
	case screenWelcome:
		body = m.viewWelcome(contentWidth)
	case screenHistory:
		body = m.viewDeploymentHistory(contentWidth)
	case screenComponents:
		body = m.viewComponents(contentWidth)
	case screenTemplates:
		body = m.viewTemplates(contentWidth)
	case screenDashboard:
		body = m.viewDashboard(contentWidth)
	case screenProvider:
		body = m.viewProvider(contentWidth)
	case screenOptions:
		body = m.viewOptions(contentWidth)
	case screenDatabricksHost:
		body = m.viewDatabricksHost(contentWidth)
	case screenConfig:
		body = m.viewConfig(contentWidth)
	case screenBoxComponents:
		body = m.viewBoxComponents(contentWidth)
	case screenDirectoryPicker:
		body = m.viewDirectoryPicker(contentWidth)
	case screenPackage:
		body = m.viewPackage(contentWidth)
	case screenValidate:
		body = m.viewValidate(contentWidth)
	case screenDeploy:
		body = m.viewDeploy(contentWidth)
	case screenTeardown:
		body = m.viewTeardown(contentWidth)
	case screenBoxCCG:
		body = m.viewBoxCCG(contentWidth)
	case screenBoxSwitch:
		body = m.viewBoxSwitch(contentWidth)
	}
	return lipgloss.NewStyle().Margin(1, 3).Width(contentWidth).Render(m.header(contentWidth) + "\n\n" + body + "\n\n" + m.footer())
}

func (m rootShellModel) header(width int) string {
	mark := lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("B/")
	product := lipgloss.NewStyle().Bold(true).Foreground(white).Render("  DISPATCH")
	tag := dimStyle.Render("  SOLUTION ASSEMBLY")
	domain := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("UNOFFICIALBOX.DEV")
	used := lipgloss.Width(mark + product + tag + domain)
	gap := max(width-used, 2)
	return mark + product + tag + strings.Repeat(" ", gap) + domain
}

func (m rootShellModel) stepper() string {
	current := 0
	switch m.screen {
	case screenComponents:
		current = 0
	case screenDashboard, screenProvider, screenOptions, screenDatabricksHost:
		current = 1
	case screenTemplates:
		current = 2
	case screenConfig, screenDirectoryPicker:
		current = 3
	case screenPackage:
		current = 4
	case screenValidate:
		current = 5
	case screenDeploy:
		current = 6
	default:
		current = 1
	}
	labels := []string{"BUILD", "CONNECT", "TEMPLATE", "CONFIG", "PACKAGE", "VALIDATE", "DEPLOY"}
	if m.width < 100 {
		labels = []string{"BUILD", "LINK", "TPL", "CONFIG", "PACK", "CHECK", "SHIP"}
	}
	parts := make([]string, len(labels))
	for i, label := range labels {
		color := muted
		icon := "○"
		if i < current {
			color, icon = green, "●"
		} else if i == current {
			color, icon = coral, "◆"
		}
		parts[i] = lipgloss.NewStyle().Bold(i == current).Foreground(color).Render(icon + " " + label)
	}
	return strings.Join(parts, dimStyle.Render(" ━ "))
}

func (m rootShellModel) viewWelcome(width int) string {
	eyebrow := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("Community-built. Open source. Punk Rock. 🤘")
	headline := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(white).Render("BOX "),
		lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("DISPATCH"),
		lipgloss.NewStyle().Foreground(coral).Render("  🚀"),
	)
	description := dimStyle.Copy().Width(58).Render("Open tools for the builders extending Box, from composable building blocks to industry solution accelerators.")
	tags := lipgloss.NewStyle().Bold(true).Foreground(ice).Render("BOX  /  AGENTFORCE  /  DATABRICKS  /  AWS BEDROCK AGENTCORE")
	copy := lipgloss.JoinVertical(lipgloss.Left, eyebrow, "", headline, "", description, "", tags)
	options := welcomeOptions
	optionRows := make([]string, len(options))
	for index, option := range options {
		style := lipgloss.NewStyle().Width(42).Padding(0, 2).Foreground(ice)
		marker := "  "
		if index == m.cursor {
			style = style.Bold(true).Foreground(navy).Background(coral)
			marker = "→ "
		}
		optionRows[index] = style.Render(marker + option)
	}
	heroContent := lipgloss.JoinVertical(lipgloss.Left, copy, "", strings.Join(optionRows, "\n"))
	hero := panel.Copy().BorderForeground(white).BorderLeftForeground(coral).Width(width-4).Padding(2, 3).Render(heroContent)

	routeLabel := dimStyle.Render("YOUR ROUTE")
	route := strings.Join([]string{
		accent.Render("01  SELECT STACK"),
		dimStyle.Render("━━"),
		accent.Render("02  CONNECT"),
		dimStyle.Render("━━"),
		accent.Render("03  PICK QUICKSTART"),
		dimStyle.Render("━━"),
		lipgloss.NewStyle().Bold(true).Foreground(green).Render("04  SHIP"),
	}, " ")
	return hero + "\n\n" + routeLabel + "\n" + route
}

func (m rootShellModel) viewDeploymentHistory(width int) string {
	if m.historyError != "" {
		return titleStyle.Render("Deployment history") + "\n\n" + lipgloss.NewStyle().Foreground(coral).Render(m.historyError)
	}
	if len(m.deploymentHistory) == 0 {
		return titleStyle.Render("Deployment history") + "\n" + dimStyle.Render("No deployment audit records have been exported yet.")
	}
	rows := make([]string, 0, len(m.deploymentHistory))
	for index, record := range m.deploymentHistory {
		deployed, failed := 0, 0
		for _, provider := range record.Providers {
			deployed += len(provider.Deployed)
			if provider.StatusAfter == lifecycle.StatusFailed {
				failed++
			}
		}
		outcome := lipgloss.NewStyle().Foreground(green).Render("COMPLETE")
		if failed > 0 {
			outcome = lipgloss.NewStyle().Foreground(coral).Render(fmt.Sprintf("%d FAILED", failed))
		}
		line := fmt.Sprintf("%-20s  %-12s  %3d deployed  %s", record.CompletedAt.Local().Format("2006-01-02 15:04"), record.TemplateID, deployed, outcome)
		style := lipgloss.NewStyle().Width(width-10).Padding(0, 1)
		if index == m.cursor {
			style = style.Bold(true).Foreground(white).Background(lipgloss.Color("#16355F"))
		}
		rows = append(rows, style.Render(line))
	}
	selected := m.deploymentHistory[m.cursor]
	detail := strings.Join([]string{
		titleStyle.Render(selected.DeploymentID),
		dimStyle.Render("Package") + "  " + selected.PackageRoot,
		dimStyle.Render("Strategy") + "  " + selected.Strategy,
		dimStyle.Render("Duration") + "  " + selected.Duration,
		dimStyle.Render("Audit") + "  " + selected.SourcePath,
	}, "\n")
	return titleStyle.Render("Deployment history") + "\n" + dimStyle.Render("Credential-free audit records exported by Box Dispatch, newest first.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n")) + "\n\n" +
		panel.Copy().BorderForeground(cyan).Width(width-4).Padding(1, 2).Render(detail)
}

func (m rootShellModel) viewComponents(width int) string {
	form := ""
	if m.componentForm != nil {
		form = m.componentForm.View()
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Build your solution stack") + "\n" + dimStyle.Render("A validated Huh multi-select keeps the platform plan consistent.") + "\n\n" + activePane.Copy().Width(width-4).Padding(1, 2).Render(form)
}

func (m rootShellModel) viewTemplates(width int) string {
	form := ""
	if m.templateForm != nil {
		form = m.templateForm.View()
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose an industry quickstart") + "\n" + dimStyle.Render("Huh provides keyboard selection, validation, and accessible form behavior.") + "\n\n" + activePane.Copy().Width(width-4).Padding(1, 2).Render(form)
}

func (m rootShellModel) viewDashboard(width int) string {
	providers := m.selectedProviders()
	connected := 0
	rows := make([]string, 0, len(providers))
	for i, provider := range providers {
		status := m.statuses[provider]
		if status == connectionConnected {
			connected++
		}
		line := fmt.Sprintf("  %-31s ", providerLabel(provider)) + m.statusLabel(provider)
		if details := providerConnectionDetails(m.results[provider]); details != "" {
			line += "\n      " + dimStyle.Render(details)
		}
		style := lipgloss.NewStyle().Width(width-10).Padding(0, 1)
		if i == m.cursor {
			style = style.Copy().Bold(true).Foreground(white).Background(lipgloss.Color("#16355F"))
		}
		rows = append(rows, style.Render(line))
	}
	progress := float64(1) / 7
	if len(providers) > 0 {
		progress += (float64(connected) / float64(len(providers))) / 7
	}
	checkIndex := len(providers)
	reviseIndex := checkIndex + 1
	continueIndex := reviseIndex + 1
	checkStyle, reviseStyle := panel.Copy().Padding(0, 1).Width((width-7)/2).Height(3), panel.Copy().Padding(0, 1).Width((width-7)/2).Height(3)
	if m.cursor == checkIndex {
		checkStyle = activePane.Copy().Padding(0, 1).Width((width - 7) / 2).Height(3)
	}
	if m.cursor == reviseIndex {
		reviseStyle = activePane.Copy().Padding(0, 1).Width((width - 7) / 2).Height(3)
	}
	continueStyle := panel.Copy().Padding(0, 1).Width(width - 4).Height(2)
	continueText := dimStyle.Render("○  Continue to template selection  ·  Connect every selected service first")
	if m.allSelectedConnected() {
		continueText = lipgloss.NewStyle().Bold(true).Foreground(green).Render("◆  Continue to template selection  →")
	}
	if m.cursor == continueIndex {
		continueStyle = activePane.Copy().Padding(0, 1).Width(width - 4).Height(2)
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Connect selected services") + "\n" + dimStyle.Render("Confirm access before choosing an industry quickstart.") + "\n\n" +
		m.progress.ViewAs(progress) + "  " + fmt.Sprintf("%d/%d connections", connected, len(providers)) + "\n\n" +
		panel.Copy().Padding(0, 1).Width(width-4).Render(strings.Join(rows, "\n")) + "\n" +
		lipgloss.JoinHorizontal(lipgloss.Top,
			checkStyle.Render("◆  Recheck selected services\n   Run all connectivity checks"), " ",
			reviseStyle.Render("↺  Revise component stack\n   Change selected services")) + "\n" +
		continueStyle.Render(continueText)
}

func providerConnectionDetails(result checker.ProviderResult) string {
	parts := make([]string, 0, 4)
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, label+" "+value)
		}
	}
	switch result.Name {
	case "box":
		add("auth", result.Discovery.AuthType)
		add("user", result.Discovery.Identity)
		add("UID", result.Discovery.Account)
		add("EID", result.Discovery.Enterprise)
	case "salesforce":
		add("user", result.Discovery.Identity)
		add("alias", result.Discovery.Profile)
		add("org", result.Discovery.Host)
	case "databricks":
		add("user", result.Discovery.Identity)
		add("profile", result.Discovery.Profile)
		add("workspace", result.Discovery.Host)
	case "aws":
		add("account", result.Discovery.Account)
		add("profile", result.Discovery.Profile)
		add("region", result.Discovery.Region)
	}
	return strings.Join(parts, "  ·  ")
}

func (m rootShellModel) viewProvider(width int) string {
	result := m.results[m.provider]
	detail := "No check has been run."
	if len(result.Checks) > 0 {
		detail = strings.Join(result.Checks, "\n")
	}
	actionLabels := map[string]string{
		"check": "Check connection", "choose": "Choose authenticated profile", "ccg": "Connect with a CCG app (client credentials)", "switch": "Switch Box connection / set default", "connect": "Connect using provider CLI", "back": "Back to launch plan",
	}
	rows := []string{}
	for i, action := range m.providerActions() {
		style := panel.Copy().Width(width - 4)
		if i == m.cursor {
			style = activePane.Copy().Width(width - 4)
		}
		rows = append(rows, style.Render(actionLabels[action]))
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render(providerLabel(m.provider)) + "  " + m.statusLabel(m.provider) + "\n\n" +
		panel.Copy().Width(width-4).Render(detail) + "\n" + strings.Join(rows, "\n")
}

func (m rootShellModel) viewOptions(width int) string {
	options := m.results[m.provider].Discovery.Options
	rows := make([]string, len(options))
	for i, option := range options {
		style := panel.Copy().Width(width - 4)
		if i == m.cursor {
			style = activePane.Copy().Width(width - 4)
		}
		rows[i] = style.Render(option)
	}
	return titleStyle.Render("Choose "+providerLabel(m.provider)+" profile") + "\n\n" + strings.Join(rows, "\n")
}

func (m rootShellModel) viewDatabricksHost(width int) string {
	return titleStyle.Render("Connect Databricks") + "\n" + dimStyle.Render("Use the workspace URL shown in your Databricks browser address bar.") + "\n\n" +
		activePane.Copy().Width(width-4).Render(m.hostInput.View())
}

func (m rootShellModel) viewConfig(width int) string {
	templateName, repository := "Solution template", "Not selected"
	if m.selected != nil {
		templateName, repository = m.selected.name, m.selected.repository
	}
	components := make([]string, 0, len(m.selectedProviders()))
	for _, provider := range m.selectedProviders() {
		components = append(components, providerLabel(provider))
	}
	summary := accent.Render(templateName) + dimStyle.Render("  ·  "+strings.Join(components, " + ")) + "\n" + dimStyle.Render(repository)
	directoryStyle := panel.Copy().Padding(0, 1).Width(width - 4)
	nameStyle := panel.Copy().Padding(0, 1).Width(width - 4)
	strategyStyle := panel.Copy().Padding(0, 1).Width(width - 4)
	boxStyle := panel.Copy().Padding(0, 1).Width(width - 4)
	continueStyle := panel.Copy().Padding(0, 1).Width(width - 4)
	if m.configFocus == 0 {
		directoryStyle = activePane.Copy().Padding(0, 1).Width(width - 4)
	} else if m.configFocus == 1 {
		nameStyle = activePane.Copy().Padding(0, 1).Width(width - 4)
	} else if m.configFocus == 2 {
		strategyStyle = activePane.Copy().Padding(0, 1).Width(width - 4)
	} else if m.configFocus == 3 {
		boxStyle = activePane.Copy().Padding(0, 1).Width(width - 4)
	} else {
		continueStyle = activePane.Copy().Padding(0, 1).Width(width - 4)
	}
	destination := filepath.Join(m.directoryInput.Value(), m.packageInput.Value())
	return m.stageHeader() + "\n\n" + titleStyle.Render("Configure the solution package") + "\n" + dimStyle.Render("Choose a parent folder, edit the package name, then continue.") + "\n\n" +
		summary + "\n" +
		directoryStyle.Render(titleStyle.Render("1  Parent directory  ")+dimStyle.Render("[b browse]")+"\n"+m.directoryInput.View()) + "\n" +
		nameStyle.Render(titleStyle.Render("2  Package directory name")+"\n"+m.packageInput.View()) + "\n" +
		strategyStyle.Render(titleStyle.Render("3  Deployment strategy  ")+dimStyle.Render("[←/→ change]")+"\n"+accent.Render(deploymentStrategyLabel(m.answers.deploymentStrategy))+"\n"+dimStyle.Render(m.deploymentNamePreview())) + "\n" +
		boxStyle.Render(titleStyle.Render("4  Box components  ")+dimStyle.Render("[Enter configure]")+"\n"+accent.Render(strings.ToUpper(m.boxComponentMode))+dimStyle.Render(fmt.Sprintf(" · %d capabilities", len(m.boxCapabilities)))) + "\n" +
		continueStyle.Render(accent.Render("5  Create package  →")+"\n"+dimStyle.Render(destination))
}

func (m *rootShellModel) cycleDeploymentStrategy(delta int) {
	strategies := []string{solution.StrategyCreateNew, solution.StrategyReuse}
	index := slices.Index(strategies, m.answers.deploymentStrategy)
	if index < 0 {
		index = 0
	}
	m.answers.deploymentStrategy = strategies[(index+delta+len(strategies))%len(strategies)]
}

func deploymentStrategyLabel(strategy string) string {
	if strategy == solution.StrategyReuse {
		return "REUSE EXISTING"
	}
	return "CREATE NEW"
}

func (m rootShellModel) deploymentNamePreview() string {
	if m.answers.deploymentStrategy == solution.StrategyReuse {
		return "Uses the configured workspace name and existing matching resources."
	}
	return "Creates a unique workspace using the package run ID; retries reuse that run's workspace."
}

func (m rootShellModel) updateBoxComponents(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		m.screen, m.cursor, m.message = screenWelcome, 0, ""
	case "esc", "left":
		m.screen = screenConfig
		m.setConfigFocus(3)
		m.message = "Returned to package configuration."
	case "enter", "right":
		m.screen = screenConfig
		m.setConfigFocus(3)
		m.message = "Box component selection saved for package creation."
	case "up", "k":
		if len(m.boxCapabilities) > 0 {
			m.cursor = (m.cursor - 1 + len(m.boxCapabilities)) % len(m.boxCapabilities)
		}
	case "down", "j":
		if len(m.boxCapabilities) > 0 {
			m.cursor = (m.cursor + 1) % len(m.boxCapabilities)
		}
	case "a":
		m.boxComponentMode = "all"
	case "n":
		m.boxComponentMode = "none"
	case "d":
		m.boxComponentMode = "defaults"
	case "c":
		m.boxComponentMode = "custom"
	case " ":
		if len(m.boxCapabilities) > 0 {
			capability := m.boxCapabilities[m.cursor]
			m.boxComponentMode = "custom"
			m.boxComponentValues[capability.ID] = !m.boxComponentValues[capability.ID]
		}
	}
	return m, nil
}

func (m rootShellModel) viewBoxComponents(width int) string {
	rows := make([]string, 0, len(m.boxCapabilities))
	for i, capability := range m.boxCapabilities {
		enabled := m.boxCapabilitySelected(capability)
		status := strings.ToUpper(capability.API)
		if capability.Handler != "" {
			status = "READY"
		} else if capability.API == "public" {
			status = "ADAPTER PENDING"
		}
		// Same marker vocabulary and colours the validate and deploy checklists
		// use: green tick for what will be deployed, gold ring for anything left
		// manual, dim ring for what is not selected.
		marker, tone := "○", muted
		switch {
		case !enabled:
			status = "EXCLUDED"
		case capability.Handler != "":
			marker, tone = "✓", green
		default:
			tone = gold
		}
		line := fmt.Sprintf("%s  %-25s %s", marker, capability.ComponentType, status)
		style := lipgloss.NewStyle().Width(width - 10).Foreground(tone)
		if i == m.cursor {
			style = style.Copy().Bold(true).Background(lipgloss.Color("#12384A"))
		}
		rows = append(rows, style.Render(line))
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("This template does not declare configurable Box capabilities."))
	}
	mode := accent.Render(strings.ToUpper(m.boxComponentMode))
	controls := dimStyle.Render("a enable all · n disable all · d defaults · c customize · Space toggle · Enter save")
	return m.stageHeader() + "\n\n" + titleStyle.Render("Configure Box components") + "  " + mode + "\n" +
		dimStyle.Render("Global modes override individual values. Toggling a row switches to Custom.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n")) + "\n" + controls
}

func (m rootShellModel) boxCapabilitySelected(capability solution.Capability) bool {
	defaultEnabled := capability.EnabledByDefault == nil || *capability.EnabledByDefault
	switch m.boxComponentMode {
	case "all":
		return true
	case "none":
		return false
	case "custom":
		return m.boxComponentValues[capability.ID]
	default:
		return defaultEnabled
	}
}

func (m rootShellModel) viewDirectoryPicker(width int) string {
	content := titleStyle.Render("Browsing: ") + accent.Render(m.directoryPicker.CurrentDirectory) + "\n\n" + m.directoryPicker.View()
	controls := dimStyle.Render("↑/↓ navigate  ·  Enter/→ open  ·  ← parent  ·  Space choose highlighted  ·  c choose current  ·  Esc cancel")
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose the parent directory") + "\n" + dimStyle.Render("Navigate the folder tree, then choose where Box Dispatch should create the package.") + "\n\n" +
		activePane.Copy().Padding(1, 2).Width(width-4).Render(content) + "\n" + controls
}

func (m rootShellModel) viewPackage(width int) string {
	status := m.spinner.View() + " PACKAGING"
	color := gold
	detail := "Cloning the selected quickstart and filtering provider-specific components."
	if m.packageDone {
		status, color = "● PACKAGE COMPLETE", green
		detail = "Created " + m.packagePath + "\nDetached upstream Git metadata and wrote .dispatch/package.json."
	} else if !m.packageStarted {
		status, color = "○ PACKAGE NOT CREATED", muted
		detail = "Press Enter to retry packaging with the configured destination."
	}
	content := lipgloss.NewStyle().Bold(true).Foreground(color).Render(status) + "\n\n" + detail
	if m.packageDone {
		content += "\n\n" + accent.Render("Press Enter / → to validate provider configuration")
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Assemble the solution package") + "\n" +
		panel.Copy().BorderForeground(color).Width(width-4).Padding(2, 3).Render(content)
}

func (m rootShellModel) viewValidate(width int) string {
	rows := []string{}
	for _, provider := range m.selectedProviders() {
		rows = append(rows, m.providerProgressRow(provider, m.validationProgress[provider], findLifecycleItem(m.validationItems, provider), provider == m.currentValidation, "validate", width-10))
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("No validation results yet."))
	}
	status := "VALIDATION RESULTS"
	if m.validateRunning {
		status = m.spinner.View() + " VALIDATING PACKAGE AND PROVIDERS"
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render(status) + "\n" + dimStyle.Render("Receipts identify existing provider state; unverified packaged assets remain visible as deployment work.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n\n")) + "\n" + accent.Render("Enter / →  Continue to Deploy    r  Validate again")
}

// viewTeardown previews exactly what a reset will delete, then reports the
// outcome per resource once it runs.
func (m rootShellModel) viewTeardown(width int) string {
	title := titleStyle.Render("Reset demo environment")
	if m.teardownRecord == nil {
		return title + "\n" + dimStyle.Render("No deployment is selected.")
	}
	record := *m.teardownRecord
	subtitle := dimStyle.Render(fmt.Sprintf("Deployment %s  ·  %s", record.DeploymentID, record.PackageRoot))

	if m.teardownError != "" {
		return title + "\n" + subtitle + "\n\n" + lipgloss.NewStyle().Foreground(gold).Render(m.teardownError)
	}

	body := []string{}
	if m.teardownDone || m.teardownStarted {
		for _, provider := range m.teardownProviders {
			body = append(body, m.providerProgressRow(provider, m.teardownProgress[provider], nil, provider == m.currentTeardown, "teardown", width-10))
		}
		for _, result := range m.teardownResults {
			body = append(body, "", accent.Render(strings.ToUpper(providerLabel(result.Provider)))+"  "+dimStyle.Render(result.Detail))
			for _, outcome := range result.Remaining() {
				marker, style := "×", lipgloss.NewStyle().Foreground(coral)
				reason := outcome.Error
				if outcome.Unmanaged {
					marker, style = "○", lipgloss.NewStyle().Foreground(gold)
					reason = "no delete API; remove manually"
				}
				body = append(body, style.Render(fmt.Sprintf("  %s %s %s", marker, outcome.Resource.Kind, outcome.Resource.Name))+dimStyle.Render("  "+reason))
			}
		}
	} else {
		// Preview: every resource the reset would delete, by kind and id.
		body = append(body, lipgloss.NewStyle().Bold(true).Foreground(coral).Render("The following resources will be permanently deleted:"))
		for _, provider := range record.Providers {
			if len(provider.Resources) == 0 {
				continue
			}
			body = append(body, "", accent.Render(strings.ToUpper(providerLabel(provider.Provider))))
			for _, resource := range provider.Resources {
				body = append(body, dimStyle.Render(fmt.Sprintf("  • %-18s %s", resource.Kind, resource.Name))+lipgloss.NewStyle().Foreground(muted).Render("  "+resource.ID))
			}
		}
	}

	panelBody := panel.Copy().Padding(1, 2).Width(width - 4).Render(strings.Join(body, "\n"))
	if m.confirmingTeardown && m.teardownConfirmForm != nil {
		return title + "\n" + subtitle + "\n\n" + panelBody + "\n" + activePane.Copy().Width(width-4).Render(m.teardownConfirmForm.View())
	}
	return title + "\n" + subtitle + "\n\n" + panelBody
}

func (m rootShellModel) viewDeploy(width int) string {
	rows := []string{}
	deployable := 0
	for _, item := range m.validationItems {
		if item.Status == lifecycle.StatusMissing && item.Deployable {
			deployable++
		}
		rows = append(rows, m.providerProgressRow(item.Provider, m.deploymentProgress[item.Provider], &item, item.Provider == m.currentDeployment, "deploy", width-10))
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("Validate the package before deployment."))
	}
	action := accent.Render(fmt.Sprintf("Enter / →  Deploy %d supported missing configuration set(s)", deployable))
	if m.confirmingDeploy && m.deployConfirmForm != nil {
		action = activePane.Copy().Width(width - 4).Render(m.deployConfirmForm.View())
	}
	if m.deployStarted {
		// Box Forms and Apps are provisioned through the browser, which may raise
		// a Box sign-in window and wait for it before the deploy continues.
		action = m.spinner.View() + " Deploying provider configuration...\n" +
			dimStyle.Render("   If a Box sign-in window opens, complete it and the deployment continues.")
	} else if m.deployDone {
		action = lipgloss.NewStyle().Bold(true).Foreground(green).Render("Deployment run complete")
		if m.deploymentAuditPath == "" {
			action += "\n" + accent.Render("Enter / →  Export deployment audit log")
		} else {
			action += "\n" + lipgloss.NewStyle().Foreground(green).Render("✓ Deployment audit exported")
			action += "\n" + dimStyle.Render(m.deploymentAuditPath)
			action += "\n" + accent.Render("Enter / →  Return to Box Dispatch home")
		}
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Deploy missing configuration") + "\n" + dimStyle.Render("Box Dispatch runs only native deploy adapters and leaves unsupported/manual work explicit.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n\n")) + "\n" + action
}

func (m rootShellModel) providerProgressRow(provider string, value float64, item *lifecycle.Item, active bool, phase string, width int) string {
	state := dimStyle.Render("PENDING")
	switch {
	case active:
		state = lipgloss.NewStyle().Bold(true).Foreground(gold).Render(m.spinner.View() + " IN PROGRESS")
	case item != nil && item.Status == lifecycle.StatusFailed:
		// A finished run that failed is not complete; saying so contradicted the
		// failure reported directly underneath it.
		state = lipgloss.NewStyle().Bold(true).Foreground(coral).Render("FAILED")
	case value >= 1:
		state = lipgloss.NewStyle().Bold(true).Foreground(green).Render("COMPLETE")
	}
	header := titleStyle.Render(providerLabel(provider)) + "  " + state
	row := header + "\n" + m.progress.ViewAs(value)
	if item != nil {
		row += "\n" + lifecycleRow(*item, width)
	}
	if item != nil && (len(item.Planned) > 0 || len(item.Present) > 0 || len(item.Missing) > 0) {
		row += "\n" + renderComponentChecklist(*item, phase, active, value, m.spinner.View(), width)
	}
	return row
}

func renderComponentChecklist(item lifecycle.Item, phase string, active bool, progressValue float64, spinnerView string, width int) string {
	if item.Provider == "salesforce" && len(item.Missing) == 0 {
		lines := []string{
			accent.Render("DEPLOYMENT CHECKLIST"),
			lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("✓ Salesforce metadata  100%% · %d components present", len(item.Present))),
		}
		return lipgloss.NewStyle().MaxWidth(width).PaddingLeft(2).Render(strings.Join(lines, "\n"))
	}
	type counts struct{ present, deployable, pending, experimental, manual int }
	groups := map[string]counts{}
	for _, component := range item.Planned {
		name := strings.SplitN(component, ":", 2)[0]
		count := groups[name]
		count.pending++
		groups[name] = count
	}
	for _, component := range item.Present {
		name := strings.SplitN(component, ":", 2)[0]
		count := groups[name]
		count.present++
		groups[name] = count
	}
	for _, component := range item.Missing {
		name := strings.SplitN(component, ":", 2)[0]
		count := groups[name]
		if slices.Contains(item.DeployableComponents, component) {
			count.deployable++
		} else if slices.Contains(item.AdapterPending, component) {
			count.pending++
		} else if slices.Contains(item.Experimental, component) {
			count.experimental++
		} else {
			count.manual++
		}
		groups[name] = count
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	order := item.ComponentOrder
	slices.SortStableFunc(names, func(a, b string) int { return slices.Index(order, a) - slices.Index(order, b) })
	deployableNames := make([]string, 0, len(names))
	for _, name := range names {
		if groups[name].deployable > 0 {
			deployableNames = append(deployableNames, name)
		}
	}
	lines := []string{accent.Render("DEPLOYMENT CHECKLIST")}
	for index, name := range names {
		count := groups[name]
		if len(item.Planned) > 0 {
			categoryProgress := progressValue*float64(len(names)) - float64(index)
			categoryProgress = math.Min(math.Max(categoryProgress, 0), 1)
			percentage := int(categoryProgress * 100)
			marker := "○"
			style := dimStyle
			if active && categoryProgress > 0 && categoryProgress < 1 {
				marker = spinnerView
				style = lipgloss.NewStyle().Foreground(gold)
			} else if active && categoryProgress >= 1 {
				marker = "✓"
				style = lipgloss.NewStyle().Foreground(green)
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s %-22s %3d%% · %d pending", marker, name, percentage, count.pending)))
			continue
		}
		parts := []string{fmt.Sprintf("%d present", count.present)}
		if count.deployable > 0 {
			parts = append(parts, fmt.Sprintf("%d to deploy", count.deployable))
		}
		if count.pending > 0 {
			parts = append(parts, fmt.Sprintf("%d adapter pending", count.pending))
		}
		if count.experimental > 0 {
			parts = append(parts, fmt.Sprintf("%d experimental", count.experimental))
		}
		if count.manual > 0 {
			parts = append(parts, fmt.Sprintf("%d later/manual", count.manual))
		}
		detail := strings.Join(parts, " · ")
		if phase == "validate" {
			lines = append(lines, lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("✓ %-22s 100%% · %s", name, detail)))
			continue
		}
		if count.deployable == 0 && count.pending == 0 && count.experimental == 0 && count.manual == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("✓ %-22s 100%% · %s", name, detail)))
			continue
		}
		total := count.present + count.deployable + count.pending + count.experimental + count.manual
		percentage := 0
		if total > 0 {
			percentage = count.present * 100 / total
		}
		marker := "○"
		style := lipgloss.NewStyle().Foreground(gold)
		if active && count.deployable > 0 {
			position := slices.Index(deployableNames, name)
			categoryProgress := progressValue*float64(len(deployableNames)) - float64(position)
			categoryProgress = math.Min(math.Max(categoryProgress, 0), 1)
			percentage = (count.present*100 + int(categoryProgress*100)*count.deployable) / total
			if categoryProgress > 0 && categoryProgress < 1 {
				marker = spinnerView
			} else if categoryProgress >= 1 {
				marker = "✓"
				style = lipgloss.NewStyle().Foreground(green)
			}
		} else if item.Status == lifecycle.StatusFailed && progressValue >= 1 && count.deployable > 0 {
			marker = "×"
			style = lipgloss.NewStyle().Foreground(coral)
		} else if percentage >= 100 {
			marker = "✓"
			style = lipgloss.NewStyle().Foreground(green)
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s %-22s %3d%% · %s", marker, name, percentage, detail)))
	}
	return lipgloss.NewStyle().MaxWidth(width).PaddingLeft(2).Render(strings.Join(lines, "\n"))
}

func findLifecycleItem(items []lifecycle.Item, provider string) *lifecycle.Item {
	for i := range items {
		if items[i].Provider == provider {
			return &items[i]
		}
	}
	return nil
}

func lifecycleRow(item lifecycle.Item, width int) string {
	color, marker := gold, "◆"
	switch item.Status {
	case lifecycle.StatusPending:
		color, marker = muted, "○"
	case lifecycle.StatusPresent:
		color, marker = green, "●"
	case lifecycle.StatusFailed:
		color, marker = coral, "×"
	case lifecycle.StatusManual:
		color, marker = muted, "◇"
	}
	label := lipgloss.NewStyle().Bold(true).Foreground(color).Render(marker + " " + strings.ToUpper(item.Provider) + "  " + string(item.Status))
	detail := dimStyle.Copy().Width(width).Render(item.Name + "\n" + item.Detail)
	return label + "\n" + detail
}

func (m rootShellModel) statusLabel(provider string) string {
	switch m.statuses[provider] {
	case connectionChecking:
		return m.spinner.View() + " " + lipgloss.NewStyle().Foreground(gold).Render("CHECKING")
	case connectionConnected:
		return lipgloss.NewStyle().Bold(true).Foreground(green).Render("● CONNECTED")
	case connectionFailed:
		return lipgloss.NewStyle().Bold(true).Foreground(coral).Render("● ACTION NEEDED")
	default:
		return dimStyle.Render("○ NOT CHECKED")
	}
}

func (m rootShellModel) footer() string {
	bindings := contextualHelp{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "steps")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	}
	if m.screen == screenConfig {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("tab", "up", "down"), key.WithHelp("tab/↑/↓", "change field")),
			key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "browse folders")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/continue")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	if m.screen == screenDirectoryPicker {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("enter", "right"), key.WithHelp("enter/→", "open")),
			key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "parent")),
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "choose")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "current")),
		}
	}
	if m.screen == screenComponents {
		bindings = append(bindings, key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")))
	}
	if m.screen == screenHistory {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "browse history")),
			key.NewBinding(key.WithKeys("left", "esc"), key.WithHelp("←/esc", "home")),
		}
	}
	if m.screen == screenBoxSwitch {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "set default")),
			key.NewBinding(key.WithKeys("left", "esc"), key.WithHelp("←/esc", "back")),
		}
	}
	quitHelp := "home"
	if m.screen == screenWelcome {
		quitHelp = "quit"
	}
	if !(m.screen == screenConfig && m.configFocus < 2) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("q"), key.WithHelp("q", quitHelp)))
	}
	helpView := m.help.View(bindings)
	if m.message != "" {
		return lipgloss.NewStyle().Foreground(gold).Render(m.message) + "\n" + helpView
	}
	return helpView
}

func providerLabel(provider string) string {
	switch provider {
	case "box":
		return "Box"
	case "salesforce":
		return "Agentforce"
	case "databricks":
		return "Databricks"
	case "aws":
		return "AWS Bedrock AgentCore"
	default:
		return provider
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m rootShellModel) stageHeader() string {
	if m.screen == screenDashboard {
		return m.stepper()
	}
	value := m.overallJourneyProgress()
	percent := lipgloss.NewStyle().Bold(true).Foreground(cyan).Render(fmt.Sprintf("%3.0f%%", value*100))
	return m.stepper() + "\n\n" + dimStyle.Render("OVERALL PROGRESS") + "  " + percent + "\n" + m.progress.ViewAs(value)
}

func (m rootShellModel) overallJourneyProgress() float64 {
	switch m.screen {
	case screenComponents:
		return 0.08
	case screenDashboard, screenProvider, screenOptions, screenDatabricksHost:
		return 0.18
	case screenTemplates:
		return 0.35
	case screenConfig, screenDirectoryPicker:
		return 0.48
	case screenPackage:
		if m.packageDone {
			return 0.65
		}
		return 0.55
	case screenValidate:
		if m.validateDone {
			return 0.82
		}
		return 0.72
	case screenDeploy:
		if m.deployDone {
			return 1
		}
		return 0.9
	default:
		return 0
	}
}
