package webapi

import (
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

func loadPersistedConnections() (config.ConnectionSettings, error) {
	return shellstate.LoadConnectionSettings()
}

func loadPlan() (config.SolutionPlan, error) {
	return shellstate.LoadSolutionPlan()
}

func savePlan(plan config.SolutionPlan) error {
	return shellstate.SaveSolutionPlan(plan)
}
