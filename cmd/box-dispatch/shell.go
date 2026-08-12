package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
	"github.com/unofficialbox/box-dispatch/internal/workspace"
)

// welcomeOptions is the home menu; the key handler and the view read the same
// slice so the cursor range never drifts from what is rendered.
var welcomeOptions = []string{"Start new deployment", "Show deployment history"}

// dispatchBanner is the ANSI-shadow product wordmark (6 rows, 60 cols) shown in
// the welcome hero on wide terminals; smaller terminals fall back to a one-line
// headline. boxBanner is the half-height BOX kicker above it, and chevronBanner
// is the large coral » accent shown to the right of DISPATCH on wide terminals.
var dispatchBanner = []string{
	"██████╗ ██╗███████╗██████╗  █████╗ ████████╗ ██████╗██╗  ██╗",
	"██╔══██╗██║██╔════╝██╔══██╗██╔══██╗╚══██╔══╝██╔════╝██║  ██║",
	"██║  ██║██║███████╗██████╔╝███████║   ██║   ██║     ███████║",
	"██║  ██║██║╚════██║██╔═══╝ ██╔══██║   ██║   ██║     ██╔══██║",
	"██████╔╝██║███████║██║     ██║  ██║   ██║   ╚██████╗██║  ██║",
	"╚═════╝ ╚═╝╚══════╝╚═╝     ╚═╝  ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝",
}

var boxBanner = []string{
	"█▀▄ █▀█ █ █",
	"█▀▄ █ █ ▄▀▄",
	"▀▀  ▀▀▀ ▀ ▀",
}

var chevronBanner = []string{
	"██╗ ██╗  ",
	"╚██╗╚██╗ ",
	" ╚██╗╚██╗",
	" ██╔╝██╔╝",
	"██╔╝██╔╝ ",
	"╚═╝ ╚═╝  ",
}

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
	screenHelp
	screenDiagnostic
	screenScratchConfirm
	screenDevHubs
)

type connectionStatus int

const (
	connectionPending connectionStatus = iota
	connectionChecking
	connectionConnected
	connectionFailed
)

type deploymentPhase string

const (
	deploymentPhaseReview   deploymentPhase = "review"
	deploymentPhasePackage  deploymentPhase = "package"
	deploymentPhaseValidate deploymentPhase = "validate"
	deploymentPhaseApply    deploymentPhase = "deploy"
	deploymentPhaseFailed   deploymentPhase = "failed"
	deploymentPhaseComplete deploymentPhase = "complete"
)

type componentChoice struct {
	provider   string
	name       string
	role       string
	selected   bool
	required   bool
	comingSoon bool
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

type postDeployOpenFinishedMsg struct {
	label string
	err   error
}

type scratchOrgFinishedMsg struct {
	alias string
	info  salesforceorg.Info
	err   error
}

type devHubListFinishedMsg struct {
	hubs []salesforceorg.DevHub
	err  error
}

type devHubLoginFinishedMsg struct {
	err error
}

type packageFinishedMsg struct {
	manifest workspace.PackageManifest
	err      error
}

type accessibleFormKind string

const (
	accessibleChooseForm   accessibleFormKind = "choose"
	accessibleTemplateForm accessibleFormKind = "template"
	accessibleTeardownForm accessibleFormKind = "teardown"
	accessibleBoxCCGForm   accessibleFormKind = "box-ccg"
)

type accessibleFormFinishedMsg struct {
	kind accessibleFormKind
	err  error
}

type accessibleFormCommand struct {
	form   *huh.Form
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (c *accessibleFormCommand) Run() error {
	return c.form.WithAccessible(true).WithInput(c.stdin).WithOutput(c.stdout).Run()
}

func (c *accessibleFormCommand) SetStdin(reader io.Reader)  { c.stdin = reader }
func (c *accessibleFormCommand) SetStdout(writer io.Writer) { c.stdout = writer }
func (c *accessibleFormCommand) SetStderr(writer io.Writer) { c.stderr = writer }

// activityMsg is one live progress line emitted by a running lifecycle task.
type activityMsg struct {
	provider string
	line     string
}

// activityLogCap bounds the retained step history so a very long run cannot grow
// memory without limit; the collapsed feed only shows the tail anyway.
const activityLogCap = 200

// waitForActivity blocks on the task channel and surfaces the next message —
// either an activityMsg step or the task's final provider*FinishedMsg — as a
// tea.Msg. A closed channel yields nil, ending the read loop.
func waitForActivity(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
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
	quickstarts        []string
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
	divider    = lipgloss.Color("#25384F") // subtle rule between chrome and content
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

	// ThemeCharm titles are indigo and its select cursors/indicators fuchsia —
	// both hard to read on our navy panes. Remap onto our scheme: titles white,
	// pointers coral, prompts cyan.
	theme.Focused.Title = theme.Focused.Title.Foreground(white).Bold(true)
	theme.Focused.NoteTitle = theme.Focused.NoteTitle.Foreground(white).Bold(true)
	theme.Blurred.Title = theme.Blurred.Title.Foreground(muted).Bold(true)
	theme.Blurred.NoteTitle = theme.Blurred.NoteTitle.Foreground(muted).Bold(true)
	theme.Group.Title = theme.Focused.Title
	for _, sel := range []*lipgloss.Style{
		&theme.Focused.SelectSelector, &theme.Focused.MultiSelectSelector,
		&theme.Focused.NextIndicator, &theme.Focused.PrevIndicator,
	} {
		*sel = sel.Foreground(coral)
	}
	theme.Focused.TextInput.Prompt = theme.Focused.TextInput.Prompt.Foreground(cyan)
	theme.Focused.TextInput.Cursor = theme.Focused.TextInput.Cursor.Foreground(coral)
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
	activityLog           []string     // live step lines for the running task
	activityExpanded      bool         // whether the activity feed is expanded
	activityCh            chan tea.Msg // steps + final result from the running task
	help                  help.Model
	helpReturn            shellScreen
	helpScroll            int
	accessibleForms       bool
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
	lifecycleScroll       int  // first visible row of validation/deployment provider details
	lifecycleFollowTail   bool // live validation/deployment follows the newest checklist row
	deployStarted         bool
	deployDone            bool
	deployShowDetails     bool // progressive disclosure for provider checklists and deployed resources
	confirmingDeploy      bool
	deployConfirmCursor   int // 0 = affirmative (Deploy), 1 = Cancel
	deployConfirmTitle    string
	deployConfirmDesc     string
	deployConfirmAffirm   string
	deploymentQueue       []lifecycle.Item
	deploymentProgress    map[string]float64
	currentDeployment     string
	deploymentBaseline    []lifecycle.Item
	deploymentStartedAt   time.Time
	deploymentCompletedAt time.Time
	deploymentAuditPath   string
	deployAssetsScroll    int // first visible row of the deployed-assets table
	deploymentPhase       deploymentPhase
	deploymentHistory     []deploymentaudit.DeploymentRecord
	historyError          string
	validationItems       []lifecycle.Item
	message               string
	providerDiagnostics   map[string]string
	diagnosticTitle       string
	diagnosticBody        string
	diagnosticReturn      shellScreen
	diagnosticScroll      int
	salesforceCreating    bool
	pendingScratchAlias   string
	scratchConfirmCursor  int
	devHubs               []salesforceorg.DevHub
	selectedDevHub        salesforceorg.DevHub

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
	teardownScroll       int // first visible row of the (often long) resource preview

	// Box CCG credential entry. The values are held behind pointers so the huh
	// form writes to stable heap storage: bubbletea passes the model by value, so
	// a plain string field would be bound by the address of a copy huh keeps
	// updating while saveBoxCCG reads a different, stale copy.
	boxCCGForm      *huh.Form
	ccgClientID     *string
	ccgClientSecret *string
	ccgSubjectType  *string
	ccgSubjectID    *string

	// Box connection switcher.
	boxConnections   []boxconn.Connection
	boxRemovePending string // connection name awaiting a second remove keypress
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
		// Blue → red; end deliberately red rather than the pinker coral accent.
		progress.WithGradient("#0866D9", "#E23A2C"),
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
			components[i].selected = components[i].provider == "box" || (!components[i].comingSoon && components[i].provider == provider)
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
		quickstarts:        []string{activeTemplateID},
		templateID:         activeTemplateID,
		directory:          filepath.Dir(cwd),
		packageName:        packageNameForTemplate(templates, activeTemplateID),
		deploymentStrategy: solution.StrategyCreateNew,
	}
	m := rootShellModel{
		screen:              screenWelcome,
		components:          components,
		templates:           templates,
		statuses:            map[string]connectionStatus{},
		results:             map[string]checker.ProviderResult{},
		providerDiagnostics: map[string]string{},
		validationProgress:  map[string]float64{},
		deploymentProgress:  map[string]float64{},
		spinner:             spin,
		hostInput:           host,
		progress:            bar,
		help:                helpModel,
		answers:             answers,
	}
	if uiSettings, err := shellstate.LoadUISettings(); err == nil {
		m.accessibleForms = uiSettings.AccessibleForms
	}
	m.rebuildComponentForm()
	m.rebuildTemplateForm()
	m.prepareConfigInputs()
	m.prepareBoxComponentSelection()
	return m
}

func newDispatchShell() rootShellModel { return newSetupOnlyShell() }

func newResetShell() rootShellModel {
	m := newDispatchShell()
	history, err := deploymentaudit.ListDeployments()
	m.deploymentHistory = history
	m.screen, m.cursor = screenHistory, 0
	m.message = "Choose the deployment to reset, then press enter."
	if err != nil {
		m.historyError = err.Error()
	} else if len(history) == 0 {
		m.historyError = "No deployment has been recorded yet."
	}
	return m
}

// componentsFromRuntime builds the provider portion of the Choose picker from
// the active BCL scenario, translating BCL provider IDs to internal keys and
// filling display copy from the provider config (falling back to built-in copy
// for anything the config omits). Availability is fixed and independent of the
// selected quickstart; BCL only enriches each platform's display copy.
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
		{provider: "salesforce", name: "Salesforce", role: "CRM, structured records, and customer workflows"},
		{provider: "databricks", name: "Databricks", role: "Analytics, models, and data intelligence", comingSoon: true},
		{provider: "aws", name: "AWS Bedrock AgentCore", role: "Agent runtime and orchestration", comingSoon: true},
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
	templateOptions := make([]huh.Option[string], 0, len(m.templates))
	for _, template := range m.templates {
		templateOptions = append(templateOptions, huh.NewOption(template.name, template.id).Selected(template.id == m.answers.templateID))
	}
	m.answers.quickstarts = []string{m.answers.templateID}
	componentOptions := make([]huh.Option[string], 0, len(m.components))
	for _, component := range m.components {
		label := component.name
		if component.required {
			label += "  ·  REQUIRED"
		} else if component.comingSoon {
			label = dimStyle.Render(label + "  (Coming soon)")
		}
		componentOptions = append(componentOptions, huh.NewOption(label, component.provider).Selected(!component.comingSoon && slices.Contains(m.answers.components, component.provider)))
	}
	m.componentForm = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Key("template").
				Title("Choose a solution quickstart").
				Description("Highlight with arrows, then press Space to select one.").
				Options(templateOptions...).
				Limit(1).
				Value(&m.answers.quickstarts).
				Validate(func(values []string) error {
					if len(values) != 1 {
						return errors.New("choose one solution quickstart")
					}
					return nil
				}),
			huh.NewMultiSelect[string]().
				Key("components").
				Title("Choose platform components").
				Description("Box is required; partner platforms are optional.").
				Options(componentOptions...).
				Value(&m.answers.components).
				Validate(func(values []string) error {
					if !slices.Contains(values, "box") {
						return errors.New("Box is required for every Box Dispatch solution")
					}
					for _, component := range m.components {
						if component.comingSoon && slices.Contains(values, component.provider) {
							return fmt.Errorf("%s is coming soon and cannot be selected yet", component.name)
						}
					}
					return nil
				}),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithAccessible(m.accessibleForms).WithWidth(76)
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
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithAccessible(m.accessibleForms).WithWidth(76)
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
	uiSettings, err := shellstate.LoadUISettings()
	if err != nil {
		m.message = "Could not load .dispatch/ui-settings.bcl: " + err.Error()
		return
	}
	if uiSettings.BoxComponentVisibility == nil {
		uiSettings.BoxComponentVisibility = map[string]bool{}
	}
	settingsChanged := false
	for _, capability := range manifest.Box.Capabilities {
		id := manifest.CapabilityID(capability)
		visible, configured := uiSettings.BoxComponentVisibility[id]
		if !configured {
			visible = capability.CanDeploy()
			uiSettings.BoxComponentVisibility[id] = visible
			settingsChanged = true
		}
		if !visible {
			continue
		}
		m.boxCapabilities = append(m.boxCapabilities, capability)
	}
	if settingsChanged {
		if err := shellstate.SaveUISettings(uiSettings); err != nil {
			m.message = "Could not write .dispatch/ui-settings.bcl: " + err.Error()
		}
	}
	for _, capability := range m.boxCapabilities {
		if !capability.CanDeploy() {
			continue
		}
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
	if len(m.answers.quickstarts) != 1 {
		return errors.New("choose one solution quickstart")
	}
	m.answers.templateID = m.answers.quickstarts[0]
	if !slices.Contains(m.answers.components, "box") {
		return errors.New("Box is required for every Box Dispatch solution")
	}
	m.answers.components = slices.DeleteFunc(m.answers.components, func(provider string) bool {
		for _, component := range m.components {
			if component.provider == provider {
				return component.comingSoon
			}
		}
		return false
	})
	for i := range m.components {
		m.components[i].selected = !m.components[i].comingSoon && slices.Contains(m.answers.components, m.components[i].provider)
	}
	return nil
}

func (m rootShellModel) selectTemplateAndConfigure() (tea.Model, tea.Cmd) {
	m.selectTemplate()
	m.prepareConfigInputs()
	m.prepareBoxComponentSelection()
	m.savePlan()
	m.screen, m.cursor, m.configFocus = screenConfig, 0, 0
	return m, textinput.Blink
}

func (m *rootShellModel) selectTemplate() {
	m.selected = nil
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
	m.answers.packageName = packageNameForTemplate(m.templates, m.selected.id)
}

func (m *rootShellModel) packageRequest() (workspace.PackageRequest, error) {
	if m.selected == nil {
		return workspace.PackageRequest{}, errors.New("choose a solution quickstart before reviewing the deployment")
	}
	if strings.TrimSpace(m.selected.repository) == "" {
		return workspace.PackageRequest{}, errors.New("this solution has no source repository configured; set a repository for scenario " + m.selected.id + " in the runtime config before deploying")
	}
	m.answers.directory = strings.TrimSpace(m.directoryInput.Value())
	m.answers.packageName = strings.TrimSpace(m.packageInput.Value())
	info, err := os.Stat(m.answers.directory)
	if err != nil || !info.IsDir() {
		return workspace.PackageRequest{}, errors.New("choose an existing parent directory")
	}
	name := m.answers.packageName
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return workspace.PackageRequest{}, errors.New("enter a valid package directory name without slashes")
	}
	m.packagePath = filepath.Join(m.answers.directory, strings.TrimSpace(m.answers.packageName))
	return workspace.PackageRequest{
		Repository: m.selected.repository, Destination: m.packagePath,
		TemplateID: m.selected.id, Components: m.selectedProviders(), BoxComponents: m.boxComponentSelection(),
		BoxStrategy: m.answers.deploymentStrategy,
	}, nil
}

func (m rootShellModel) prepareReview() (tea.Model, tea.Cmd) {
	if _, err := m.packageRequest(); err != nil {
		m.message = err.Error()
		if strings.Contains(err.Error(), "directory name") {
			m.setConfigFocus(1)
		}
		return m, nil
	}
	m.deploymentPhase = deploymentPhaseReview
	m.screen, m.cursor = screenPackage, 0
	m.message = "Review the deployment plan before anything is created or changed."
	m.savePlan()
	return m, nil
}

func (m rootShellModel) startPackage() (tea.Model, tea.Cmd) {
	req, err := m.packageRequest()
	if err != nil {
		m.deploymentPhase = deploymentPhaseFailed
		m.message = "Package failed: " + err.Error()
		return m, nil
	}
	m.packageStarted = true
	m.packageDone = false
	m.screen = screenDeploy
	m.deploymentPhase = deploymentPhasePackage
	m.message = "Pulling the selected solution components from GitHub..."
	return m, tea.Batch(m.spinner.Tick, packageCmd(req))
}

func (m rootShellModel) startDeploymentPipeline() (tea.Model, tea.Cmd) {
	m.packageStarted = false
	m.validateStarted, m.validateRunning, m.validateDone = false, false, false
	m.deployStarted, m.deployDone = false, false
	m.deployShowDetails = false
	m.validationItems = nil
	if m.packageDone {
		m.deploymentPhase = deploymentPhaseValidate
		m.message = "Reusing the assembled package. Validating provider configuration and prerequisites..."
		return m.startValidation()
	}
	m.deploymentPhase = deploymentPhasePackage
	return m.startPackage()
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
	m.activityLog, m.activityExpanded = nil, false
	m.lifecycleScroll = 0
	m.lifecycleFollowTail = true
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
	next, cmd := m.startNextValidation()
	result := next.(rootShellModel)
	result.followLifecycleTail()
	return result, cmd
}

func (m rootShellModel) startDeploy() (tea.Model, tea.Cmd) {
	m.deploymentPhase = deploymentPhaseApply
	m.deployStarted = true
	m.deployDone = false
	m.activityLog, m.activityExpanded = nil, false
	m.lifecycleScroll = 0
	m.lifecycleFollowTail = true
	m.deployAssetsScroll = 0
	m.deployShowDetails = false
	m.deploymentBaseline = append([]lifecycle.Item(nil), m.validationItems...)
	m.deploymentStartedAt = time.Now().UTC()
	m.deploymentCompletedAt = time.Time{}
	m.deploymentAuditPath = ""
	m.deploymentQueue = append([]lifecycle.Item(nil), m.validationItems...)
	m.deploymentProgress = map[string]float64{}
	for _, item := range m.deploymentQueue {
		m.deploymentProgress[item.Provider] = 0
	}
	next, cmd := m.startNextDeployment()
	result := next.(rootShellModel)
	result.followLifecycleTail()
	return result, cmd
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
	m.confirmingDeploy = true
	m.lifecycleScroll = 0
	m.deployConfirmCursor = 1 // default to the safe choice (Cancel)
	m.deployConfirmTitle = fmt.Sprintf("Deploy %d provider configuration set(s)?", deployable)
	m.deployConfirmDesc = "Box Dispatch will deploy missing configuration to: " + strings.Join(providers, ", ") + ". Existing components will be skipped."
	m.deployConfirmAffirm = "Deploy"
	if deployable == 0 {
		m.deployConfirmTitle = "Complete with no deployment changes?"
		m.deployConfirmDesc = "Validation found no supported missing configuration. No provider commands will run."
		m.deployConfirmAffirm = "Complete"
	}
	if m.deploymentPhase == deploymentPhaseReview {
		m.deployConfirmTitle = "Deploy this solution?"
		m.deployConfirmDesc = "Dispatch will assemble the package, validate provider state and prerequisites, then apply only supported missing configuration to: " + strings.Join(m.selectedProviderLabels(), ", ") + "."
		m.deployConfirmAffirm = "Deploy"
	}
	m.message = "Review and confirm the deployment plan."
	return m, nil
}

// renderDeployConfirm draws the destructive-confirm prompt: a coral title, the
// plan description, and two fixed-colour buttons — Deploy always blue, Cancel
// always coral — with the focused one filled so the choice reads at a glance.
func (m rootShellModel) renderDeployConfirm(width int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(coral).Width(width).Render(m.deployConfirmTitle)
	desc := dimStyle.Copy().Width(width).Render(m.deployConfirmDesc)

	deploy := confirmButton(m.deployConfirmAffirm, cyan, m.deployConfirmCursor == 0)
	cancel := confirmButton("Cancel", coral, m.deployConfirmCursor == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, deploy, "  ", cancel)
	return title + "\n\n" + desc + "\n\n" + buttons
}

// confirmButton renders one confirm button in its fixed hue: filled (white on
// the hue) when focused, outlined (the hue on the shell ground) otherwise.
func confirmButton(label string, hue lipgloss.Color, focused bool) string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 3)
	if focused {
		return style.Foreground(white).Background(hue).Render(label)
	}
	return style.Foreground(hue).Background(lipgloss.Color("#172235")).Render(label)
}

func (m rootShellModel) startNextValidation() (tea.Model, tea.Cmd) {
	if len(m.validationQueue) == 0 {
		m.validateRunning = false
		m.validateDone = true
		m.currentValidation = ""
		if m.deploymentPhase == deploymentPhaseValidate {
			if m.validationHasFailures() {
				m.deploymentPhase = deploymentPhaseFailed
				m.message = "Deployment stopped because provider validation failed. Resolve the error and retry from Review."
				return m, nil
			}
			m.message = "Validation complete. Applying supported missing configuration..."
			return m.startDeploy()
		}
		m.message = "Validation complete. Review existing and missing configuration before deployment."
		return m, nil
	}
	provider := m.validationQueue[0]
	m.validationQueue = m.validationQueue[1:]
	m.currentValidation = provider
	m.validationProgress[provider] = 0.05
	m.message = "Validating " + providerLabel(provider) + " configuration..."
	root := m.packagePath
	ch := make(chan tea.Msg, 64)
	m.activityCh = ch
	go func() {
		report := lifecycle.Reporter(func(line string) { ch <- activityMsg{provider: provider, line: line} })
		item, err := lifecycle.ValidateProvider(root, provider, report)
		ch <- providerValidationFinishedMsg{provider: provider, item: item, err: err}
	}()
	return m, tea.Batch(m.spinner.Tick, validationProgressCmd(provider), waitForActivity(ch))
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
		deployItem := item
		ch := make(chan tea.Msg, 64)
		m.activityCh = ch
		go func() {
			report := lifecycle.Reporter(func(line string) { ch <- activityMsg{provider: deployItem.Provider, line: line} })
			result, err := lifecycle.DeployProvider(root, deployItem, report)
			ch <- providerDeployFinishedMsg{provider: deployItem.Provider, item: result, err: err}
		}()
		return m, tea.Batch(m.spinner.Tick, deploymentProgressCmd(item.Provider), waitForActivity(ch))
	}
	m.deployStarted = false
	m.deployDone = true
	m.deploymentPhase = deploymentPhaseComplete
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
	m.teardownScroll = 0
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
	).WithTheme(dispatchHuhTheme()).WithShowHelp(false).WithAccessible(m.accessibleForms).WithWidth(76)
	m.message = "Type the package name to confirm the reset."
	return m, m.formCommand(m.teardownConfirmForm, accessibleTeardownForm)
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
	m.activityLog, m.activityExpanded = nil, false
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
		ch := make(chan tea.Msg, 64)
		m.activityCh = ch
		go func() {
			report := lifecycle.Reporter(func(line string) { ch <- activityMsg{provider: provider, line: line} })
			result, err := lifecycle.DestroyProvider(root, provider, resources, report)
			ch <- providerTeardownFinishedMsg{provider: provider, result: result, err: err}
		}()
		return m, tea.Batch(m.spinner.Tick, teardownProgressCmd(provider), waitForActivity(ch))
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

func (m rootShellModel) formCommand(form *huh.Form, kind accessibleFormKind) tea.Cmd {
	if !m.accessibleForms {
		return form.Init()
	}
	return tea.Exec(&accessibleFormCommand{form: form}, func(err error) tea.Msg {
		return accessibleFormFinishedMsg{kind: kind, err: err}
	})
}

func (m rootShellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.route(msg)
}

func (m rootShellModel) route(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.directoryInput.Width = min(max(msg.Width-34, 30), 72)
		m.packageInput.Width = min(max(msg.Width-34, 30), 56)
		m.directoryPicker.Height = min(max(msg.Height-18, 5), 16)
		m.followLifecycleTail()
		return m, nil
	case spinner.TickMsg:
		if !m.spinnerActive() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case activityMsg:
		m.activityLog = append(m.activityLog, msg.line)
		if len(m.activityLog) > activityLogCap {
			m.activityLog = m.activityLog[len(m.activityLog)-activityLogCap:]
		}
		// The activity feed already shows the latest step; don't also push it into
		// m.message, which the footer renders — that duplicated the line at the
		// bottom of the screen.
		m.followLifecycleTail()
		return m, waitForActivity(m.activityCh)
	case accessibleFormFinishedMsg:
		if msg.err != nil {
			m.message = "Accessible form stopped: " + msg.err.Error()
			return m, nil
		}
		switch msg.kind {
		case accessibleChooseForm:
			if err := m.syncComponentAnswers(); err != nil {
				m.message = err.Error()
				m.rebuildComponentForm()
				return m, m.formCommand(m.componentForm, accessibleChooseForm)
			}
			m.selectTemplate()
			m.screen, m.cursor = screenDashboard, 0
			return m.beginChecks(m.selectedProviders())
		case accessibleTemplateForm:
			return m.selectTemplateAndConfigure()
		case accessibleTeardownForm:
			m.confirmingTeardown = false
			return m.startTeardown()
		case accessibleBoxCCGForm:
			return m.saveBoxCCG()
		}
		return m, nil
	case checkFinishedMsg:
		if msg.err != nil {
			m.statuses[msg.provider] = connectionFailed
			m.message = msg.err.Error()
		} else {
			m.results[msg.provider] = msg.result
			if strings.TrimSpace(msg.result.Diagnostic) != "" {
				m.providerDiagnostics[msg.provider] = msg.result.Diagnostic
			}
			if msg.result.ConnectivityOK {
				if msg.provider == "salesforce" {
					profile := firstNonEmptyString(msg.result.Discovery.Profile, msg.result.Discovery.Identity)
					if profile != "" {
						m.saveSalesforceDiscovery(msg.result.Discovery)
						_ = os.Setenv("SF_ALIAS", profile)
					}
					delete(m.providerDiagnostics, "salesforce")
				}
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
			return m, nil
		}
		m.message = "Authentication finished. Verifying connection..."
		return m.beginChecks([]string{msg.provider})
	case postDeployOpenFinishedMsg:
		if msg.err != nil {
			m.message = "Unable to open " + msg.label + ": " + msg.err.Error()
		} else {
			m.message = "Opened " + msg.label + "."
		}
		return m, nil
	case scratchOrgFinishedMsg:
		m.salesforceCreating = false
		if msg.err != nil {
			m.statuses["salesforce"] = connectionFailed
			m.message = "Scratch-org creation failed. Review the guidance above or press d for the full Salesforce CLI error."
			result := m.results["salesforce"]
			result.Name = "salesforce"
			result.Checks = []string{msg.err.Error()}
			result.RequiresAttention = true
			if failure, ok := msg.err.(*salesforceorg.Failure); ok {
				m.providerDiagnostics["salesforce"] = failure.Diagnostic
				result.Diagnostic = failure.Diagnostic
			}
			m.results["salesforce"] = result
			return m, nil
		}
		m.saveSalesforceTarget(msg.alias, msg.info)
		_ = os.Setenv("SF_ALIAS", msg.alias)
		delete(m.providerDiagnostics, "salesforce")
		m.message = "Scratch org created. Verifying status and expiration..."
		return m.beginChecks([]string{"salesforce"})
	case devHubListFinishedMsg:
		if msg.err != nil {
			result := m.results["salesforce"]
			result.Name = "salesforce"
			result.Checks = []string{msg.err.Error()}
			result.RequiresAttention = true
			if failure, ok := msg.err.(*salesforceorg.Failure); ok {
				result.Diagnostic = failure.Diagnostic
				m.providerDiagnostics["salesforce"] = failure.Diagnostic
			}
			m.results["salesforce"] = result
			m.message = "Dev Hub discovery failed. Review the guidance above or press d for the full Salesforce CLI error."
			return m, nil
		}
		m.devHubs = msg.hubs
		if len(m.devHubs) == 0 {
			m.message = "No authenticated Dev Hub found. Opening Salesforce login..."
			cmd := exec.Command("sf", "org", "login", "web", "--set-default-dev-hub", "--alias", "box-dispatch-devhub")
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return devHubLoginFinishedMsg{err: err} })
		}
		m.cursor = preferredDevHubCursor(m.devHubs)
		m.screen = screenDevHubs
		m.message = "Choose which authenticated Dev Hub should create the scratch org."
		return m, nil
	case devHubLoginFinishedMsg:
		if msg.err != nil {
			m.message = "Dev Hub authentication failed: " + msg.err.Error()
			return m, nil
		}
		m.message = "Dev Hub authentication finished. Discovering available Dev Hubs..."
		return m, tea.Batch(m.spinner.Tick, listDevHubsCmd())
	case packageFinishedMsg:
		m.packageStarted = false
		if msg.err != nil {
			m.deploymentPhase = deploymentPhaseFailed
			m.message = "Package failed: " + msg.err.Error()
			return m, nil
		}
		m.packageDone = true
		m.packagePath = msg.manifest.Destination
		m.message = "Package created. Validating provider configuration and prerequisites..."
		m.savePlan()
		if m.screen == screenDeploy && m.deploymentPhase == deploymentPhasePackage {
			m.deploymentPhase = deploymentPhaseValidate
			return m.startValidation()
		}
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
		next, cmd := m.startNextValidation()
		result := next.(rootShellModel)
		result.followLifecycleTail()
		return result, cmd
	case providerValidationProgressMsg:
		if m.validateRunning && m.currentValidation == msg.provider && m.validationProgress[msg.provider] < 0.92 {
			m.validationProgress[msg.provider] = math.Min(m.validationProgress[msg.provider]+0.035, 0.92)
			m.followLifecycleTail()
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
		next, cmd := m.startNextDeployment()
		result := next.(rootShellModel)
		result.followLifecycleTail()
		return result, cmd
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
	if m.screen == screenDeploy && m.confirmingDeploy {
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
				m.screen = screenPackage
				m.message = "Deployment cancelled. No provider configuration was changed."
				return m, nil
			case "down", "j":
				m.scrollLifecycleResults(1)
				return m, nil
			case "up", "k":
				m.scrollLifecycleResults(-1)
				return m, nil
			case "left", "right", "tab", "h", "l":
				m.deployConfirmCursor = 1 - m.deployConfirmCursor
				return m, nil
			case "enter", " ", "spacebar":
				m.confirmingDeploy = false
				if m.deployConfirmCursor == 0 {
					if m.deploymentPhase == deploymentPhaseReview {
						return m.startDeploymentPipeline()
					}
					return m.startDeploy()
				}
				m.screen = screenPackage
				m.message = "Deployment cancelled. No provider configuration was changed."
				return m, nil
			}
		}
		return m, nil
	}
	if m.screen == screenDevHubs {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "left", "q":
				m.screen, m.cursor = screenProvider, 0
				m.message = "Scratch-org creation cancelled."
				return m, nil
			case "up", "down", "j", "k", "tab":
				m.moveCursor(key, len(m.devHubs))
				return m, nil
			case "enter", "right", " ", "spacebar":
				if len(m.devHubs) == 0 || m.cursor >= len(m.devHubs) {
					return m, nil
				}
				m.selectedDevHub = m.devHubs[m.cursor]
				settings, _ := shellstate.LoadConnectionSettings()
				settings.SalesforceDevHubAlias = m.selectedDevHub.Target()
				_ = shellstate.SaveConnectionSettings(settings)
				m.pendingScratchAlias = scratchOrgAlias(time.Now())
				m.scratchConfirmCursor = 1
				m.screen = screenScratchConfirm
				m.message = "Review the selected Dev Hub and scratch-org allocation."
				return m, nil
			}
		}
		return m, nil
	}
	if m.screen == screenScratchConfirm {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "q":
				m.screen, m.cursor = screenProvider, 0
				m.message = "Scratch-org creation cancelled."
				return m, nil
			case "left", "right", "tab", "h", "l":
				m.scratchConfirmCursor = 1 - m.scratchConfirmCursor
				return m, nil
			case "enter", " ", "spacebar":
				if m.scratchConfirmCursor != 0 {
					m.screen, m.cursor = screenProvider, 0
					m.message = "Scratch-org creation cancelled."
					return m, nil
				}
				alias := m.pendingScratchAlias
				m.screen, m.cursor = screenProvider, 0
				m.salesforceCreating = true
				m.statuses["salesforce"] = connectionChecking
				devHub := m.selectedDevHub.Target()
				m.message = "Creating a 30-day Developer scratch org using Dev Hub " + devHub + "..."
				return m, tea.Batch(m.spinner.Tick, createScratchOrgCmd(alias, devHub))
			}
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.screen == screenHelp {
		switch key.String() {
		case "?", "f1", "esc", "left", "q":
			m.screen = m.helpReturn
			return m, nil
		case "down", "j", "pgdown":
			delta := 1
			if key.String() == "pgdown" {
				delta = m.helpVisibleRows()
			}
			m.helpScroll = clampLifecycleScroll(m.helpScroll, delta, m.helpScrollLimit())
			return m, nil
		case "up", "k", "pgup":
			delta := -1
			if key.String() == "pgup" {
				delta = -m.helpVisibleRows()
			}
			m.helpScroll = clampLifecycleScroll(m.helpScroll, delta, m.helpScrollLimit())
			return m, nil
		}
		return m, nil
	}
	if (key.String() == "?" || key.String() == "f1") && !(m.screen == screenConfig && m.configFocus < 2) {
		m.helpReturn = m.screen
		m.helpScroll = 0
		m.screen = screenHelp
		return m, nil
	}
	if m.screen == screenDiagnostic {
		switch key.String() {
		case "esc", "left", "d":
			m.screen = m.diagnosticReturn
			m.diagnosticScroll = 0
			return m, nil
		case "down", "j":
			m.scrollDiagnostic(1)
			return m, nil
		case "up", "k":
			m.scrollDiagnostic(-1)
			return m, nil
		case "pgdown":
			m.scrollDiagnostic(max(m.height-16, 5))
			return m, nil
		case "pgup":
			m.scrollDiagnostic(-max(m.height-16, 5))
			return m, nil
		}
	}
	if key.String() == "d" && (m.screen == screenProvider || m.screen == screenValidate || m.screen == screenDeploy) {
		if m.openDiagnostic() {
			return m, nil
		}
	}
	// While a long task runs, `e` expands/collapses the live activity feed.
	if key.String() == "e" && m.taskRunning() && len(m.activityLog) > 0 {
		m.activityExpanded = !m.activityExpanded
		return m, nil
	}
	if key.String() == "q" && !(m.screen == screenConfig && m.configFocus < 2) {
		if m.screen == screenWelcome {
			return m, tea.Quit
		}
		m.screen, m.cursor, m.message = screenWelcome, 0, ""
		return m, nil
	}
	// Spacebar activates the highlighted row on the plain list screens, matching
	// the Huh-driven screens where space selects — so arrows navigate and
	// space/enter both select, everywhere. Screens that give space its own
	// meaning (component toggle, folder choose, strategy cycle) are excluded and
	// handle it themselves below.
	if key.String() == " " || key.String() == "spacebar" {
		switch m.screen {
		case screenWelcome, screenHistory, screenDashboard, screenProvider, screenOptions, screenBoxSwitch:
			key = tea.KeyMsg{Type: tea.KeyEnter}
		}
	}
	switch m.screen {
	case screenWelcome:
		m.moveCursor(key, len(welcomeOptions))
		if key.String() == "enter" || key.String() == "right" {
			if m.cursor == 0 {
				m.screen = screenComponents
				m.message = ""
				return m, m.formCommand(m.componentForm, accessibleChooseForm)
			}
			history, err := deploymentaudit.ListDeployments()
			m.deploymentHistory = history
			m.historyError = ""
			if err != nil {
				m.historyError = err.Error()
			} else if len(history) == 0 {
				m.historyError = "No deployment has been recorded yet."
			}
			m.screen, m.cursor = screenHistory, 0
			m.message = "Choose a deployment to review its recorded resources or reset it."
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
		switch key.String() {
		case "down", "j":
			total := len(m.teardownBodyRows(m.width))
			maxScroll := total - m.teardownVisibleRows()
			if m.teardownScroll < maxScroll {
				m.teardownScroll++
			}
			return m, nil
		case "up", "k":
			if m.teardownScroll > 0 {
				m.teardownScroll--
			}
			return m, nil
		}
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
		if key.String() == "left" || key.String() == "esc" {
			m.screen, m.cursor = screenWelcome, 0
			return m, nil
		}
		if key.String() == "tab" || key.String() == "shift+tab" {
			if m.componentForm.GetFocusedField().GetKey() == "template" {
				return m, m.componentForm.NextField()
			}
			return m, m.componentForm.PrevField()
		}
		if key.String() == "ctrl+a" && m.componentForm.GetFocusedField().GetKey() == "components" {
			m.message = "Choose available providers individually; Databricks and AWS Bedrock AgentCore are coming soon."
			return m, nil
		}
		if key.String() == " " || key.String() == "spacebar" || key.String() == "x" {
			if component, unavailable := m.hoveredComingSoonComponent(); unavailable {
				m.message = component.name + " is coming soon and cannot be selected yet."
				return m, nil
			}
			if selected, cmd := m.selectHoveredQuickstart(); selected {
				return m, cmd
			}
		}
		if key.String() == "right" {
			if err := m.syncComponentAnswers(); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.selectTemplate()
			m.screen, m.cursor = screenDashboard, 0
			return m.beginChecks(m.selectedProviders())
		}
		form, cmd := m.componentForm.Update(msg)
		m.componentForm = form.(*huh.Form)
		if m.componentForm.State == huh.StateCompleted {
			if err := m.syncComponentAnswers(); err != nil {
				m.message = err.Error()
				m.rebuildComponentForm()
				return m, m.formCommand(m.componentForm, accessibleChooseForm)
			}
			m.selectTemplate()
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
			m.message = ""
			return m, m.formCommand(m.componentForm, accessibleChooseForm)
		}
		if key.String() == "right" {
			if m.allSelectedConnected() {
				return m.selectTemplateAndConfigure()
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
				m.rebuildComponentForm()
				m.screen, m.cursor = screenComponents, 0
				m.message = ""
				return m, m.formCommand(m.componentForm, accessibleChooseForm)
			} else if m.allSelectedConnected() {
				return m.selectTemplateAndConfigure()
			} else {
				m.message = "Connect every selected service before continuing."
			}
		}
	case screenProvider:
		if key.String() == "r" && m.provider == "salesforce" {
			return m.beginChecks([]string{"salesforce"})
		}
		m.moveCursor(key, len(m.providerActions()))
		if key.String() == "left" {
			m.screen, m.cursor = screenDashboard, 0
			return m, nil
		}
		if key.String() == "enter" || key.String() == "right" {
			return m.runProviderAction(m.cursor)
		}
	case screenBoxSwitch:
		switch key.String() {
		case "up", "down", "k", "j", "tab":
			m.boxRemovePending = ""
			m.moveCursor(key, len(m.boxConnections))
			return m, nil
		case "left", "esc":
			m.boxRemovePending = ""
			m.screen, m.cursor = screenProvider, 0
			m.message = ""
			return m, nil
		case "enter", "right":
			m.boxRemovePending = ""
			if m.cursor < len(m.boxConnections) {
				return m.setBoxDefault(m.boxConnections[m.cursor])
			}
		case "x", "delete", "backspace":
			if m.cursor < len(m.boxConnections) {
				return m.removeBoxConnection(m.boxConnections[m.cursor])
			}
		}
	case screenOptions:
		options := m.providerOptions()
		m.moveCursor(key, len(options))
		m.optionCursor = m.cursor
		if (key.String() == "enter" || key.String() == "right") && len(options) > 0 {
			selected := options[m.cursor]
			if m.provider == "salesforce" && selected == salesforceLoginOption {
				m.screen, m.cursor = screenProvider, 0
				m.message = "Opening Salesforce login..."
				cmd := exec.Command("sf", "org", "login", "web", "--set-default")
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return externalFinishedMsg{provider: "salesforce", err: err}
				})
			}
			m.saveProviderOption(m.provider, selected)
			m.screen, m.cursor = screenProvider, 0
			if m.provider == "salesforce" {
				m.message = "Salesforce org selected. Verifying connection..."
				return m.beginChecks([]string{"salesforce"})
			}
			m.message = "Selection saved. Choose Check connection when ready."
		}
		if key.String() == "left" {
			m.screen, m.cursor = screenProvider, 0
		}
	case screenConfig:
		if key.String() == "esc" || (key.String() == "left" && m.configFocus == 4) {
			m.screen, m.cursor = screenDashboard, 0
			return m, nil
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
				return m.prepareReview()
			}
		case "right":
			if m.configFocus == 2 {
				m.cycleDeploymentStrategy(1)
				return m, nil
			}
			if m.configFocus == 4 {
				return m.prepareReview()
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
		if key.String() == "right" || key.String() == "enter" {
			m.screen = screenDeploy
			m.deploymentPhase = deploymentPhaseReview
			return m.requestDeployConfirmation()
		}
	case screenValidate:
		if key.String() == "down" || key.String() == "j" {
			m.scrollLifecycleResults(1)
			return m, nil
		}
		if key.String() == "up" || key.String() == "k" {
			m.scrollLifecycleResults(-1)
			return m, nil
		}
		if key.String() == "left" {
			m.screen = screenPackage
			return m, nil
		}
		if key.String() == "right" || (key.String() == "enter" && m.validateDone) {
			if !m.validateDone {
				m.message = "Complete validation before deployment."
				return m, nil
			}
			if m.validationHasFailures() {
				m.message = "Deployment is blocked because provider validation failed. Resolve the error and press r to validate again."
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
		if key.String() == "v" && !m.confirmingDeploy && m.deploymentPhase != deploymentPhaseReview && m.deploymentPhase != deploymentPhasePackage {
			m.deployShowDetails = !m.deployShowDetails
			m.lifecycleScroll = 0
			m.deployAssetsScroll = 0
			if m.deployShowDetails {
				m.message = "Showing detailed provider results and deployed resources."
			} else {
				m.message = "Showing the focused deployment summary."
			}
			return m, nil
		}
		if m.deployShowDetails && !m.deployDone && (key.String() == "down" || key.String() == "j") {
			m.scrollLifecycleResults(1)
			return m, nil
		}
		if m.deployShowDetails && !m.deployDone && (key.String() == "up" || key.String() == "k") {
			m.scrollLifecycleResults(-1)
			return m, nil
		}
		if m.deployDone {
			switch key.String() {
			case "down", "j":
				if m.deployShowDetails {
					total := len(m.deployedAssets())
					if maxScroll := total - m.deployTableCapacity(); m.deployAssetsScroll < maxScroll {
						m.deployAssetsScroll++
					}
				}
				return m, nil
			case "up", "k":
				if m.deployShowDetails && m.deployAssetsScroll > 0 {
					m.deployAssetsScroll--
				}
				return m, nil
			case "b":
				return m.openPostDeployBox()
			case "s":
				return m.openPostDeploySalesforce()
			}
		}
		if key.String() == "left" {
			if !m.packageStarted && !m.validateRunning && !m.deployStarted {
				m.screen = screenPackage
				m.deploymentPhase = deploymentPhaseReview
			}
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
		if (key.String() == "enter" || key.String() == "right") && !m.deployStarted && m.deploymentPhase == deploymentPhaseReview {
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

// hoveredComingSoonComponent reports whether the highlighted provider option is
// a read-only roadmap item. Huh does not currently expose disabled multi-select
// options, so Dispatch intercepts the toggle before the form mutates its value.
func (m rootShellModel) hoveredComingSoonComponent() (componentChoice, bool) {
	if m.componentForm == nil {
		return componentChoice{}, false
	}
	field, ok := m.componentForm.GetFocusedField().(*huh.MultiSelect[string])
	if !ok || field.GetKey() != "components" {
		return componentChoice{}, false
	}
	provider, ok := field.Hovered()
	if !ok {
		return componentChoice{}, false
	}
	for _, component := range m.components {
		if component.provider == provider && component.comingSoon {
			return component, true
		}
	}
	return componentChoice{}, false
}

// selectHoveredQuickstart gives the quickstart list radio-button semantics:
// arrows move the cursor, while Space commits exactly one selection. Rebuilding
// the form updates the visible checkmark without moving the cursor.
func (m *rootShellModel) selectHoveredQuickstart() (bool, tea.Cmd) {
	if m.componentForm == nil {
		return false, nil
	}
	field, ok := m.componentForm.GetFocusedField().(*huh.MultiSelect[string])
	if !ok || field.GetKey() != "template" {
		return false, nil
	}
	hovered, ok := field.Hovered()
	if !ok {
		return true, nil
	}
	m.answers.templateID = hovered
	m.rebuildComponentForm()
	cmds := []tea.Cmd{m.componentForm.Init()}
	form, cmd := m.componentForm.Update(tea.KeyMsg{Type: tea.KeyHome})
	m.componentForm = form.(*huh.Form)
	cmds = append(cmds, cmd)
	for _, template := range m.templates {
		if template.id == hovered {
			break
		}
		form, cmd := m.componentForm.Update(tea.KeyMsg{Type: tea.KeyDown})
		m.componentForm = form.(*huh.Form)
		cmds = append(cmds, cmd)
	}
	return true, tea.Batch(cmds...)
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
	case "salesforce-done":
		m.screen = screenDashboard
		m.cursor = len(m.selectedProviders()) + 2
		m.message = "Salesforce org ready. Continue when every selected service is connected."
		return m, nil
	case "salesforce-existing":
		m.screen, m.cursor = screenOptions, 0
		return m, nil
	case "check":
		return m.beginChecks([]string{m.provider})
	case "choose":
		m.screen, m.cursor = screenOptions, 0
		return m, nil
	case "ccg":
		return m.openBoxCCGForm()
	case "switch":
		return m.openBoxSwitch()
	case "scratch":
		m.message = "Discovering authenticated Salesforce Dev Hubs..."
		return m, tea.Batch(m.spinner.Tick, listDevHubsCmd())
	case "connect":
		if m.provider == "databricks" {
			m.screen = screenDatabricksHost
			m.hostInput.Focus()
			return m, textinput.Blink
		}
		parts := providerCLIConnectCommand(m.provider)
		cmd := exec.Command(parts[0], parts[1:]...)
		provider := m.provider
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return externalFinishedMsg{provider: provider, err: err} })
	case "back":
		m.screen, m.cursor = screenDashboard, 0
	}
	return m, nil
}

func providerCLIConnectCommand(provider string) []string {
	commands := map[string][]string{
		"box":        {"box", "login"},
		"salesforce": {"sf", "org", "login", "web", "--set-default"},
		"aws":        {"aws", "configure", "sso"},
	}
	return commands[provider]
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
	if m.provider == "salesforce" {
		if m.statuses["salesforce"] == connectionConnected {
			return []string{"salesforce-done", "salesforce-existing", "scratch"}
		}
		return []string{"salesforce-existing", "scratch", "back"}
	}
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

const salesforceLoginOption = "Sign in to another Salesforce org..."

func (m rootShellModel) providerOptions() []string {
	options := append([]string(nil), m.results[m.provider].Discovery.Options...)
	if m.provider == "salesforce" {
		options = append(options, salesforceLoginOption)
	}
	return options
}

// openBoxCCGForm captures a Box Client Credentials Grant app and its subject, so
// box-dispatch can mint a token that carries the enterprise app's scopes (the
// CLI's OAuth token lacks some, e.g. Doc Gen).
func (m rootShellModel) openBoxCCGForm() (tea.Model, tea.Cmd) {
	settings, _ := shellstate.LoadConnectionSettings()
	subjectType := settings.BoxCCGSubjectType
	if subjectType == "" {
		subjectType = "user"
	}
	// Heap-allocated so the form's writes survive bubbletea's model copies.
	m.ccgClientID = &settings.BoxCCGClientID
	m.ccgClientSecret = &settings.BoxCCGClientSecret
	m.ccgSubjectType = &subjectType
	m.ccgSubjectID = &settings.BoxCCGSubjectID
	m.boxCCGForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Client ID").Value(m.ccgClientID).Validate(requiredField("Client ID")),
			huh.NewInput().Title("Client Secret").Password(true).Value(m.ccgClientSecret).Validate(requiredField("Client Secret")),
			huh.NewSelect[string]().Title("Subject type").
				Options(
					huh.NewOption("user — created resources owned by the user", "user"),
					huh.NewOption("enterprise — acts as the service account", "enterprise"),
				).Value(m.ccgSubjectType),
			huh.NewInput().Title("Subject ID").
				Description("Box user ID when subject is user, enterprise ID when enterprise").
				Value(m.ccgSubjectID).Validate(requiredField("Subject ID")),
		),
	).WithTheme(dispatchHuhTheme()).WithShowHelp(true).WithAccessible(m.accessibleForms).WithWidth(76)
	m.screen = screenBoxCCG
	m.message = "Enter the CCG app credentials. Esc cancels."
	return m, m.formCommand(m.boxCCGForm, accessibleBoxCCGForm)
}

func requiredField(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

// saveBoxCCG persists the captured credentials and makes the new CCG connection
// the default, then returns to the Box provider screen. Values are read through
// the heap pointers the form wrote to (see the model field comment). The record
// is written 0600, but the secret is on-disk plaintext.
func (m rootShellModel) saveBoxCCG() (tea.Model, tea.Cmd) {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	subjectType := deref(m.ccgSubjectType)
	if subjectType == "" {
		subjectType = "user"
	}
	settings, _ := shellstate.LoadConnectionSettings()
	settings.BoxCCGClientID = deref(m.ccgClientID)
	settings.BoxCCGClientSecret = deref(m.ccgClientSecret)
	settings.BoxCCGSubjectType = subjectType
	settings.BoxCCGSubjectID = deref(m.ccgSubjectID)
	// A freshly captured connection becomes the default box-dispatch deploys with.
	settings.BoxDefaultConnection = boxconn.DispatchCCGName
	if err := shellstate.SaveConnectionSettings(settings); err != nil {
		m.message = "Could not save Box CCG credentials: " + err.Error()
	} else {
		m.message = "Box CCG credentials saved and set as the default. Choose Check connection to verify."
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

// removeBoxConnection deletes a connection, guarded by a confirming second
// keypress since removing a CLI environment is a global, sign-in-losing change.
func (m rootShellModel) removeBoxConnection(conn boxconn.Connection) (tea.Model, tea.Cmd) {
	if m.boxRemovePending != conn.Name {
		m.boxRemovePending = conn.Name
		what := "the box CLI environment " + conn.Name + " (you would need to sign in again to restore it)"
		if conn.Source == boxconn.SourceDispatch {
			what = "the box-dispatch CCG credentials"
		}
		m.message = "Press x again to remove " + what + ". Any other key cancels."
		return m, nil
	}
	m.boxRemovePending = ""
	if err := boxconn.Remove(conn); err != nil {
		m.message = "Could not remove " + conn.Name + ": " + err.Error()
		return m, nil
	}
	m.boxConnections = boxconn.List()
	if m.cursor >= len(m.boxConnections) && m.cursor > 0 {
		m.cursor = len(m.boxConnections) - 1
	}
	m.message = "Removed " + conn.Name + "."
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

func (m rootShellModel) saveProviderOption(provider, value string) {
	settings, _ := shellstate.LoadConnectionSettings()
	switch provider {
	case "salesforce":
		settings.SalesforceAlias = value
	case "databricks":
		settings.DatabricksProfile = value
	case "aws":
		settings.AWSProfile = value
	}
	_ = shellstate.SaveConnectionSettings(settings)
}

func (m rootShellModel) saveSalesforceTarget(alias string, info salesforceorg.Info) {
	settings, _ := shellstate.LoadConnectionSettings()
	settings.SalesforceAlias = firstNonEmptyString(alias, info.Alias, info.Username)
	settings.SalesforceOrgID = info.ID
	settings.SalesforceOrgStatus = info.EffectiveStatus()
	settings.SalesforceExpirationDate = info.ExpirationDate
	settings.SalesforceOrgType = "persistent"
	if info.IsScratch() {
		settings.SalesforceOrgType = "scratch"
	}
	_ = shellstate.SaveConnectionSettings(settings)
}

func (m rootShellModel) saveSalesforceDiscovery(discovery checker.ProviderDiscovery) {
	settings, _ := shellstate.LoadConnectionSettings()
	settings.SalesforceAlias = firstNonEmptyString(discovery.Profile, discovery.Identity)
	settings.SalesforceOrgID = discovery.OrgID
	settings.SalesforceOrgStatus = discovery.OrgStatus
	settings.SalesforceExpirationDate = discovery.ExpiresAt
	settings.SalesforceOrgType = discovery.OrgType
	_ = shellstate.SaveConnectionSettings(settings)
}

func scratchOrgAlias(now time.Time) string {
	return "box-dispatch-" + now.UTC().Format("20060102-150405")
}

func listDevHubsCmd() tea.Cmd {
	return func() tea.Msg {
		hubs, err := salesforceorg.ListDevHubs()
		return devHubListFinishedMsg{hubs: hubs, err: err}
	}
}

func preferredDevHubCursor(hubs []salesforceorg.DevHub) int {
	settings, _ := shellstate.LoadConnectionSettings()
	for i, hub := range hubs {
		if hub.Target() == settings.SalesforceDevHubAlias {
			return i
		}
	}
	for i, hub := range hubs {
		if hub.Connected() {
			return i
		}
	}
	return 0
}

func createScratchOrgCmd(alias, devHub string) tea.Cmd {
	return func() tea.Msg {
		info, err := salesforceorg.CreateScratch(alias, devHub)
		return scratchOrgFinishedMsg{alias: alias, info: info, err: err}
	}
}

const boxAdminConsoleURL = "https://app.box.com/master"

func (m rootShellModel) openPostDeployBox() (tea.Model, tea.Cmd) {
	cmd := browserOpenCommand(runtime.GOOS, boxAdminConsoleURL)
	if cmd == nil {
		m.message = "Opening the Box enterprise is not supported on this operating system."
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return postDeployOpenFinishedMsg{label: "Box enterprise", err: err}
	})
}

func (m rootShellModel) openPostDeploySalesforce() (tea.Model, tea.Cmd) {
	alias, ok := m.postDeploySalesforceTarget()
	if !ok {
		m.message = "No selected Salesforce scratch org is available to open."
		return m, nil
	}
	cmd := exec.Command("sf", "org", "open", "--target-org", alias)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return postDeployOpenFinishedMsg{label: "Salesforce scratch org", err: err}
	})
}

func browserOpenCommand(goos, target string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", target)
	case "linux":
		return exec.Command("xdg-open", target)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		return nil
	}
}

func (m rootShellModel) postDeploySalesforceTarget() (string, bool) {
	if !slices.Contains(m.selectedProviders(), "salesforce") {
		return "", false
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err == nil && strings.TrimSpace(settings.SalesforceAlias) != "" && strings.EqualFold(settings.SalesforceOrgType, "scratch") {
		return settings.SalesforceAlias, true
	}
	discovery := m.results["salesforce"].Discovery
	alias := firstNonEmptyString(discovery.Profile, discovery.Identity)
	return alias, alias != "" && strings.EqualFold(discovery.OrgType, "scratch")
}

func (m rootShellModel) postDeployBoxEnterpriseSuffix() string {
	enterpriseID := strings.TrimSpace(m.results["box"].Discovery.Enterprise)
	if enterpriseID == "" {
		return ""
	}
	return " (EID " + enterpriseID + ")"
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
		if item.selected && !item.comingSoon {
			providers = append(providers, item.provider)
		}
	}
	return providers
}

func (m rootShellModel) selectedProviderLabels() []string {
	providers := m.selectedProviders()
	labels := make([]string, 0, len(providers))
	for _, provider := range providers {
		labels = append(labels, providerLabel(provider))
	}
	return labels
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
	case screenHelp:
		body = m.viewExpandedHelp(contentWidth)
	case screenDiagnostic:
		body = m.viewDiagnostic(contentWidth)
	case screenScratchConfirm:
		body = m.viewScratchConfirm(contentWidth)
	case screenDevHubs:
		body = m.viewDevHubs(contentWidth)
	}
	return lipgloss.NewStyle().Margin(1, 3).Width(contentWidth).Render(m.header(contentWidth) + "\n\n" + body + "\n\n" + m.footer())
}

func (m rootShellModel) header(width int) string {
	// A filled brand chip reads as a small logo rather than loose glyphs.
	mark := lipgloss.NewStyle().Bold(true).Foreground(white).Background(cyan).Render(" B/ ")
	product := lipgloss.NewStyle().Bold(true).Foreground(white).Render("  DISPATCH")
	tag := lipgloss.NewStyle().Foreground(muted).Render("  SOLUTION ASSEMBLY")
	domain := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("UNOFFICIALBOX.DEV")
	used := lipgloss.Width(mark + product + tag + domain)
	gap := max(width-used, 2)
	row := mark + product + tag + strings.Repeat(" ", gap) + domain
	// A hairline rule frames the chrome and separates it from the content below.
	return lipgloss.NewStyle().Width(width).BorderStyle(lipgloss.NormalBorder()).BorderTop(false).BorderBottom(true).BorderLeft(false).BorderRight(false).BorderForeground(divider).Render(row)
}

func (m rootShellModel) expandedHelpLines(width int) []string {
	workflow := []string{
		"1  Choose     Select a quickstart and providers",
		"2  Connect    Verify the selected services",
		"3  Configure  Set destination, strategy, and capabilities",
		"4  Review     Preview the complete plan without changing anything",
		"5  Deploy     Assemble, validate, install prerequisites, and apply",
	}
	controls := []string{
		"↑/↓ or j/k   Move through rows and bounded result lists",
		"Enter/Space  Select, toggle, or confirm the focused action",
		"← or Esc     Return to the previous screen",
		"d            Open sanitized full diagnostics when available",
		"e            Expand or collapse live task activity",
		"q            Return home; from Home, quit",
		"? or F1      Open or close this help",
	}
	presentation := "Set metadata.accessibleForms = true in .dispatch/ui-settings.bcl to enable Huh's screen-reader form prompts. Set NO_COLOR, TERM=dumb, or pass --no-color to disable ANSI color. TERM=dumb uses the non-full-screen command output."
	lines := []string{titleStyle.Render("WORKFLOW")}
	lines = append(lines, workflow...)
	lines = append(lines, "", titleStyle.Render("CONTROLS"))
	lines = append(lines, controls...)
	lines = append(lines, "", titleStyle.Render("ACCESSIBLE OUTPUT"))
	lines = append(lines, strings.Split(dimStyle.Copy().Width(max(width-10, 20)).Render(presentation), "\n")...)
	return lines
}

func (m rootShellModel) helpVisibleRows() int {
	return max(m.height-18, 4)
}

func (m rootShellModel) helpScrollLimit() int {
	width := min(max(m.width-8, 64), 112)
	return lifecycleScrollLimit(len(m.expandedHelpLines(width)), m.helpVisibleRows())
}

func (m rootShellModel) viewExpandedHelp(width int) string {
	body := renderLifecycleViewport(m.expandedHelpLines(width), m.helpVisibleRows(), m.helpScroll)
	return titleStyle.Render("Keyboard and accessibility help") + "\n" +
		dimStyle.Render("The footer stays contextual; this view contains the complete interaction model.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(body)
}

// taskRunning reports whether a long-running lifecycle task is in progress, so
// the activity feed and its expand key are only active while there is work.
func (m rootShellModel) taskRunning() bool {
	return m.packageStarted || m.validateRunning || m.deployStarted || m.teardownStarted
}

// spinnerActive prevents a completed validation or deployment from scheduling
// an endless chain of repaint ticks. The old unconditional TickMsg handler made
// static checklists look as though they were still scrolling or looping.
func (m rootShellModel) spinnerActive() bool {
	if m.packageStarted || m.validateRunning || m.deployStarted || m.teardownStarted || m.salesforceCreating {
		return true
	}
	for _, status := range m.statuses {
		if status == connectionChecking {
			return true
		}
	}
	return false
}

// renderActivity draws the live "still working" feed: a spinner header and, by
// default, the last two step lines (the most recent brightened) — like the
// collapsed thinking view in Claude/Codex/Cursor. Pressing `e` expands it to the
// recent history. It renders nothing until the running task emits its first step.
func (m rootShellModel) renderActivity(width int) string {
	if len(m.activityLog) == 0 {
		return ""
	}
	total := len(m.activityLog)
	header := lipgloss.NewStyle().Bold(true).Foreground(gold).Render(m.spinner.View() + " Working")
	line := func(text string, recent bool) string {
		style := dimStyle
		if recent {
			style = lipgloss.NewStyle().Foreground(ice)
		}
		return style.Render("  " + truncateCell(text, max(width-4, 20)))
	}
	if m.activityExpanded {
		shown := m.activityLog
		const window = 12
		if len(shown) > window {
			shown = shown[len(shown)-window:]
		}
		body := make([]string, len(shown))
		for i, l := range shown {
			body[i] = line(l, i == len(shown)-1)
		}
		hint := dimStyle.Render(fmt.Sprintf("  (%d steps · e to collapse)", total))
		return header + "\n" + strings.Join(body, "\n") + "\n" + hint
	}
	shown := m.activityLog
	if len(shown) > 2 {
		shown = shown[len(shown)-2:]
	}
	body := make([]string, len(shown))
	for i, l := range shown {
		body[i] = line(l, i == len(shown)-1)
	}
	hint := dimStyle.Render(fmt.Sprintf("  e to expand · %d steps", total))
	return header + "\n" + strings.Join(body, "\n") + "\n" + hint
}

func (m rootShellModel) stepper() string {
	current := 0
	switch m.screen {
	case screenComponents, screenTemplates:
		current = 0
	case screenDashboard, screenProvider, screenOptions, screenDatabricksHost, screenScratchConfirm, screenDevHubs:
		current = 1
	case screenConfig, screenDirectoryPicker, screenBoxComponents:
		current = 2
	case screenPackage:
		current = 3
	case screenValidate, screenDeploy:
		current = 4
	default:
		current = 1
	}
	labels := []string{"CHOOSE", "CONNECT", "CONFIGURE", "REVIEW", "DEPLOY"}
	// Interleave each step with a connector so the row reads as a single track:
	// the traveled path (up to the current step) is green, the road ahead muted.
	out := make([]string, 0, len(labels)*2-1)
	for i, label := range labels {
		if i > 0 {
			seg := muted
			if i <= current {
				seg = green
			}
			out = append(out, lipgloss.NewStyle().Foreground(seg).Render(" ── "))
		}
		color, icon := muted, "○"
		if i < current {
			color, icon = green, "●"
		} else if i == current {
			color, icon = coral, "◆"
		}
		out = append(out, lipgloss.NewStyle().Bold(i == current).Foreground(color).Render(icon+" "+label))
	}
	return strings.Join(out, "")
}

func (m rootShellModel) viewWelcome(width int) string {
	inner := width - 16 // panel border (2) + horizontal padding (2*5), rounded up

	accentMark := lipgloss.NewStyle().Bold(true).Foreground(coral).Render(" »")
	eyebrow := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("COMMUNITY-BUILT · OPEN SOURCE · PUNK ROCK 🤘")
	description := lipgloss.NewStyle().Foreground(ice).Width(min(inner, 66)).Render("Open tools for the builders extending Box — from composable building blocks to industry solution accelerators.")
	tags := lipgloss.NewStyle().Bold(true).Foreground(muted).Render("BOX   ·   SALESFORCE   ·   DATABRICKS   ·   AWS BEDROCK AGENTCORE")

	options := welcomeOptions
	optionRows := make([]string, len(options))
	rowWidth := min(inner, 48)
	for index, option := range options {
		// A coral left rail + faint surface marks the selection instead of a heavy
		// full-coral block; the rail keeps every row aligned.
		base := lipgloss.NewStyle().Width(rowWidth).Padding(0, 2).Border(lipgloss.NormalBorder(), false, false, false, true)
		if index == m.cursor {
			optionRows[index] = base.BorderForeground(coral).Background(lipgloss.Color("#14263F")).Foreground(white).Bold(true).Render("▸ " + option)
		} else {
			optionRows[index] = base.BorderForeground(divider).Foreground(ice).Render("  " + option)
		}
	}
	menu := strings.Join(optionRows, "\n")
	if m.height < 36 {
		headline := lipgloss.NewStyle().Bold(true).Foreground(ice).Render("BOX ") +
			lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("DISPATCH") + accentMark
		compactDescription := lipgloss.NewStyle().Foreground(ice).Width(min(width-4, 66)).Render("Assemble and deploy Box-backed solution accelerators.")
		compactRoute := dimStyle.Render("YOUR ROUTE") + "\n" + accent.Render("CHOOSE › CONNECT › CONFIGURE › REVIEW › DEPLOY")
		return lipgloss.JoinVertical(lipgloss.Left, eyebrow, headline, compactDescription, "", menu, "", compactRoute)
	}

	// The big wordmark (BOX kicker over the DISPATCH banner) needs a wide, tall
	// terminal; otherwise fall back to a one-line headline so the block art never
	// overruns the frame. Layout top-to-bottom: eyebrow, wordmark, the
	// description, then the menu. The tag line is anchored to the panel foot.
	var top string
	if inner >= 60 && m.height >= 42 {
		box := lipgloss.NewStyle().Bold(true).Foreground(ice).Render(strings.Join(boxBanner, "\n"))
		dispatch := lipgloss.NewStyle().Bold(true).Foreground(cyan).Render(strings.Join(dispatchBanner, "\n"))
		wordmark := lipgloss.JoinVertical(lipgloss.Left, box, dispatch)
		// A large coral chevron sits to the right of DISPATCH when there's room.
		if inner >= 72 {
			chev := lipgloss.NewStyle().Bold(true).Foreground(coral).Render(strings.Join(chevronBanner, "\n"))
			wordmark = lipgloss.JoinHorizontal(lipgloss.Bottom, wordmark, "  ", chev)
		}
		top = lipgloss.JoinVertical(lipgloss.Left, eyebrow, "", wordmark, "", description, "", menu)
	} else {
		headline := lipgloss.NewStyle().Bold(true).Foreground(ice).Render("BOX ") +
			lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("DISPATCH") + accentMark
		top = lipgloss.JoinVertical(lipgloss.Left, eyebrow, "", headline, "", description, "", menu)
	}
	bottom := tags

	th, bh := lipgloss.Height(top), lipgloss.Height(bottom)
	// Fill the panel to just under the terminal height (chrome around the hero is
	// ~17 rows); a floor keeps a gap even when there's little room.
	target := max(m.height-18, th+bh+2)
	spacer := max(target-th-bh, 1)
	heroContent := top + strings.Repeat("\n", spacer+1) + bottom

	hero := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#2E4B6E")).
		BorderLeftForeground(coral).
		Width(width-4).
		Padding(2, 5).
		Render(heroContent)

	return hero + "\n\n" + m.routeStrip()
}

// routeStrip renders the five-stage overview as numbered chips joined by chevrons
// — a clean flow where every step reads the same and only the destination (Deploy)
// is set apart in green.
func (m rootShellModel) routeStrip() string {
	steps := []struct {
		num, label string
		tone, text lipgloss.Color
	}{
		{"01", "CHOOSE", cyan, white},
		{"02", "CONNECT", cyan, white},
		{"03", "CONFIGURE", cyan, white},
		{"04", "REVIEW", cyan, white},
		{"05", "DEPLOY", green, navy},
	}
	label := lipgloss.NewStyle().Foreground(muted).Bold(true).Render("YOUR ROUTE")
	chevron := lipgloss.NewStyle().Foreground(divider).Render(" › ")
	parts := make([]string, 0, len(steps)*2)
	for i, s := range steps {
		if i > 0 {
			parts = append(parts, chevron)
		}
		chip := lipgloss.NewStyle().Bold(true).Foreground(s.text).Background(s.tone).Padding(0, 1).Render(s.num)
		parts = append(parts, chip+lipgloss.NewStyle().Foreground(ice).Render(" "+s.label))
	}
	return label + "\n" + strings.Join(parts, "")
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
	return titleStyle.Render("Deployment history") + "\n" + dimStyle.Render("Select a credential-free audit record to review its resources or reset that deployment.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n")) + "\n\n" +
		panel.Copy().BorderForeground(cyan).Width(width-4).Padding(1, 2).Render(detail)
}

func (m rootShellModel) viewComponents(width int) string {
	form := ""
	if m.componentForm != nil {
		form = m.componentForm.View()
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose a quickstart and providers") + "\n" + dimStyle.Render("Pick the solution architecture first, then add only the platforms it needs.") + "\n\n" + activePane.Copy().Width(width-4).Padding(1, 2).Render(form)
}

func (m rootShellModel) viewTemplates(width int) string {
	form := ""
	if m.templateForm != nil {
		form = m.templateForm.View()
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose an industry quickstart") + "\n" + dimStyle.Render("Start from a validated solution template.") + "\n\n" + activePane.Copy().Width(width-4).Padding(1, 2).Render(form)
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
		if summary := renderProviderConnectionSummary(m.results[provider]); summary != "" {
			line += "\n" + summary
		}
		style := lipgloss.NewStyle().Width(width-10).Padding(0, 1)
		if i == m.cursor {
			style = style.Copy().Bold(true).Foreground(white).Background(lipgloss.Color("#16355F"))
		}
		rows = append(rows, style.Render(line))
	}
	checkIndex := len(providers)
	reviseIndex := checkIndex + 1
	continueIndex := reviseIndex + 1
	checkStyle, reviseStyle := panel.Copy().Padding(0, 1).Width((width-7)/2), panel.Copy().Padding(0, 1).Width((width-7)/2)
	if m.cursor == checkIndex {
		checkStyle = activePane.Copy().Padding(0, 1).Width((width - 7) / 2)
	}
	if m.cursor == reviseIndex {
		reviseStyle = activePane.Copy().Padding(0, 1).Width((width - 7) / 2)
	}
	continueStyle := panel.Copy().Padding(0, 1).Width(width - 4).Height(2)
	continueText := dimStyle.Render("○  Continue to configuration  ·  Connect every selected service first")
	if m.allSelectedConnected() {
		continueText = lipgloss.NewStyle().Bold(true).Foreground(green).Render("◆  Continue to configuration  →")
	}
	if m.cursor == continueIndex {
		continueStyle = activePane.Copy().Padding(0, 1).Width(width - 4).Height(2)
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Connect selected services") + "\n" + dimStyle.Render("Confirm access for the providers selected with this quickstart.") + "\n\n" +
		accent.Render(fmt.Sprintf("%d/%d connected", connected, len(providers))) + "\n\n" +
		panel.Copy().Padding(0, 1).Width(width-4).Render(strings.Join(rows, "\n")) + "\n" +
		lipgloss.JoinHorizontal(lipgloss.Top,
			checkStyle.Render("◆  Recheck connections"), " ",
			reviseStyle.Render("↺  Change providers")) + "\n" +
		continueStyle.Render(continueText)
}

func providerConnectionSummary(result checker.ProviderResult) string {
	parts := make([]string, 0, 2)
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" && len(parts) < 2 {
			parts = append(parts, label+" "+value)
		}
	}
	switch result.Name {
	case "box":
		add("user", result.Discovery.Identity)
		add("enterprise", result.Discovery.Enterprise)
	case "salesforce":
		add("org", firstNonEmptyString(result.Discovery.Profile, result.Discovery.Identity))
		if result.Discovery.ExpiresAt != "" {
			add("expires", result.Discovery.ExpiresAt)
		} else {
			add("type", result.Discovery.OrgType)
		}
	case "databricks":
		add("profile", firstNonEmptyString(result.Discovery.Profile, result.Discovery.Identity))
		add("workspace", result.Discovery.Host)
	case "aws":
		add("profile", result.Discovery.Profile)
		add("region", result.Discovery.Region)
	}
	return strings.Join(parts, "\n")
}

func renderProviderConnectionSummary(result checker.ProviderResult) string {
	summary := providerConnectionSummary(result)
	if summary == "" {
		return ""
	}
	lines := strings.Split(summary, "\n")
	for i, line := range lines {
		lines[i] = dimStyle.Copy().PaddingLeft(6).Render(line)
	}
	return strings.Join(lines, "\n")
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
		add("type", result.Discovery.OrgType)
		add("status", result.Discovery.OrgStatus)
		add("expires", result.Discovery.ExpiresAt)
		add("org ID", result.Discovery.OrgID)
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
	return strings.Join(parts, "\n")
}

// renderProviderConnectionDetails styles each physical line separately. A
// single lipgloss Render over multiline text pads every line to the longest
// value with default-background cells, which cuts black blocks into a selected
// row's blue background.
func renderProviderConnectionDetails(result checker.ProviderResult) string {
	details := providerConnectionDetails(result)
	if details == "" {
		return ""
	}
	lines := strings.Split(details, "\n")
	for i, line := range lines {
		lines[i] = dimStyle.Copy().PaddingLeft(6).Render(line)
	}
	return strings.Join(lines, "\n")
}

func (m rootShellModel) viewProvider(width int) string {
	result := m.results[m.provider]
	detail := "No check has been run."
	if m.provider == "salesforce" && m.statuses["salesforce"] == connectionConnected && providerConnectionDetails(result) != "" {
		detail = titleStyle.Render("Current org") + "\n" + renderProviderConnectionDetails(result)
	} else if len(result.Checks) > 0 {
		checks := append([]string(nil), result.Checks...)
		if m.provider == "salesforce" {
			checks = slices.DeleteFunc(checks, func(check string) bool {
				return strings.EqualFold(strings.TrimSpace(check), "salesforce tools discovered")
			})
		}
		if len(checks) > 0 {
			detail = strings.Join(checks, "\n")
		}
	}
	actionLabels := map[string]string{
		"salesforce-done": "Continue with this org", "salesforce-existing": "Use a different Salesforce org", "check": "Check connection", "choose": "Choose authenticated profile", "ccg": "Connect with a CCG app (client credentials)", "switch": "Switch Box connection / set default", "scratch": "Create or replace a 30-day scratch org", "connect": "Connect using provider CLI", "back": "Back to launch plan",
	}
	if m.provider == "salesforce" && m.statuses["salesforce"] != connectionConnected {
		actionLabels["salesforce-existing"] = "Use an existing Salesforce org"
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
	options := m.providerOptions()
	rows := make([]string, len(options))
	for i, option := range options {
		style := panel.Copy().Width(width - 4)
		if i == m.cursor {
			style = activePane.Copy().Width(width - 4)
		}
		rows[i] = style.Render(option)
	}
	title := "Choose " + providerLabel(m.provider) + " profile"
	subtitle := ""
	if m.provider == "salesforce" {
		title = "Use an existing Salesforce org"
		subtitle = dimStyle.Render("Choose a locally authenticated org or sign in to another one.") + "\n\n"
	}
	return titleStyle.Render(title) + "\n" + subtitle + strings.Join(rows, "\n")
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
		directoryStyle.Render(titleStyle.Render("1  Parent directory")+"\n"+m.directoryInput.View()) + "\n" +
		nameStyle.Render(titleStyle.Render("2  Package directory name")+"\n"+m.packageInput.View()) + "\n" +
		strategyStyle.Render(titleStyle.Render("3  Deployment strategy")+"\n"+accent.Render(deploymentStrategyLabel(m.answers.deploymentStrategy))+"\n"+dimStyle.Render(m.deploymentNamePreview())) + "\n" +
		boxStyle.Render(titleStyle.Render("4  Box components")+"\n"+accent.Render(strings.ToUpper(m.boxComponentMode))+dimStyle.Render(fmt.Sprintf(" · %d capabilities", len(m.boxCapabilities)))) + "\n" +
		continueStyle.Render(lipgloss.NewStyle().Bold(true).Foreground(coral).Render("5  Review deployment  →")+"\n"+dimStyle.Render(destination))
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
			if !capability.CanDeploy() {
				m.message = capability.ComponentType + " is reference-only because its public API does not support deployment."
				return m, nil
			}
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
		canDeploy := capability.CanDeploy()
		status := "NO PUBLIC API"
		if strings.EqualFold(strings.TrimSpace(capability.API), "partial") {
			status = "PARTIAL API"
		}
		// Same marker vocabulary and colours the validate and deploy checklists
		// use: green tick for what will be deployed, gold dash for reference-only
		// capabilities, dim ring for supported capabilities not selected.
		marker, tone := "—", gold
		switch {
		case canDeploy && !enabled:
			marker, tone = "○", muted
			status = "EXCLUDED"
		case canDeploy:
			marker, tone = "✓", green
			status = "READY"
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
	return m.stageHeader() + "\n\n" + titleStyle.Render("Configure Box components") + "  " + mode + "\n" +
		dimStyle.Render("Global modes affect supported rows only. Gold rows are visible references and cannot be selected.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n"))
}

func (m rootShellModel) boxCapabilitySelected(capability solution.Capability) bool {
	if !capability.CanDeploy() {
		return false
	}
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
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose the parent directory") + "\n" + dimStyle.Render("Navigate the folder tree, then choose where Box Dispatch should create the package.") + "\n\n" +
		activePane.Copy().Padding(1, 2).Width(width-4).Render(content)
}

func (m rootShellModel) viewPackage(width int) string {
	templateName, repository := "No quickstart selected", ""
	if m.selected != nil {
		templateName, repository = m.selected.name, m.selected.repository
	}
	selectedCapabilities := 0
	for _, capability := range m.boxCapabilities {
		if m.boxCapabilitySelected(capability) {
			selectedCapabilities++
		}
	}
	details := []string{
		titleStyle.Render("Quickstart") + "\n" + accent.Render(templateName) + "\n" + dimStyle.Render(repository),
		titleStyle.Render("Providers") + "\n" + strings.Join(m.selectedProviderLabels(), " + "),
		titleStyle.Render("Destination") + "\n" + m.packagePath,
		titleStyle.Render("Deployment strategy") + "\n" + deploymentStrategyLabel(m.answers.deploymentStrategy),
		titleStyle.Render("Box components") + "\n" + fmt.Sprintf("%s · %d supported capabilities selected", strings.ToUpper(m.boxComponentMode), selectedCapabilities),
	}
	pipeline := strings.Join([]string{
		accent.Render("DEPLOYMENT PIPELINE"),
		"1  Assemble the selected quickstart package",
		"2  Validate provider state and prerequisites",
		"3  Install managed packages and deploy supported missing configuration",
		"4  Assign and verify required permission sets",
	}, "\n")
	return m.stageHeader() + "\n\n" + titleStyle.Render("Review deployment") + "\n" + dimStyle.Render("Nothing has been created or changed yet.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(details, "\n\n")) + "\n" +
		panel.Copy().BorderForeground(cyan).Width(width-4).Padding(1, 2).Render(pipeline) + "\n" +
		accent.Render("Enter / →  Confirm and deploy")
}

func (m rootShellModel) viewValidate(width int) string {
	prefix, suffix, lines, visible := m.validationViewport(width)
	body := renderLifecycleViewport(lines, visible, m.lifecycleScroll)
	return prefix + panel.Copy().Width(width-4).Padding(1, 2).Render(body) + suffix
}

func (m rootShellModel) validationHasFailures() bool {
	for _, item := range m.validationItems {
		if item.Status == lifecycle.StatusFailed {
			return true
		}
	}
	return false
}

func (m rootShellModel) lifecycleDiagnostic() (string, string) {
	for _, item := range m.validationItems {
		if strings.TrimSpace(item.Diagnostic) != "" {
			title := providerLabel(item.Provider) + " full CLI diagnostics"
			return title, item.Diagnostic
		}
	}
	return "", ""
}

func (m *rootShellModel) openDiagnostic() bool {
	title, body := "", ""
	if m.screen == screenProvider {
		body = m.providerDiagnostics[m.provider]
		if body != "" {
			title = providerLabel(m.provider) + " full CLI diagnostics"
		}
	} else {
		title, body = m.lifecycleDiagnostic()
	}
	if strings.TrimSpace(body) == "" {
		m.message = "No full diagnostic payload is available for this result."
		return false
	}
	m.diagnosticTitle = title
	m.diagnosticBody = body
	m.diagnosticReturn = m.screen
	m.diagnosticScroll = 0
	m.screen = screenDiagnostic
	m.message = "Sensitive token and secret fields are redacted."
	return true
}

func (m rootShellModel) diagnosticLines(width int) []string {
	wrapped := lipgloss.NewStyle().Width(max(width-10, 20)).Render(m.diagnosticBody)
	return strings.Split(wrapped, "\n")
}

func (m rootShellModel) diagnosticCapacity() int {
	return max(m.height-18, 5)
}

func (m *rootShellModel) scrollDiagnostic(delta int) {
	width := min(max(m.width-8, 64), 112)
	limit := max(len(m.diagnosticLines(width))-m.diagnosticCapacity(), 0)
	m.diagnosticScroll = min(max(m.diagnosticScroll+delta, 0), limit)
}

func (m rootShellModel) viewDiagnostic(width int) string {
	lines := m.diagnosticLines(width)
	capacity := m.diagnosticCapacity()
	start := min(max(m.diagnosticScroll, 0), max(len(lines)-capacity, 0))
	end := min(start+capacity, len(lines))
	shown := lines[start:end]
	position := fmt.Sprintf("lines %d–%d of %d", start+1, end, len(lines))
	if len(lines) == 0 {
		shown = []string{"No diagnostic payload was returned."}
		position = "no diagnostic lines"
	}
	return titleStyle.Render(m.diagnosticTitle) + "\n" +
		dimStyle.Render("Complete sanitized Salesforce CLI payload. Stack and error.data are retained; credentials are redacted.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(shown, "\n")) + "\n" +
		dimStyle.Render(position+" · ↑/↓ or PgUp/PgDn scroll · d/Esc returns")
}

func (m rootShellModel) viewScratchConfirm(width int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("Create a 30-day Salesforce scratch org?")
	detail := dimStyle.Copy().Width(width - 10).Render(
		"Dispatch will create a Developer edition scratch org with the explicitly selected Dev Hub and make it the selected Salesforce target. This consumes one Dev Hub scratch-org allocation.",
	)
	selection := titleStyle.Render("Dev Hub  ") + firstNonEmptyString(m.selectedDevHub.Alias, m.selectedDevHub.Username) + "\n" +
		titleStyle.Render("Alias    ") + m.pendingScratchAlias
	create := confirmButton("Create scratch org", cyan, m.scratchConfirmCursor == 0)
	cancel := confirmButton("Cancel", coral, m.scratchConfirmCursor == 1)
	hint := dimStyle.Render("←/→ switch · Enter/Space confirm · Esc cancel")
	body := title + "\n\n" + detail + "\n\n" + selection + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, create, "  ", cancel) + "\n\n" + hint
	return m.stageHeader() + "\n\n" + activePane.Copy().Width(width-4).Padding(1, 2).Render(body)
}

func (m rootShellModel) viewDevHubs(width int) string {
	rows := make([]string, 0, len(m.devHubs))
	for i, hub := range m.devHubs {
		status := lipgloss.NewStyle().Bold(true).Foreground(gold).Render("STATUS UNKNOWN")
		if hub.Connected() {
			status = lipgloss.NewStyle().Bold(true).Foreground(green).Render("● CONNECTED")
		}
		name := firstNonEmptyString(hub.Alias, hub.Username)
		detail := titleStyle.Render(name) + "  " + status
		if hub.Username != "" && hub.Username != name {
			detail += "\n" + dimStyle.Render("user "+hub.Username)
		}
		if hub.OrgID != "" {
			detail += "\n" + dimStyle.Render("org ID "+hub.OrgID)
		}
		style := panel.Copy().Width(width - 4)
		if i == m.cursor {
			style = activePane.Copy().Width(width - 4)
		}
		rows = append(rows, style.Render(detail))
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose a Salesforce Dev Hub") + "\n" +
		dimStyle.Render("Dispatch passes this selection explicitly to sf org create scratch; no CLI-wide default is required.") + "\n\n" +
		strings.Join(rows, "\n")
}

func (m rootShellModel) validationViewport(width int) (prefix, suffix string, lines []string, visible int) {
	rows := m.validationRows(width)
	status := "VALIDATION RESULTS"
	if m.validateRunning {
		status = m.spinner.View() + " VALIDATING PACKAGE AND PROVIDERS"
	}
	footer := accent.Render("Enter / →  Continue to Deploy    r  Validate again")
	if m.screen == screenDeploy {
		footer = dimStyle.Render("Deployment continues automatically after validation succeeds.")
	}
	if m.validationHasFailures() {
		footer = lipgloss.NewStyle().Bold(true).Foreground(coral).Render("Deployment stopped: resolve failed validation")
		if m.screen == screenValidate {
			footer += "    " + accent.Render("r  Validate again")
		} else {
			footer += "    " + accent.Render("←  Return to Review")
		}
	}
	if _, diagnostic := m.lifecycleDiagnostic(); diagnostic != "" {
		footer += "    " + accent.Render("d  Full error details")
	}
	if m.validateRunning {
		if activity := m.renderActivity(width - 4); activity != "" {
			footer = activity
		}
	}
	if m.screen == screenDeploy {
		status = "Validating providers"
		if m.validationHasFailures() {
			status = "Validation stopped"
		}
		prefix = m.stageHeader() + "\n\n" + m.deployPipelineStrip() + "\n\n" + titleStyle.Render(status) + "\n" + dimStyle.Render("Checking provider state and deployment prerequisites.") + "\n\n"
	} else {
		prefix = m.stageHeader() + "\n\n" + titleStyle.Render(status) + "\n" + dimStyle.Render("Receipts identify existing provider state; unverified packaged assets remain visible as deployment work.") + "\n\n"
	}
	suffix = "\n" + footer
	separator := "\n\n"
	if m.screen == screenDeploy && !m.deployShowDetails {
		separator = "\n"
	}
	lines = lifecycleResultLines(rows, separator)
	visible = m.lifecycleViewportCapacity(width, prefix, suffix)
	return prefix, suffix, lines, visible
}

func (m rootShellModel) validationRows(width int) []string {
	rows := []string{}
	for _, provider := range m.selectedProviders() {
		item := findLifecycleItem(m.validationItems, provider)
		if m.screen == screenDeploy && !m.deployShowDetails {
			rows = append(rows, m.focusedProviderRow(provider, m.validationProgress[provider], item, provider == m.currentValidation, "validate", width-10))
		} else {
			rows = append(rows, m.providerProgressRow(provider, m.validationProgress[provider], item, provider == m.currentValidation, "validate", width-10))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("No validation results yet."))
	}
	return rows
}

func lifecycleResultLines(rows []string, separator string) []string {
	if len(rows) == 0 {
		return nil
	}
	return strings.Split(strings.Join(rows, separator), "\n")
}

// lifecycleViewportCapacity measures the real shell chrome around an empty
// results panel. Replacing the panel's single empty content row with N visible
// rows adds N-1 lines, so the remaining terminal height is the exact viewport
// capacity. A small floor keeps very short terminals usable.
func (m rootShellModel) lifecycleViewportCapacity(width int, prefix, suffix string) int {
	emptyPanel := panel.Copy().Width(width-4).Padding(1, 2).Render("")
	framed := lipgloss.NewStyle().Margin(1, 3).Width(width).Render(
		m.header(width) + "\n\n" + prefix + emptyPanel + suffix + "\n\n" + m.footer(),
	)
	return max(m.height-lipgloss.Height(framed)+1, 3)
}

func lifecycleViewportContentRows(total, visible int) int {
	if total > visible {
		return max(visible-1, 1) // reserve one line for the scroll position
	}
	return max(visible, 1)
}

func lifecycleScrollLimit(total, visible int) int {
	return max(total-lifecycleViewportContentRows(total, visible), 0)
}

// clampLifecycleScroll advances within a bounded checklist without wrapping.
// Repeated Down presses at the final row and repeated Up presses at the first
// row are deliberate no-ops. Normalizing current first also recovers cleanly
// when a terminal resize makes an old offset larger than the new viewport.
func clampLifecycleScroll(current, delta, limit int) int {
	current = min(max(current, 0), max(limit, 0))
	if delta > 0 {
		return min(current+delta, limit)
	}
	if delta < 0 {
		return max(current+delta, 0)
	}
	return current
}

func renderLifecycleViewport(lines []string, visible, scroll int) string {
	if len(lines) == 0 {
		return dimStyle.Render("No provider results yet.")
	}
	contentRows := lifecycleViewportContentRows(len(lines), visible)
	limit := lifecycleScrollLimit(len(lines), visible)
	scroll = clampLifecycleScroll(scroll, 0, limit)
	end := min(scroll+contentRows, len(lines))
	shown := append([]string(nil), lines[scroll:end]...)
	if len(lines) > visible {
		up, down := "  ", "  "
		if scroll > 0 {
			up = "↑ "
		}
		if end < len(lines) {
			down = "↓ "
		}
		position := fmt.Sprintf("%s%s lines %d–%d of %d · ↑/↓ scroll", up, down, scroll+1, end, len(lines))
		if end == len(lines) {
			position += " · end of checklist"
		}
		shown = append(shown, dimStyle.Render(position))
	}
	return strings.Join(shown, "\n")
}

func (m *rootShellModel) scrollLifecycleResults(delta int) {
	limit, ok := m.lifecycleScrollLimit()
	if !ok {
		return
	}
	m.lifecycleScroll = clampLifecycleScroll(m.lifecycleScroll, delta, limit)
	if delta < 0 {
		// Like tail -f, moving away from the latest row pauses following until
		// the operator navigates back to the bottom.
		m.lifecycleFollowTail = false
	} else if delta > 0 {
		m.lifecycleFollowTail = m.lifecycleScroll == limit
	}
}

func (m *rootShellModel) followLifecycleTail() {
	if !m.lifecycleFollowTail {
		return
	}
	if (m.screen == screenValidate || m.screen == screenDeploy) && m.validateRunning && m.currentValidation != "" {
		if scroll, ok := m.activeValidationScroll(); ok {
			m.lifecycleScroll = scroll
			return
		}
	}
	limit, ok := m.lifecycleScrollLimit()
	if ok {
		m.lifecycleScroll = limit
	}
}

// activeValidationScroll keeps the category currently advancing at the bottom
// of the viewport. The checklist is pre-rendered, so blindly following its
// absolute bottom would show pending work until validation was almost done.
func (m rootShellModel) activeValidationScroll() (int, bool) {
	width := min(max(m.width-8, 64), 112)
	providers := m.selectedProviders()
	providerIndex := slices.Index(providers, m.currentValidation)
	if providerIndex < 0 {
		return 0, false
	}
	rows := m.validationRows(width)
	if providerIndex >= len(rows) {
		return 0, false
	}
	rowLines := strings.Split(rows[providerIndex], "\n")
	checklistHeader := -1
	for index, line := range rowLines {
		if strings.Contains(line, "DEPLOYMENT CHECKLIST") {
			checklistHeader = index
			break
		}
	}
	targetInRow := 0
	if checklistHeader >= 0 {
		categoryCount := len(rowLines) - checklistHeader - 1
		if categoryCount > 0 {
			progress := math.Min(math.Max(m.validationProgress[m.currentValidation], 0), 1)
			categoryIndex := int(math.Ceil(progress*float64(categoryCount))) - 1
			categoryIndex = min(max(categoryIndex, 0), categoryCount-1)
			targetInRow = checklistHeader + 1 + categoryIndex
		} else {
			targetInRow = checklistHeader
		}
	}
	target := targetInRow
	for index := 0; index < providerIndex; index++ {
		target += len(strings.Split(rows[index], "\n")) + 1 // one blank separator line
	}
	_, _, lines, visible := m.validationViewport(width)
	contentRows := lifecycleViewportContentRows(len(lines), visible)
	limit := lifecycleScrollLimit(len(lines), visible)
	return clampLifecycleScroll(target-contentRows+1, 0, limit), true
}

func (m rootShellModel) lifecycleScrollLimit() (int, bool) {
	width := min(max(m.width-8, 64), 112)
	var lines []string
	visible := 0
	switch m.screen {
	case screenValidate:
		_, _, lines, visible = m.validationViewport(width)
	case screenDeploy:
		if m.deploymentPhase == deploymentPhaseValidate || (m.deploymentPhase == deploymentPhaseFailed && m.validateDone) {
			_, _, lines, visible = m.validationViewport(width)
		} else {
			_, _, lines, visible = m.deployProviderViewport(width)
		}
	default:
		return 0, false
	}
	return lifecycleScrollLimit(len(lines), visible), true
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

	rows := m.teardownBodyRows(width)
	// The resource list is often longer than the terminal, so window it around the
	// scroll offset (↑/↓ move it) instead of overflowing off-screen.
	visible := m.teardownVisibleRows()
	scroll := m.clampedTeardownScroll(len(rows), visible)
	shown := rows
	if len(rows) > visible {
		shown = append([]string(nil), rows[scroll:min(scroll+visible, len(rows))]...)
		hint := fmt.Sprintf("rows %d–%d of %d", scroll+1, scroll+len(shown), len(rows))
		up, down := "  ", "  "
		if scroll > 0 {
			up = "↑ "
		}
		if scroll+visible < len(rows) {
			down = "↓ "
		}
		shown = append(shown, dimStyle.Render(fmt.Sprintf("%s%s more · %s scroll", up, down, hint)))
	}

	panelBody := panel.Copy().Padding(1, 2).Width(width - 4).Render(strings.Join(shown, "\n"))
	if m.confirmingTeardown && m.teardownConfirmForm != nil {
		return title + "\n" + subtitle + "\n\n" + panelBody + "\n" + activePane.Copy().Width(width-4).Render(m.teardownConfirmForm.View())
	}
	view := title + "\n" + subtitle + "\n\n" + panelBody
	if m.teardownStarted && !m.teardownDone {
		if activity := m.renderActivity(width - 4); activity != "" {
			view += "\n" + activity
		}
	}
	return view
}

// teardownBodyRows builds every line of the reset preview / result view. It is
// shared by the renderer (which windows it) and the key handler (which clamps
// the scroll offset to its length).
func (m rootShellModel) teardownBodyRows(width int) []string {
	if m.teardownRecord == nil {
		return nil
	}
	record := *m.teardownRecord
	body := []string{}
	if m.teardownDone {
		// Mirror the post-deploy screen: a status table of every resource the
		// reset touched, deleted and remaining alike.
		return m.teardownResultRows(width)
	}
	if m.teardownStarted {
		for _, provider := range m.teardownProviders {
			body = append(body, m.providerProgressRow(provider, m.teardownProgress[provider], nil, provider == m.currentTeardown, "teardown", width-10))
		}
		return body
	}
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
	return body
}

// teardownResultRows renders the reset results as a status table — one row per
// resource the destroy pass touched, mirroring the post-deploy assets table so
// the two screens read the same. Deleted rows show green, failures coral, and
// unmanaged (no delete API) gold with a reason.
func (m rootShellModel) teardownResultRows(width int) []string {
	inner := width - 8
	wStatus, wSource, wType := 10, 10, 16
	rest := inner - wStatus - wSource - wType - 4 // 4 single-space gaps
	if rest < 24 {
		rest = 24
	}
	wName := rest * 3 / 5
	wID := rest - wName
	cell := func(s string, n int, tone lipgloss.Color) string {
		style := lipgloss.NewStyle().Width(n)
		if tone != "" {
			style = style.Foreground(tone)
		}
		return style.Render(truncateCell(s, n))
	}
	deleted, remaining := 0, 0
	dataRows := []string{}
	for _, result := range m.teardownResults {
		for _, o := range result.Outcomes {
			status, tone, reason := "✓ deleted", green, ""
			switch {
			case o.Deleted:
				deleted++
			case o.Unmanaged:
				status, tone, reason = "○ manual", gold, "no delete API; remove manually"
				remaining++
			default:
				status, tone, reason = "× failed", coral, o.Error
				remaining++
			}
			id := o.Resource.ID
			if strings.TrimSpace(id) == "" {
				id = "—"
			}
			row := strings.Join([]string{
				cell(status, wStatus, tone),
				cell(providerLabel(result.Provider), wSource, white),
				cell(o.Resource.Kind, wType, ""),
				cell(o.Resource.Name, wName, ""),
				cell(id, wID, muted),
			}, " ")
			if reason != "" {
				row += dimStyle.Render("  " + reason)
			}
			dataRows = append(dataRows, row)
		}
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(green).Render(fmt.Sprintf("✓  %d deleted", deleted))
	if remaining > 0 {
		heading += lipgloss.NewStyle().Bold(true).Foreground(coral).Render(fmt.Sprintf("   ·   %d remaining", remaining))
	}
	header := dimStyle.Render(strings.Join([]string{
		cellPlain("STATUS", wStatus), cellPlain("SOURCE", wSource), cellPlain("TYPE", wType),
		cellPlain("NAME", wName), cellPlain("ID", wID),
	}, " "))
	rows := []string{heading, "", header}
	return append(rows, dataRows...)
}

// teardownVisibleRows is how many preview rows fit under the header, title, and
// footer chrome on the current terminal.
func (m rootShellModel) teardownVisibleRows() int {
	n := m.height - 14
	if m.confirmingTeardown {
		n -= 4
	}
	if n < 6 {
		n = 6
	}
	return n
}

// clampedTeardownScroll keeps the offset within [0, total-visible].
func (m rootShellModel) clampedTeardownScroll(total, visible int) int {
	maxScroll := total - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.teardownScroll > maxScroll {
		return maxScroll
	}
	if m.teardownScroll < 0 {
		return 0
	}
	return m.teardownScroll
}

func (m rootShellModel) viewDeploy(width int) string {
	if m.confirmingDeploy && m.deploymentPhase == deploymentPhaseReview {
		return m.stageHeader() + "\n\n" + m.deployPipelineStrip() + "\n\n" + titleStyle.Render("Deploy solution") + "\n" +
			dimStyle.Render("One confirmation starts package assembly, validation, prerequisites, and provider deployment.") + "\n\n" +
			activePane.Copy().Width(width-4).Render(m.renderDeployConfirm(width-8))
	}
	if m.deploymentPhase == deploymentPhasePackage {
		status := m.spinner.View() + " Assembling the selected solution package"
		if !m.packageStarted {
			status = "○ Package assembly stopped"
		}
		body := panel.Copy().Width(width-4).Padding(1, 2).Render(lipgloss.NewStyle().Bold(true).Foreground(gold).Render(status))
		if activity := m.renderActivity(width - 4); activity != "" {
			body += "\n" + activity
		}
		return m.stageHeader() + "\n\n" + m.deployPipelineStrip() + "\n\n" + titleStyle.Render("Assembling package") + "\n" + dimStyle.Render("Preparing the reviewed solution for validation.") + "\n\n" + body
	}
	if m.deploymentPhase == deploymentPhaseValidate || (m.deploymentPhase == deploymentPhaseFailed && m.validateDone) {
		return m.viewValidate(width)
	}
	if m.deploymentPhase == deploymentPhaseFailed {
		return m.stageHeader() + "\n\n" + m.deployPipelineStrip() + "\n\n" + titleStyle.Render("Deployment stopped") + "\n\n" +
			panel.Copy().BorderForeground(coral).Width(width-4).Padding(1, 2).Render(lipgloss.NewStyle().Foreground(coral).Render(m.message)) + "\n" +
			accent.Render("←  Return to Review")
	}
	head, action := m.deployHeadAction(width)
	assets := ""
	if m.deployDone && m.deployShowDetails {
		if table := m.renderDeployedAssetsTable(width, m.deployTableCapacity()); table != "" {
			assets = "\n" + table
		}
	}
	return head + assets + "\n" + action
}

// deployPipelineStrip is the stable local orientation for the unified Deploy
// stage. It stays in place while the content below changes from assembly to
// validation, apply and completion, so the operator never has to reconstruct
// where the long-running workflow is.
func (m rootShellModel) deployPipelineStrip() string {
	labels := []string{"ASSEMBLE", "VALIDATE", "APPLY", "FINISH"}
	active := 0
	switch m.deploymentPhase {
	case deploymentPhaseValidate:
		active = 1
	case deploymentPhaseApply:
		active = 2
	case deploymentPhaseComplete:
		active = 3
	case deploymentPhaseFailed:
		if m.packageDone {
			active = 1
		}
		if m.validateDone && !m.validationHasFailures() {
			active = 2
		}
	}
	failedAt := -1
	if m.deploymentPhase == deploymentPhaseFailed {
		failedAt = active
	} else if m.deploymentPhase == deploymentPhaseComplete && m.deploymentFailureCount() > 0 {
		failedAt = 3
	}
	parts := make([]string, 0, len(labels)*2-1)
	for index, label := range labels {
		if index > 0 {
			parts = append(parts, dimStyle.Render(" ─ "))
		}
		style, marker := dimStyle, "○"
		switch {
		case index == failedAt:
			style, marker = lipgloss.NewStyle().Bold(true).Foreground(coral), "×"
		case index < active || (m.deploymentPhase == deploymentPhaseComplete && index <= active):
			style, marker = lipgloss.NewStyle().Bold(true).Foreground(green), "✓"
		case index == active && m.deploymentPhase != deploymentPhaseReview:
			style, marker = lipgloss.NewStyle().Bold(true).Foreground(gold), m.spinner.View()
		}
		parts = append(parts, style.Render(marker+" "+label))
	}
	return strings.Join(parts, "")
}

func (m rootShellModel) deploymentFailureCount() int {
	failed := 0
	for _, item := range m.validationItems {
		if item.Status == lifecycle.StatusFailed {
			failed++
		}
	}
	return failed
}

func (m rootShellModel) deploymentOutcomeTitle() string {
	if failed := m.deploymentFailureCount(); failed > 0 {
		return "Deployment finished with errors"
	}
	return "Deployment complete"
}

func (m rootShellModel) deploymentOutcomeCounts() (created, existing, failed int) {
	for _, asset := range m.deployedAssets() {
		if asset.created {
			created++
		} else {
			existing++
		}
	}
	return created, existing, m.deploymentFailureCount()
}

func (m rootShellModel) deploymentDurationSuffix() string {
	if m.deploymentStartedAt.IsZero() || m.deploymentCompletedAt.IsZero() {
		return ""
	}
	duration := m.deploymentCompletedAt.Sub(m.deploymentStartedAt).Round(time.Second)
	if duration < 0 {
		return ""
	}
	return " · " + duration.String()
}

// deployHeadAction builds the two fixed parts of the deploy screen — everything
// above the assets table (stage header, title, and the provider progress panel)
// and the action block below it. Keeping them separate lets deployTableCapacity
// measure the surrounding chrome so the assets table is sized to fit the screen.
func (m rootShellModel) deployHeadAction(width int) (head, action string) {
	prefix, action, lines, visible := m.deployProviderViewport(width)
	body := strings.Join(lines, "\n")
	if !m.deployDone {
		body = renderLifecycleViewport(lines, visible, m.lifecycleScroll)
	}
	head = prefix + panel.Copy().Width(width-4).Padding(1, 2).Render(body)
	return head, action
}

func (m rootShellModel) deployProviderViewport(width int) (prefix, action string, lines []string, visible int) {
	rows, deployable, rowSep := m.deployProviderRows(width)
	action = m.deployAction(width, deployable)
	title, subtitle := "Applying configuration", "Only supported missing configuration will be applied."
	if m.deployDone {
		title, subtitle = m.deploymentOutcomeTitle(), "The deployment is finished. Open a destination or review the detailed resources."
	}
	prefix = m.stageHeader() + "\n\n" + m.deployPipelineStrip() + "\n\n" + titleStyle.Render(title) + "\n" + dimStyle.Render(subtitle) + "\n\n"
	lines = lifecycleResultLines(rows, rowSep)
	visible = m.lifecycleViewportCapacity(width, prefix, "\n"+action)
	return prefix, action, lines, visible
}

func (m rootShellModel) deployProviderRows(width int) (rows []string, deployable int, rowSep string) {
	rows = []string{}
	rowSep = "\n\n"
	for _, item := range m.validationItems {
		if item.Status == lifecycle.StatusMissing && item.Deployable {
			deployable++
		}
		if m.deployDone {
			// One line per provider once the run is done; the results live in the
			// deployed-assets table below.
			rows = append(rows, m.providerSummaryLine(item, width-10))
			rowSep = "\n"
			continue
		}
		if m.deployShowDetails {
			rows = append(rows, m.providerProgressRow(item.Provider, m.deploymentProgress[item.Provider], &item, item.Provider == m.currentDeployment, "deploy", width-10))
		} else {
			rows = append(rows, m.focusedProviderRow(item.Provider, m.deploymentProgress[item.Provider], &item, item.Provider == m.currentDeployment, "deploy", width-10))
			rowSep = "\n"
		}
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("Validate the package before deployment."))
	}
	return rows, deployable, rowSep
}

func (m rootShellModel) deployAction(width, deployable int) string {
	action := accent.Render(fmt.Sprintf("Enter / →  Deploy %d supported missing configuration set(s)", deployable))
	if m.confirmingDeploy {
		action = activePane.Copy().Width(width - 4).Render(m.renderDeployConfirm(width - 8))
	}
	if m.deployStarted {
		activity := m.renderActivity(width - 4)
		if activity == "" {
			activity = m.spinner.View() + " Deploying provider configuration..."
		}
		action = activity
	} else if m.deployDone {
		created, existing, failed := m.deploymentOutcomeCounts()
		tone, marker := green, "✓"
		if failed > 0 {
			tone, marker = coral, "×"
		}
		action = lipgloss.NewStyle().Bold(true).Foreground(tone).Render(marker + " " + m.deploymentOutcomeTitle())
		action += "\n" + dimStyle.Render(fmt.Sprintf("%d created · %d already present · %d failed%s", created, existing, failed, m.deploymentDurationSuffix()))
		if m.deploymentAuditPath == "" {
			action += "\n" + lipgloss.NewStyle().Foreground(coral).Render("× Audit export needs attention")
		} else {
			action += "\n" + lipgloss.NewStyle().Foreground(green).Render("✓ Audit saved")
			if m.deployShowDetails {
				action += "\n" + dimStyle.Render(m.deploymentAuditPath)
			}
		}
	}
	return action
}

// deployTableCapacity returns how many asset rows fit beneath the deploy chrome
// on the current terminal, so the whole screen (chrome + table) stays within the
// viewport and the table remains scrollable in both directions.
func (m rootShellModel) deployTableCapacity() int {
	width := min(max(m.width-8, 64), 112) // same content width the View() wrapper uses
	head, action := m.deployHeadAction(width)
	// View() wraps body with the header, two blank separators and the footer,
	// inside a one-row top/bottom margin (~8 rows); the table panel adds its
	// heading, legend, blank, column header and footer plus border and padding
	// (~9 rows).
	const viewFrame, tableFrame = 8, 9
	used := lipgloss.Height(head) + lipgloss.Height(action) + viewFrame + tableFrame
	if n := m.height - used; n >= 3 {
		return n
	}
	return 3
}

// deployedResources flattens every asset recorded across providers this run.
func (m rootShellModel) deployedResources() []lifecycle.ResourceReference {
	out := []lifecycle.ResourceReference{}
	for _, item := range m.validationItems {
		out = append(out, item.Resources...)
	}
	return out
}

// deployedAsset pairs a resource reference with whether this run created it. The
// deploy adapters only record IDs for components they create, so already-present
// configuration (metadata templates, Doc Gen templates, AI agents, hubs, the
// configuration carries no ID — but the operator still wants to see it in the
// post-deploy table. Those are surfaced from the validation Present list as
// "existing" rows so the table reflects the whole solution, not just new files.
type deployedAsset struct {
	ref     lifecycle.ResourceReference
	created bool
}

func (m rootShellModel) deployedAssets() []deployedAsset {
	assets := []deployedAsset{}
	// Component keys already represented by a created resource, so an existing
	// row is not also emitted for the same thing.
	covered := map[string]bool{}
	for _, item := range m.validationItems {
		for _, r := range item.Resources {
			assets = append(assets, deployedAsset{ref: r, created: true})
			covered[r.Provider+"|"+r.Component] = true
		}
	}
	for _, item := range m.validationItems {
		for _, component := range item.Present {
			if covered[item.Provider+"|"+component] {
				continue
			}
			assets = append(assets, deployedAsset{ref: lifecycle.ResourceReference{
				Provider:  item.Provider,
				Component: component,
				Kind:      kindForComponent(component),
				Name:      componentName(component),
			}})
		}
	}
	return assets
}

// componentName returns the display name half of a "Type:Name" component key.
func componentName(component string) string {
	if i := strings.Index(component, ":"); i >= 0 {
		return component[i+1:]
	}
	return component
}

// kindForComponent maps a component's "Type:" prefix onto the same kind label
// the deploy adapters record for created resources, so existing and created rows
// read consistently. Unknown types fall back to a snake_case of the prefix.
func kindForComponent(component string) string {
	switch componentType(component) {
	case "Metadata Template":
		return "metadata_template"
	case "Doc Gen Template":
		return "docgen_template"
	case "AI Agent":
		return "ai_agent"
	case "Extract Configuration":
		return "extract"
	case "Box Hub":
		return "hub"
	case "Automate Workflow":
		return "automate_workflow"
	}
	return strings.ReplaceAll(strings.ToLower(componentType(component)), " ", "_")
}

// renderDeployedAssetsTable lists what a successful run actually created — one row
// per asset with its status, source system, component, kind, name, and id. The
// list is windowed by deployAssetsScroll (↑/↓) so a large deployment stays on
// screen.
func (m rootShellModel) renderDeployedAssetsTable(width, visible int) string {
	assets := m.deployedAssets()
	if len(assets) == 0 {
		return ""
	}
	inner := width - 8
	wStatus, wSource, wComp, wType := 8, 10, 20, 16
	rest := inner - wStatus - wSource - wComp - wType - 5 // 5 single-space gaps
	if rest < 30 {
		rest = 30
	}
	wName := rest * 3 / 5
	wID := rest - wName
	cell := func(s string, n int, tone lipgloss.Color) string {
		style := lipgloss.NewStyle().Width(n)
		if tone != "" {
			style = style.Foreground(tone)
		}
		return style.Render(truncateCell(s, n))
	}
	head := dimStyle.Render(strings.Join([]string{
		cellPlain("STATUS", wStatus), cellPlain("SOURCE", wSource), cellPlain("COMPONENT", wComp),
		cellPlain("TYPE", wType), cellPlain("NAME", wName), cellPlain("ID", wID),
	}, " "))
	created := 0
	rowsAll := make([]string, 0, len(assets))
	for _, a := range assets {
		status, tone, id := "existing", muted, a.ref.ID
		if a.created {
			created++
			status, tone = "✓ new", green
		}
		if strings.TrimSpace(id) == "" {
			id = "—"
		}
		rowsAll = append(rowsAll, strings.Join([]string{
			cell(status, wStatus, tone),
			cell(providerLabel(a.ref.Provider), wSource, white),
			cell(componentType(a.ref.Component), wComp, ""),
			cell(a.ref.Kind, wType, ""),
			cell(a.ref.Name, wName, ""),
			cell(id, wID, muted),
		}, " "))
	}
	existing := len(rowsAll) - created
	scroll := m.deployAssetsScroll
	if maxScroll := len(rowsAll) - visible; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	shown, footer := rowsAll, ""
	if len(rowsAll) > visible {
		shown = rowsAll[scroll:min(scroll+visible, len(rowsAll))]
		up, down := "  ", "  "
		if scroll > 0 {
			up = "↑ "
		}
		if scroll+visible < len(rowsAll) {
			down = "↓ "
		}
		footer = "\n" + dimStyle.Render(fmt.Sprintf("%s%s rows %d–%d of %d · ↑/↓ scroll", up, down, scroll+1, scroll+len(shown), len(rowsAll)))
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(green).Render(fmt.Sprintf("✓  %d deployed", created))
	if existing > 0 {
		heading += dimStyle.Render(fmt.Sprintf("   ·   %d already present", existing))
	}
	legend := dimStyle.Render("✓ new = created this run · existing = configuration already in the tenant")
	body := heading + "\n" + legend + "\n\n" + head + "\n" + strings.Join(shown, "\n") + footer
	return panel.Copy().Width(width-4).Padding(1, 2).Render(body)
}

// cellPlain pads a header label to a fixed display width without colour.
func cellPlain(s string, n int) string {
	return lipgloss.NewStyle().Width(n).Render(truncateCell(s, n))
}

// truncateCell shortens a value to at most n display cells, adding an ellipsis.
func truncateCell(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// componentType strips the display-name suffix from a "Type:Name" component key.
func componentType(component string) string {
	if i := strings.Index(component, ":"); i >= 0 {
		return component[:i]
	}
	return component
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

// focusedProviderRow is the default Deploy presentation: completed and waiting
// providers collapse to one line, while only the provider doing work expands.
// The animated gradient is intentionally shown without a numeric percentage;
// provider CLIs do not expose measurable completion and timer-derived numbers
// would imply precision Dispatch does not have.
func (m rootShellModel) focusedProviderRow(provider string, value float64, item *lifecycle.Item, active bool, phase string, width int) string {
	if item != nil && item.Status == lifecycle.StatusFailed {
		return m.providerSummaryLine(*item, width)
	}
	if !active {
		if value >= 1 {
			if item == nil {
				item = &lifecycle.Item{Provider: provider}
			}
			return m.providerSummaryLine(*item, width)
		}
		return dimStyle.Render("○  ") + titleStyle.Render(providerLabel(provider)) + dimStyle.Render("  WAITING")
	}
	bar := m.progress
	bar.ShowPercentage = false
	detail := "Working through provider configuration"
	if item != nil {
		count := len(item.Planned)
		if count == 0 {
			count = len(item.Missing) + len(item.Present)
		}
		if phase == "validate" {
			detail = "Checking provider state and prerequisites"
			if count > 0 {
				detail = fmt.Sprintf("Checking %d configuration component(s)", count)
			}
		} else {
			count = len(item.DeployableComponents)
			detail = "Applying supported missing configuration"
			if count > 0 {
				detail = fmt.Sprintf("Applying %d supported missing component(s)", count)
			}
		}
	}
	header := titleStyle.Render(providerLabel(provider)) + "  " + lipgloss.NewStyle().Bold(true).Foreground(gold).Render(m.spinner.View()+" IN PROGRESS")
	return header + "\n" + bar.ViewAs(value) + "\n" + dimStyle.Copy().Width(width).Render(detail)
}

// providerSummaryLine is the compact one-line provider state shown on the
// post-deploy screen in place of the full progress row, so the deployed-assets
// table has room and the whole screen stays on-screen. Failed providers keep
// their detail inline so a failure is never hidden by the compaction.
func (m rootShellModel) providerSummaryLine(item lifecycle.Item, width int) string {
	label := titleStyle.Render(providerLabel(item.Provider))
	if item.Status == lifecycle.StatusFailed {
		detail := dimStyle.Render(truncateCell(item.Detail, max(width-24, 10)))
		return lipgloss.NewStyle().Bold(true).Foreground(coral).Render("×  ") + label + "  " +
			lipgloss.NewStyle().Bold(true).Foreground(coral).Render("FAILED") + "   " + detail
	}
	return lipgloss.NewStyle().Bold(true).Foreground(green).Render("✓  ") + label + "  " +
		lipgloss.NewStyle().Foreground(green).Render("COMPLETE")
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
	slices.SortStableFunc(names, func(a, b string) int {
		aIndex, bIndex := slices.Index(order, a), slices.Index(order, b)
		switch {
		case aIndex >= 0 && bIndex >= 0:
			return aIndex - bIndex
		case aIndex >= 0:
			return -1
		case bIndex >= 0:
			return 1
		default:
			// Salesforce does not declare a Box-style component order. Its
			// categories must still be deterministic: map iteration order changes
			// between renders and previously made scrolling look like it wrapped.
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		}
	})
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
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("esc", "left"), key.WithHelp("esc/←", "back")),
	}
	if m.screen == screenConfig {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("tab", "up", "down"), key.WithHelp("tab/↑/↓", "change field")),
			key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "adjust")),
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
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	if m.screen == screenBoxComponents {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
			key.NewBinding(key.WithKeys("a", "n", "d", "c"), key.WithHelp("a/n/d/c", "modes")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	if m.screen == screenComponents {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "rows")),
			key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch panel")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next/continue")),
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select/toggle")),
			key.NewBinding(key.WithKeys("esc", "left"), key.WithHelp("esc/←", "back")),
		}
	}
	if m.screen == screenHistory {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "browse history")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review/reset")),
			key.NewBinding(key.WithKeys("left", "esc"), key.WithHelp("←/esc", "home")),
		}
	}
	if m.screen == screenDeploy && m.confirmingDeploy {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "switch")),
			key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	} else if m.screen == screenDeploy && !m.deployDone && m.deploymentPhase != deploymentPhaseReview {
		bindings = contextualHelp{}
		if m.deploymentPhase != deploymentPhasePackage {
			label := "details"
			if m.deployShowDetails {
				label = "summary"
			}
			bindings = append(bindings, key.NewBinding(key.WithKeys("v"), key.WithHelp("v", label)))
		}
		if len(m.activityLog) > 0 {
			bindings = append(bindings, key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "activity")))
		}
		if !m.taskRunning() {
			bindings = append(bindings, key.NewBinding(key.WithKeys("esc", "left"), key.WithHelp("esc/←", "review")))
		}
	} else if m.screen == screenDeploy && m.deployDone {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "details")),
		}
		if m.deployShowDetails {
			bindings = contextualHelp{
				key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "resources")),
				key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "summary")),
			}
		}
		bindings = append(bindings, key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "open Box"+m.postDeployBoxEnterpriseSuffix())))
		if alias, ok := m.postDeploySalesforceTarget(); ok {
			bindings = append(bindings, key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "open Salesforce ("+alias+")")))
		}
		enterHelp := "home"
		if m.deploymentAuditPath == "" {
			enterHelp = "retry audit"
		}
		bindings = append(bindings, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", enterHelp)))
	}
	if m.screen == screenBoxSwitch {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "set default")),
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "remove")),
			key.NewBinding(key.WithKeys("left", "esc"), key.WithHelp("←/esc", "back")),
		}
	}
	if m.screen == screenDiagnostic {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
			key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page")),
			key.NewBinding(key.WithKeys("esc", "left", "d"), key.WithHelp("esc/←/d", "back")),
		}
	} else if m.screen == screenHelp {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
			key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page")),
			key.NewBinding(key.WithKeys("?", "f1", "esc", "left", "q"), key.WithHelp("?/f1/esc", "close")),
		}
	} else if m.screen == screenScratchConfirm {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "switch")),
			key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	} else if m.screen == screenDevHubs {
		bindings = contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "choose hub")),
			key.NewBinding(key.WithKeys("enter", "right"), key.WithHelp("enter/→", "select")),
			key.NewBinding(key.WithKeys("esc", "left"), key.WithHelp("esc/←", "back")),
		}
	} else if (m.screen == screenProvider && m.providerDiagnostics[m.provider] != "") || ((m.screen == screenValidate || m.screen == screenDeploy) && func() bool { _, detail := m.lifecycleDiagnostic(); return detail != "" }()) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "full error")))
	}
	if m.screen == screenProvider && m.provider == "salesforce" {
		bindings = append(bindings, key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recheck")))
	}
	quitHelp := "home"
	if m.screen == screenWelcome {
		quitHelp = "quit"
	}
	if m.screen != screenHelp && !(m.screen == screenConfig && m.configFocus < 2) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("q"), key.WithHelp("q", quitHelp)))
	}
	if m.screen != screenHelp && !(m.screen == screenConfig && m.configFocus < 2) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?", "help")))
	}
	helpView := m.help.View(bindings)
	content := helpView
	// While the activity feed is live it owns the status area; showing m.message
	// too would duplicate a line at the bottom of the screen.
	if m.message != "" && !(m.taskRunning() && len(m.activityLog) > 0) {
		content = lipgloss.NewStyle().Foreground(gold).Render(m.message) + "\n" + helpView
	}
	// A hairline rule mirrors the header and anchors the help line to the frame.
	width := min(max(m.width-8, 64), 112)
	return lipgloss.NewStyle().Width(width).BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderBottom(false).BorderLeft(false).BorderRight(false).BorderForeground(divider).Render(content)
}

func providerLabel(provider string) string {
	switch provider {
	case "box":
		return "Box"
	case "salesforce":
		return "Salesforce"
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
	return m.stepper()
}
