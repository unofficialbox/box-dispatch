package config

// SolutionPlan is the non-secret FTUX selection saved for later commands.
// Persistence lives in internal/shellstate (BCL format); this package only owns
// the type.
type SolutionPlan struct {
	Name        string   `json:"name,omitempty"`
	Components  []string `json:"components"`
	TemplateID  string   `json:"template_id"`
	Template    string   `json:"template"`
	Repository  string   `json:"repository"`
	Strategy    string   `json:"strategy,omitempty"`
	PackagePath string   `json:"package_path,omitempty"`
}
