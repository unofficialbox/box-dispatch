package webapi

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/solution"
	"github.com/unofficialbox/box-dispatch/internal/workspace"
)

func setPackageStrategy(plan config.SolutionPlan, strategy string, now time.Time) error {
	if strings.TrimSpace(plan.PackagePath) == "" {
		return nil
	}
	if _, err := os.Stat(plan.PackagePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	manifest, err := solution.Load(plan.PackagePath)
	if err != nil {
		return err
	}
	settings, err := solution.ReadDeploymentSettings(plan.PackagePath, manifest.DeploymentConfig)
	if err != nil {
		return err
	}
	settings.Box.GlobalStrategy = strategy
	if strategy == solution.StrategyCreateNew {
		settings.Box.RunID = now.UTC().Format("20060102T150405Z")
		if settings.Box.Naming.Suffix == "" {
			settings.Box.Naming.Suffix = "{run_id}"
		}
	} else {
		settings.Box.RunID = ""
	}
	return solution.WriteDeploymentSettings(plan.PackagePath, manifest.DeploymentConfig, settings)
}

// packageTemplate is the safe, browser-visible part of a configured solution
// source. The repository is resolved by the local service, never by a browser
// request.
type packageTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Sector      string `json:"sector,omitempty"`
	Description string `json:"description,omitempty"`
	repository  string
}

type templateStore func() ([]packageTemplate, error)
type packageAssembler func(packageTemplate, []string, string) (config.SolutionPlan, error)

type packageAssemblyRequest struct {
	TemplateID string   `json:"templateId"`
	Components []string `json:"components"`
	Strategy   string   `json:"strategy,omitempty"`
}

func (r packageAssemblyRequest) normalized() packageAssemblyRequest {
	r.TemplateID = strings.TrimSpace(r.TemplateID)
	r.Strategy = strings.ToLower(strings.TrimSpace(r.Strategy))
	if r.Strategy == "" {
		r.Strategy = solution.StrategyReuse
	}
	for i, component := range r.Components {
		r.Components[i] = strings.ToLower(strings.TrimSpace(component))
	}
	return r
}

func (r packageAssemblyRequest) validate() error {
	if _, err := solution.NormalizeDeploymentStrategy(r.Strategy); err != nil || r.Strategy == solution.StrategyInherit {
		return fmt.Errorf("choose reuse existing or create new")
	}
	if r.TemplateID == "" {
		return fmt.Errorf("choose a solution quickstart")
	}
	if len(r.Components) == 0 || !slices.Contains(r.Components, "box") {
		return fmt.Errorf("Box is required")
	}
	seen := map[string]bool{}
	for _, component := range r.Components {
		if component != "box" && component != "salesforce" {
			return fmt.Errorf("%q is not available for browser planning", component)
		}
		if seen[component] {
			return fmt.Errorf("%q is selected more than once", component)
		}
		seen[component] = true
	}
	return nil
}

func loadPackageTemplates() ([]packageTemplate, error) {
	runtime, err := config.LoadRuntimeConfig()
	if err == nil && len(runtime.Scenarios) > 0 {
		fallback := templateFallbacks()
		ids := make([]string, 0, len(runtime.Scenarios))
		for id := range runtime.Scenarios {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			switch {
			case ids[i] == runtime.ActiveScenario:
				return true
			case ids[j] == runtime.ActiveScenario:
				return false
			default:
				return ids[i] < ids[j]
			}
		})
		templates := make([]packageTemplate, 0, len(ids)+1)
		for _, id := range ids {
			scenario := runtime.Scenarios[id]
			fallbackTemplate := fallback[id]
			templates = append(templates, packageTemplate{
				ID: id, Name: firstTemplateValue(scenario.DisplayName, fallbackTemplate.Name, id),
				Sector:      firstTemplateValue(scenario.Sector, fallbackTemplate.Sector),
				Description: firstTemplateValue(scenario.Description, fallbackTemplate.Description),
				repository:  firstTemplateValue(scenario.Repository, fallbackTemplate.repository),
			})
		}
		return append(templates, newSolutionTemplate()), nil
	}
	return append(defaultPackageTemplates(), newSolutionTemplate()), nil
}

func firstTemplateValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func templateFallbacks() map[string]packageTemplate {
	templates := map[string]packageTemplate{}
	for _, template := range defaultPackageTemplates() {
		templates[template.ID] = template
	}
	return templates
}

func defaultPackageTemplates() []packageTemplate {
	return []packageTemplate{
		{ID: "clm", Name: "Contract Lifecycle Management", Sector: "LEGAL OPERATIONS", Description: "Content-centric contract workflows with Box and intelligent agents.", repository: "https://github.com/unofficialbox/box-bedrock-for-clm"},
		{ID: "lifesciences", Name: "Life Sciences", Sector: "REGULATED CONTENT", Description: "Accelerate document-heavy life sciences processes and insight.", repository: "https://github.com/unofficialbox/box-bedrock-for-lifesciences"},
		{ID: "citizen-services", Name: "Citizen Services", Sector: "PUBLIC SECTOR", Description: "Modernize constituent intake, case content, and service delivery.", repository: "https://github.com/unofficialbox/box-bedrock-for-citizen-services"},
	}
}

func newSolutionTemplate() packageTemplate {
	return packageTemplate{ID: "new", Name: "Create a New Solution", Sector: "STARTER", Description: "Begin with the Box Dispatch reference architecture and shape your own solution.", repository: "https://github.com/unofficialbox/box-bedrock-template"}
}

func assemblePackage(template packageTemplate, components []string, strategy string) (config.SolutionPlan, error) {
	if strings.TrimSpace(template.repository) == "" {
		return config.SolutionPlan{}, fmt.Errorf("the selected quickstart has no configured source repository")
	}
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	destination := filepath.Join(config.Paths().Root, ".box-dispatch", "web-packages", template.ID+"-"+stamp)
	if _, err := workspace.Build(workspace.PackageRequest{
		Repository: template.repository, Destination: destination, TemplateID: template.ID,
		Components: components, BoxStrategy: strategy,
	}); err != nil {
		return config.SolutionPlan{}, err
	}
	return config.SolutionPlan{
		TemplateID: template.ID, Template: template.Name, Repository: template.repository,
		Components: append([]string(nil), components...), Strategy: strategy, PackagePath: destination,
	}, nil
}
