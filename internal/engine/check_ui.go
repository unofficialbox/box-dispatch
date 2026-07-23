package engine

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"github.com/unofficialbox/box-dispatch/internal/providers"
)

type checkUIResultMsg struct {
	provider string
	started  time.Time
	result   ProviderCheckResult
	spec     providers.ProviderSpec
}

type checkUITickMsg time.Time

type checkUIModel struct {
	engine         *Engine
	cfg            *config.RuntimeConfig
	scenario       string
	platform       string
	offline        bool
	providers      []string
	providerStatus map[string]string
	providerDetail map[string]string
	report         *model.CommandReport
	spinnerIndex   int
	current        int
	running        bool
	done           bool
}

func newCheckUI(e *Engine, cfg *config.RuntimeConfig, scenario, platform string, offline bool, report *model.CommandReport) *checkUIModel {
	keys := cfg.Scenarios[scenario].Providers
	selected := []string{}
	for _, key := range keys {
		spec, ok := providers.Specs[key]
		if !ok {
			continue
		}
		if platform != "all" && platform != spec.Platform && platform != key {
			continue
		}
		selected = append(selected, key)
	}

	states := map[string]string{}
	details := map[string]string{}
	for _, key := range selected {
		states[key] = "pending"
		details[key] = "waiting"
	}
	return &checkUIModel{
		engine:         e,
		cfg:            cfg,
		scenario:       scenario,
		platform:       platform,
		offline:        offline,
		providers:      selected,
		providerStatus: states,
		providerDetail: details,
		report:         report,
		running:        len(selected) > 0,
	}
}

func (m checkUIModel) Init() tea.Cmd {
	if !m.running {
		m.done = true
		return nil
	}
	return tea.Batch(m.nextProvider(), m.tick())
}

func (m checkUIModel) Run() (*model.CommandReport, error) {
	program := tea.NewProgram(m)
	rawModel, err := program.Run()
	if err != nil {
		return m.report, err
	}
	finalModel, ok := rawModel.(checkUIModel)
	if !ok {
		return m.report, nil
	}
	return finalModel.report, nil
}

func (m checkUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case checkUITickMsg:
		m.spinnerIndex = (m.spinnerIndex + 1) % len(checkUISpinnerFrames())
		if m.done {
			return m, nil
		}
		return m, m.tick()
	case checkUIResultMsg:
		provider := msg.provider
		providerSpec := msg.spec
		m.providerStatus[provider] = "completed"
		detail := summarizeProviderResult(msg.result, msg.provider)
		m.providerDetail[provider] = detail
		m.current++
		phaseStart := msg.started
		m.engine.appendDependencyPhases(m.report, "check", provider, providerSpec, msg.result, phaseStart, m.offline)

		if m.current >= len(m.providers) {
			m.running = false
			m.done = true
			return m, nil
		}
		return m, m.nextProvider()
	case error:
		m.report.AddPhase("check.ui", string(model.PhaseFailed), 0, msg)
		m.running = false
		m.done = true
		return m, nil
	default:
		return m, nil
	}
}

func (m checkUIModel) View() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "box-dispatch check (%s)\n", m.scenario)
	if m.done {
		_, _ = fmt.Fprintln(&b, "status:", m.report.Status)
	} else {
		_, _ = fmt.Fprintln(&b, "status: running")
	}

	if len(m.providers) == 0 {
		_, _ = fmt.Fprintln(&b, "No providers matched this check.")
		return b.String()
	}

	_, _ = fmt.Fprintln(&b, "")
	for idx, provider := range m.providers {
		status := m.providerStatus[provider]
		icon := " "
		if status == "pending" {
			icon = "⋯"
		}
		if status == "running" {
			icon = checkUISpinnerFrames()[m.spinnerIndex]
		}
		if status == "completed" {
			switch {
			case strings.Contains(m.providerDetail[provider], "✓"):
				icon = "✓"
			case strings.Contains(m.providerDetail[provider], "blocked"):
				icon = "⚠"
			case strings.Contains(m.providerDetail[provider], "warn"):
				icon = "!"
			default:
				icon = "•"
			}
		}
		_, _ = fmt.Fprintf(&b, "  %s %s", icon, provider)
		if status == "running" {
			_, _ = fmt.Fprintf(&b, " (%s)", m.platform)
		}
		if idx == m.current && m.running && status != "completed" {
			_, _ = fmt.Fprintf(&b, "  %s", m.providerDetail[provider])
		} else if detail, ok := m.providerDetail[provider]; ok {
			_, _ = fmt.Fprintf(&b, "  %s", detail)
		}
		_, _ = fmt.Fprintln(&b)
	}

	if m.done && len(m.report.Manual) > 0 {
		_, _ = fmt.Fprintln(&b, "")
		_, _ = fmt.Fprintln(&b, "connectivity guidance:")
		for _, item := range m.report.Manual {
			if strings.HasSuffix(item, ".tools") {
				provider := strings.TrimSuffix(item, ".tools")
				_, _ = fmt.Fprintf(&b, "  - %s: install provider tool\n", provider)
				if spec, ok := providers.Specs[provider]; ok {
					_, _ = fmt.Fprintf(&b, "      required: %s\n", strings.Join(spec.RequiredTools, ", "))
				}
				continue
			}
			if strings.HasSuffix(item, ".environment") {
				provider := strings.TrimSuffix(item, ".environment")
				_, _ = fmt.Fprintf(&b, "  - %s: set required environment values\n", provider)
				if spec, ok := providers.Specs[provider]; ok {
					_, _ = fmt.Fprintf(&b, "      required: %s\n", strings.Join(spec.RequiredTokens, ", "))
				}
				continue
			}
			if strings.HasSuffix(item, ".connect") {
				provider := strings.TrimSuffix(item, ".connect")
				_, _ = fmt.Fprintf(&b, "  - %s\n", provider)
				if guidance, ok := m.report.Validation["check."+provider+".connectGuidance"]; ok {
					if values, ok := guidance.([]string); ok {
						for _, line := range values {
							_, _ = fmt.Fprintf(&b, "      %s\n", line)
						}
					}
				}
				continue
			}
		}
	}
	return b.String()
}

func (m checkUIModel) tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time time.Time) tea.Msg {
		return checkUITickMsg(time)
	})
}

func (m checkUIModel) nextProvider() tea.Cmd {
	idx := m.current
	if idx >= len(m.providers) {
		return nil
	}
	provider := m.providers[idx]
	m.providerStatus[provider] = "running"
	m.providerDetail[provider] = "checking..."
	spec, ok := providers.Specs[provider]
	if !ok {
		return func() tea.Msg {
			return checkUIResultMsg{
				provider: provider,
				started:  time.Now(),
				spec:     spec,
				result: ProviderCheckResult{
					Provider: provider,
				},
			}
		}
	}

	return func() tea.Msg {
		started := time.Now()
		result := m.engine.checkProvider(m.scenario, provider, spec, m.cfg, m.offline)
		return checkUIResultMsg{
			provider: provider,
			started:  started,
			spec:     spec,
			result:   result,
		}
	}
}

func checkUISpinnerFrames() []string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}

func summarizeProviderResult(result ProviderCheckResult, provider string) string {
	if result.ConfigMissing {
		return fmt.Sprintf("%s is missing config", provider)
	}
	if len(result.ToolsMissing) > 0 {
		return "warn: missing tools"
	}
	if len(result.MissingTokens) > 0 {
		return "blocked: missing credentials"
	}
	if result.ConnectivityError != nil {
		return "blocked: not connected"
	}
	return "✓ connected"
}
