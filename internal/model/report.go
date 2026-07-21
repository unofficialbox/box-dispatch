package model

import "time"

type PhaseStatus string

const (
	PhasePassed  PhaseStatus = "passed"
	PhaseSkipped PhaseStatus = "skipped"
	PhaseBlocked PhaseStatus = "blocked"
	PhaseFailed  PhaseStatus = "failed"
	PhaseWarn    PhaseStatus = "warn"
)

type Phase struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int    `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type CommandReport struct {
	Command         string                 `json:"command"`
	Scenario        string                 `json:"scenario,omitempty"`
	DryRun          bool                   `json:"dryRun"`
	ConfirmRequired bool                   `json:"confirmRequired"`
	Phases          []Phase                `json:"phases"`
	Manual          []string               `json:"manual"`
	Warnings        []string               `json:"warnings"`
	Validation      map[string]interface{} `json:"validation"`
	Status          string                 `json:"status"`
	NextCommand     string                 `json:"nextCommand,omitempty"`
	StartedAt       string                 `json:"startedAt,omitempty"`
}

func NewReport(command string) CommandReport {
	return CommandReport{
		Command:    command,
		Phases:     []Phase{},
		Manual:     []string{},
		Warnings:   []string{},
		Validation: map[string]interface{}{},
		Status:     string(PhasePassed),
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

func (r *CommandReport) AddPhase(name, status string, dur time.Duration, err error) {
	p := Phase{
		Name:       name,
		Status:     status,
		DurationMs: int(dur.Milliseconds()),
	}
	if err != nil {
		p.Error = err.Error()
		if status == string(PhasePassed) {
			p.Status = string(PhaseFailed)
		}
	}
	r.Phases = append(r.Phases, p)

	if status == string(PhaseBlocked) {
		r.Status = string(PhaseBlocked)
	} else if status == string(PhaseFailed) && r.Status != string(PhaseBlocked) {
		r.Status = string(PhaseFailed)
	} else if status == string(PhaseWarn) && r.Status == string(PhasePassed) {
		r.Status = string(PhaseWarn)
	}
}

func (r *CommandReport) AddManual(item string) {
	for _, m := range r.Manual {
		if m == item {
			return
		}
	}
	r.Manual = append(r.Manual, item)
}
