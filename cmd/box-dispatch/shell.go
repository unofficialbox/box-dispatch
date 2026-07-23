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
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/solution"
	"github.com/unofficialbox/box-dispatch/internal/workspace"
)

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

func windlassHuhTheme() *huh.Theme {
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

	components := []componentChoice{
		{provider: "box", name: "Box", role: "Content, unstructured data, and AI", selected: true, required: true},
		{provider: "salesforce", name: "Salesforce + Agentforce", role: "Structured data, human experience, and agents"},
		{provider: "databricks", name: "Databricks", role: "Analytics, models, and data intelligence"},
		{provider: "aws", name: "AWS Bedrock AgentCore", role: "Agent runtime and orchestration"},
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
	cwd, _ := os.Getwd()
	answers := &wizardAnswers{
		components:         selectedComponents,
		templateID:         "clm",
		directory:          filepath.Dir(cwd),
		packageName:        "box-bedrock-for-clm",
		deploymentStrategy: solution.StrategyCreateNew,
	}
	m := rootShellModel{
		screen:     screenWelcome,
		components: components,
		templates: []templateChoice{
			{id: "clm", name: "Contract Lifecycle Management", sector: "LEGAL OPERATIONS", description: "Content-centric contract workflows with Box and intelligent agents.", repository: "https://github.com/unofficialbox/box-bedrock-for-clm"},
			{id: "lifesciences", name: "Life Sciences", sector: "REGULATED CONTENT", description: "Accelerate document-heavy life sciences processes and insight.", repository: "https://github.com/unofficialbox/box-bedrock-for-lifesciences"},
			{id: "citizen-services", name: "Citizen Services", sector: "PUBLIC SECTOR", description: "Modernize constituent intake, case content, and service delivery.", repository: "https://github.com/unofficialbox/box-bedrock-for-citizen-services"},
			{id: "new", name: "Create a New Solution", sector: "STARTER", description: "Begin with the Windlass reference architecture and shape your own solution.", repository: "https://github.com/unofficialbox/box-bedrock-template"},
		},
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

func newWindlassShell() rootShellModel { return newSetupOnlyShell() }

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
						return errors.New("Box is required for every Windlass solution")
					}
					return nil
				}),
		),
	).WithTheme(windlassHuhTheme()).WithShowHelp(false).WithWidth(76)
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
				Description("Start from a proven architecture or the Windlass solution template.").
				Options(options...).
				Value(&m.answers.templateID),
		),
	).WithTheme(windlassHuhTheme()).WithShowHelp(false).WithWidth(76)
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
		return errors.New("Box is required for every Windlass solution")
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
	description := "Windlass will deploy missing configuration to: " + strings.Join(providers, ", ") + ". Existing components will be skipped."
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
	).WithTheme(windlassHuhTheme()).WithShowHelp(false).WithWidth(76)
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
	return m, nil
}

func deploymentProgressCmd(provider string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return providerDeployProgressMsg{provider: provider}
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
		m.moveCursor(key, 2)
		if key.String() == "enter" || key.String() == "right" {
			if m.cursor == 0 {
				m.screen = screenComponents
				return m, m.componentForm.Init()
			}
			history, err := deploymentaudit.ListDeployments()
			m.deploymentHistory = history
			m.historyError = ""
			if err != nil {
				m.historyError = err.Error()
			}
			m.screen, m.cursor = screenHistory, 0
			return m, nil
		}
	case screenHistory:
		m.moveCursor(key, len(m.deploymentHistory))
		if key.String() == "left" || key.String() == "esc" {
			m.screen, m.cursor = screenWelcome, 1
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
			settings, _ := config.LoadConnectionSettings()
			settings.DatabricksHost = host
			if settings.DatabricksProfile == "" {
				settings.DatabricksProfile = "windlass"
			}
			_ = config.SaveConnectionSettings(settings)
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
	actions = append(actions, "connect", "back")
	return actions
}

func (m rootShellModel) saveProviderOption(value string) {
	settings, _ := config.LoadConnectionSettings()
	switch m.provider {
	case "salesforce":
		settings.SalesforceAlias = value
	case "databricks":
		settings.DatabricksProfile = value
	case "aws":
		settings.AWSProfile = value
	}
	_ = config.SaveConnectionSettings(settings)
}

func (m rootShellModel) savePlan() {
	if m.selected == nil {
		return
	}
	_ = config.SaveSolutionPlan(config.SolutionPlan{
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
	}
	return lipgloss.NewStyle().Margin(1, 3).Width(contentWidth).Render(m.header(contentWidth) + "\n\n" + body + "\n\n" + m.footer())
}

func (m rootShellModel) header(width int) string {
	mark := lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("B/")
	product := lipgloss.NewStyle().Bold(true).Foreground(white).Render("  WINDLASS")
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
	eyebrow := lipgloss.NewStyle().Bold(true).Foreground(coral).Render("BOX DEVELOPER COMMUNITY  /  EDITION 01")
	headline := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(white).Render("BUILD BEYOND"),
		lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("THE BOX."),
	)
	description := dimStyle.Copy().Width(58).Render("Assemble an industry solution from open building blocks, verify every connection, and deploy with a complete audit trail.")
	tags := lipgloss.NewStyle().Bold(true).Foreground(ice).Render("BOX  /  SALESFORCE  /  DATABRICKS  /  AGENTCORE")
	art := lipgloss.NewStyle().Foreground(coral).Render("🚢 ↓ ⚓ 🌊")
	copy := lipgloss.JoinVertical(lipgloss.Left, eyebrow, "", headline, art, "", description, "", tags)
	options := []string{"Start new deployment", "Show deployment history"}
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
	return titleStyle.Render("Deployment history") + "\n" + dimStyle.Render("Credential-free audit records exported by Windlass, newest first.") + "\n\n" +
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
		"check": "Check connection", "choose": "Choose authenticated profile", "connect": "Connect using provider CLI", "back": "Back to launch plan",
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
		marker := "[ ]"
		if enabled {
			marker = "[x]"
		}
		status := strings.ToUpper(capability.API)
		if capability.Handler != "" {
			status = "READY"
		} else if capability.API == "public" {
			status = "ADAPTER PENDING"
		}
		line := fmt.Sprintf("%s  %-25s %s", marker, capability.ComponentType, status)
		style := lipgloss.NewStyle().Width(width - 10).Foreground(ice)
		if i == m.cursor {
			style = style.Copy().Bold(true).Foreground(white).Background(lipgloss.Color("#12384A"))
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
	return m.stageHeader() + "\n\n" + titleStyle.Render("Choose the parent directory") + "\n" + dimStyle.Render("Navigate the folder tree, then choose where Windlass should create the package.") + "\n\n" +
		activePane.Copy().Padding(1, 2).Width(width-4).Render(content) + "\n" + controls
}

func (m rootShellModel) viewPackage(width int) string {
	status := m.spinner.View() + " PACKAGING"
	color := gold
	detail := "Cloning the selected quickstart and filtering provider-specific components."
	if m.packageDone {
		status, color = "● PACKAGE COMPLETE", green
		detail = "Created " + m.packagePath + "\nDetached upstream Git metadata and wrote .windlass/package.json."
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
		action = m.spinner.View() + " Deploying provider configuration..."
	} else if m.deployDone {
		action = lipgloss.NewStyle().Bold(true).Foreground(green).Render("Deployment run complete")
		if m.deploymentAuditPath == "" {
			action += "\n" + accent.Render("Enter / →  Export deployment audit log")
		} else {
			action += "\n" + lipgloss.NewStyle().Foreground(green).Render("✓ Deployment audit exported")
			action += "\n" + dimStyle.Render(m.deploymentAuditPath)
			action += "\n" + accent.Render("Enter / →  Return to Windlass home")
		}
	}
	return m.stageHeader() + "\n\n" + titleStyle.Render("Deploy missing configuration") + "\n" + dimStyle.Render("Windlass runs only native deploy adapters and leaves unsupported/manual work explicit.") + "\n\n" +
		panel.Copy().Width(width-4).Padding(1, 2).Render(strings.Join(rows, "\n\n")) + "\n" + action
}

func (m rootShellModel) providerProgressRow(provider string, value float64, item *lifecycle.Item, active bool, phase string, width int) string {
	state := dimStyle.Render("PENDING")
	if active {
		state = lipgloss.NewStyle().Bold(true).Foreground(gold).Render(m.spinner.View() + " IN PROGRESS")
	} else if value >= 1 {
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
		return "Salesforce + Agentforce"
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
