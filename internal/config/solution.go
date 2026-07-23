package config

// SolutionPlan is the non-secret FTUX selection saved for later commands.
// Persistence lives in internal/shellstate (BCL format); this package only owns
// the type.
type SolutionPlan struct {
	Components  []string `json:"components"`
	TemplateID  string   `json:"template_id"`
	Template    string   `json:"template"`
	Repository  string   `json:"repository"`
	PackagePath string   `json:"package_path,omitempty"`
}
