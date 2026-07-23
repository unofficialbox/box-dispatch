package checker

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type checkModel struct {
	cfg       CheckConfig
	results   []ProviderResult
	pos       int
	done      bool
	spinner   int
	lastEvent string
}

type CheckModel struct {
	checkModel
}

func NewInteractiveCheckModel(cfg CheckConfig) *CheckModel {
	return &CheckModel{
		checkModel: checkModel{
			cfg:     cfg,
			results: make([]ProviderResult, 0, len(cfg.Providers)),
		},
	}
}

type tickMsg struct{ time time.Time }
type startMsg struct{}
type providerDoneMsg struct {
	result ProviderResult
}

func (m checkModel) Init() tea.Cmd {
	return tea.Batch(tickCmd(), func() tea.Msg { return startMsg{} })
}

func (m checkModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.spinner = (m.spinner + 1) % 4
		if m.done {
			return m, nil
		}
		return m, tickCmd()

	case startMsg:
		if m.pos >= len(m.cfg.Providers) {
			m.done = true
			return m, nil
		}
		name := m.cfg.Providers[m.pos]
		m.lastEvent = "checking " + name
		return m, runProviderCmd(name, m.cfg.Offline, m.cfg.Scenario, m.cfg.Platform)

	case providerDoneMsg:
		if len(m.results) <= m.pos {
			m.results = append(m.results, msg.result)
		} else {
			m.results[m.pos] = msg.result
		}
		m.pos++
		if m.pos >= len(m.cfg.Providers) {
			m.done = true
			m.lastEvent = "all checks complete"
			return m, nil
		}
		return m, runProviderCmd(m.cfg.Providers[m.pos], m.cfg.Offline, m.cfg.Scenario, m.cfg.Platform)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m checkModel) View() string {
	logo := `Box Dispatch
   ____      _         _ _      
  / __ \____(_)____   (_) |___  
 / / / / __/ / __ \ / / / __ \ 
/ /_/ / /_/ / / / // / / / / / 
\____/\__/_/_/ /_//_/_/_/ /_/  
`

	header := logo + "\nConnectivity Check\n----------------\n"
	var b strings.Builder
	b.WriteString(header)

	spinner := []string{"◴", "◷", "◶", "◵"}
	for idx, name := range m.cfg.Providers {
		icon := "·"
		switch {
		case idx < m.pos:
			r := m.results[idx]
			switch {
			case r.Blocked:
				icon = "✗"
			case r.RequiresAttention:
				icon = "!"
			default:
				icon = "✓"
			}
		case idx == m.pos && !m.done:
			icon = spinner[m.spinner%len(spinner)]
		}
		b.WriteString(fmt.Sprintf(" %s %s\n", icon, strings.ToUpper(name)))
	}

	if m.lastEvent != "" {
		b.WriteString("\n")
		b.WriteString(" → ")
		b.WriteString(m.lastEvent)
	}

	if m.done {
		b.WriteString("\n\nGuidance:\n")
		for _, r := range m.results {
			if !r.Blocked && !r.RequiresAttention {
				continue
			}
			b.WriteString("\n- ")
			b.WriteString(strings.ToUpper(r.Name))
			b.WriteString("\n")
			for _, g := range r.Guidance {
				b.WriteString("  • ")
				b.WriteString(g)
				b.WriteString("\n")
			}
		}
		if allProvidersHealthy(m.results) {
			b.WriteString("\nAll providers are healthy.\n")
		} else {
			b.WriteString("\nSome providers need attention. Complete steps above and rerun `box-dispatch check`.\n")
		}
	}
	return b.String()
}

func (m checkModel) Report() CheckReport {
	return CheckReport{
		GeneratedAt: time.Now(),
		Scenario:    m.cfg.Scenario,
		Platform:    m.cfg.Platform,
		Offline:     m.cfg.Offline,
		Providers:   append([]ProviderResult{}, m.results...),
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{time: t}
	})
}

func runProviderCmd(name string, offline bool, scenario, platform string) tea.Cmd {
	cfg := CheckConfig{Scenario: scenario, Platform: platform, Offline: offline, Providers: []string{name}}
	return func() tea.Msg {
		report, err := Check(cfg)
		if err != nil || len(report.Providers) == 0 {
			return providerDoneMsg{result: ProviderResult{
				Name:              name,
				Checks:            []string{fmt.Sprintf("check failed: %v", err)},
				Blocked:           true,
				RequiresAttention: true,
			}}
		}
		return providerDoneMsg{result: report.Providers[0]}
	}
}

func allProvidersHealthy(results []ProviderResult) bool {
	for _, r := range results {
		if r.Blocked || r.RequiresAttention {
			return false
		}
	}
	return true
}
