package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SolutionPlan is the non-secret FTUX selection saved for later commands.
type SolutionPlan struct {
	Components  []string `json:"components"`
	TemplateID  string   `json:"template_id"`
	Template    string   `json:"template"`
	Repository  string   `json:"repository"`
	PackagePath string   `json:"package_path,omitempty"`
}

func SaveSolutionPlan(plan SolutionPlan) error {
	if err := os.MkdirAll(".windlass", 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(".windlass", "solution.json"), data, 0o600)
}
