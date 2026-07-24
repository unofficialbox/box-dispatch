package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

// TeardownOutcome is the result of attempting to delete one recorded resource.
type TeardownOutcome struct {
	Resource ResourceReference `json:"resource"`
	Deleted  bool              `json:"deleted"`
	// Unmanaged marks a resource kind box-dispatch cannot delete (no API), so it
	// was reported rather than attempted.
	Unmanaged bool   `json:"unmanaged,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TeardownResult reports what a destroy pass did for a single provider.
type TeardownResult struct {
	Provider string            `json:"provider"`
	Outcomes []TeardownOutcome `json:"outcomes"`
	Detail   string            `json:"detail"`
}

// Deleted counts resources this pass actually removed.
func (r TeardownResult) Deleted() int {
	count := 0
	for _, outcome := range r.Outcomes {
		if outcome.Deleted {
			count++
		}
	}
	return count
}

// Remaining lists resources that were not removed, whether they failed or are
// unmanaged, so the caller can tell the operator what still needs attention.
func (r TeardownResult) Remaining() []TeardownOutcome {
	remaining := []TeardownOutcome{}
	for _, outcome := range r.Outcomes {
		if !outcome.Deleted {
			remaining = append(remaining, outcome)
		}
	}
	return remaining
}

// teardownKindOrder deletes leaves before their containers: the private
// browser surfaces first, then enterprise-level objects, then files, and the
// folder tree last (folder deletes are recursive, so the workspace goes last).
var teardownKindOrder = []string{
	"form",
	"app",
	"docgen_template",
	"ai_agent",
	"hub",
	"metadata_template",
	"file",
	"folder",
}

// orderResourcesForTeardown sorts recorded resources into a safe delete order.
// Within the folder kind, deeper folders are deleted first so a recursive
// workspace delete never orphans a pending child delete.
func orderResourcesForTeardown(resources []ResourceReference) []ResourceReference {
	ordered := append([]ResourceReference(nil), resources...)
	rank := func(kind string) int {
		if index := slices.Index(teardownKindOrder, kind); index >= 0 {
			return index
		}
		return len(teardownKindOrder)
	}
	slices.SortStableFunc(ordered, func(a, b ResourceReference) int {
		return rank(a.Kind) - rank(b.Kind)
	})
	return ordered
}

// boxContext carries the shared setup a Box deploy or teardown needs.
type boxContext struct {
	manifest  solution.Manifest
	settings  solution.DeploymentSettings
	selection solution.ComponentSelection
	target    boxTarget
	api       boxAPI
	ctx       context.Context
}

// loadBoxContext resolves the manifest, deployment settings, Box target and API
// client. The workspace name is resolved through the deployment naming strategy
// so deploy and teardown address the same folder.
func loadBoxContext(root string) (boxContext, error) {
	manifest, err := solution.Load(root)
	if err != nil {
		return boxContext{}, err
	}
	settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
	if err != nil {
		return boxContext{}, err
	}
	target, err := loadBoxTarget(root, manifest.Box.Workspace.Name)
	if err != nil {
		return boxContext{}, err
	}
	resolvedWorkspaceName, err := solution.ResolveDeploymentName(target.WorkspaceName, settings.Box)
	if err != nil {
		return boxContext{}, err
	}
	target.WorkspaceName = resolvedWorkspaceName
	api, err := newBoxAPI()
	if err != nil {
		return boxContext{}, err
	}
	if target.ParentFolderID == "" {
		target.ParentFolderID = "0"
	}
	return boxContext{
		manifest:  manifest,
		settings:  settings,
		selection: settings.Box.Components,
		target:    target,
		api:       api,
		ctx:       context.Background(),
	}, nil
}

// DestroyProvider deletes the resources a previous deployment recorded for a
// provider. It deletes strictly by recorded ID, never by name, so it can only
// remove what box-dispatch created. Failures are collected per resource rather
// than aborting, so one stuck resource cannot strand the rest of the reset.
func DestroyProvider(root, provider string, resources []ResourceReference) (TeardownResult, error) {
	result := TeardownResult{Provider: provider}
	scoped := []ResourceReference{}
	for _, resource := range resources {
		if resource.Provider == provider {
			scoped = append(scoped, resource)
		}
	}
	if len(scoped) == 0 {
		result.Detail = "No recorded resources to remove."
		return result, nil
	}
	switch provider {
	case "box":
		return destroyBoxResources(root, scoped)
	case "salesforce":
		return destroySalesforceMetadata(root, scoped)
	default:
		for _, resource := range scoped {
			result.Outcomes = append(result.Outcomes, TeardownOutcome{Resource: resource, Unmanaged: true})
		}
		result.Detail = fmt.Sprintf("%s teardown is not supported; %d recorded resources need manual removal.", provider, len(scoped))
		return result, nil
	}
}

func destroyBoxResources(root string, resources []ResourceReference) (TeardownResult, error) {
	result := TeardownResult{Provider: "box"}
	box, err := loadBoxContext(root)
	if err != nil {
		return result, err
	}

	// Private surfaces have no delete API and are removed through the
	// authenticated browser in one pass before the API-backed resources.
	privateOutcomes, privateHandled := destroyBoxPrivateSurfaces(root, box, resources)
	result.Outcomes = append(result.Outcomes, privateOutcomes...)

	for _, resource := range orderResourcesForTeardown(resources) {
		if privateHandled[resourceKey(resource)] {
			continue
		}
		result.Outcomes = append(result.Outcomes, deleteBoxResource(box, resource))
	}
	result.Detail = teardownDetail(result)
	return result, nil
}

// deleteBoxResource removes a single recorded Box resource by ID.
func deleteBoxResource(box boxContext, resource ResourceReference) TeardownOutcome {
	outcome := TeardownOutcome{Resource: resource}
	id := strings.TrimSpace(resource.ID)
	if id == "" {
		outcome.Error = "no recorded id"
		return outcome
	}
	// Never delete the Box root; mirrors the deploy-side root folder guard.
	if resource.Kind == "folder" && id == "0" {
		outcome.Error = "refusing to delete the Box root folder"
		return outcome
	}

	var err error
	switch resource.Kind {
	case "folder":
		err = box.api.deleteFolder(box.ctx, id)
	case "file":
		err = box.api.deleteFile(box.ctx, id)
	case "metadata_template":
		err = box.api.deleteMetadataTemplate(box.ctx, id)
	case "docgen_template":
		err = box.api.deleteDocgenTemplate(box.ctx, id)
	case "ai_agent":
		err = box.api.deleteAIAgent(box.ctx, id)
	case "hub":
		err = box.api.deleteHub(box.ctx, id)
	default:
		// Automate workflows and anything else without a delete API.
		outcome.Unmanaged = true
		return outcome
	}
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	outcome.Deleted = true
	return outcome
}

// destroySalesforceMetadata removes the metadata a deployment pushed by sending
// a destructive-changes package built from the same packaged source. Salesforce
// has no per-resource delete, so the whole packaged metadata set is retired in
// one deploy.
func destroySalesforceMetadata(root string, resources []ResourceReference) (TeardownResult, error) {
	result := TeardownResult{Provider: "salesforce"}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(settings.SalesforceAlias) == "" {
		result.Detail = "No Salesforce alias is selected; nothing was removed."
		for _, resource := range resources {
			result.Outcomes = append(result.Outcomes, TeardownOutcome{Resource: resource, Error: "no Salesforce alias selected"})
		}
		return result, nil
	}
	project := findSalesforceProject(root)
	if project == "" {
		result.Detail = "No Salesforce project was found in the package; nothing was removed."
		for _, resource := range resources {
			result.Outcomes = append(result.Outcomes, TeardownOutcome{Resource: resource, Error: "no Salesforce project in package"})
		}
		return result, nil
	}

	temp, err := os.MkdirTemp("", "box-dispatch-salesforce-destroy-")
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(temp) }()

	// `--type destroy` renders the packaged source as destructiveChanges.xml.
	generate := exec.Command("sf", "project", "generate", "manifest", "--source-dir", "force-app", "--type", "destroy", "--output-dir", temp, "--json")
	generate.Dir = project
	if output, runErr := generate.CombinedOutput(); runErr != nil {
		return result, fmt.Errorf("inventory Salesforce metadata for removal: %s", summarizeSalesforceError(output, runErr))
	}
	destructive := filepath.Join(temp, "destructiveChanges.xml")
	if _, statErr := os.Stat(destructive); statErr != nil {
		return result, fmt.Errorf("destructive manifest was not generated at %s", destructive)
	}
	// A destructive deploy still needs a package.xml; an empty one removes only
	// what destructiveChanges lists.
	emptyPackage := filepath.Join(temp, "package.xml")
	if writeErr := os.WriteFile(emptyPackage, []byte(emptySalesforcePackage), 0o600); writeErr != nil {
		return result, writeErr
	}

	deploy := exec.Command("sf", "project", "deploy", "start",
		"--manifest", emptyPackage,
		"--post-destructive-changes", destructive,
		"--target-org", settings.SalesforceAlias,
		"--ignore-warnings", "--json")
	deploy.Dir = project
	output, runErr := deploy.CombinedOutput()
	if runErr != nil {
		detail := summarizeSalesforceError(output, runErr)
		for _, resource := range resources {
			result.Outcomes = append(result.Outcomes, TeardownOutcome{Resource: resource, Error: detail})
		}
		result.Detail = "Salesforce metadata removal failed: " + detail
		return result, nil
	}
	for _, resource := range resources {
		result.Outcomes = append(result.Outcomes, TeardownOutcome{Resource: resource, Deleted: true})
	}
	result.Detail = "Removed the packaged Salesforce metadata."
	return result, nil
}

const emptySalesforcePackage = `<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <version>62.0</version>
</Package>
`

func resourceKey(resource ResourceReference) string {
	return resource.Kind + ":" + resource.ID
}

func teardownDetail(result TeardownResult) string {
	remaining := result.Remaining()
	if len(remaining) == 0 {
		return fmt.Sprintf("Removed %d resources.", result.Deleted())
	}
	return fmt.Sprintf("Removed %d resources; %d could not be removed.", result.Deleted(), len(remaining))
}
