package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusPresent Status = "present"
	StatusMissing Status = "needs deployment"
	StatusManual  Status = "manual action"
	StatusFailed  Status = "failed"
)

type Item struct {
	Provider             string              `json:"provider"`
	Name                 string              `json:"name"`
	Source               string              `json:"source"`
	Status               Status              `json:"status"`
	Detail               string              `json:"detail"`
	Diagnostic           string              `json:"diagnostic,omitempty"`
	Deployable           bool                `json:"deployable"`
	DeployableComponents []string            `json:"deployable_components,omitempty"`
	AdapterPending       []string            `json:"adapter_pending,omitempty"`
	Experimental         []string            `json:"experimental,omitempty"`
	ComponentOrder       []string            `json:"component_order,omitempty"`
	Present              []string            `json:"present,omitempty"`
	Missing              []string            `json:"missing,omitempty"`
	Planned              []string            `json:"planned,omitempty"`
	Resources            []ResourceReference `json:"resources,omitempty"`
}

type ResourceReference struct {
	Provider  string `json:"provider"`
	Component string `json:"component"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	URL       string `json:"url,omitempty"`
}

func addResource(item *Item, component, kind, name, id, resourceURL string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	for _, resource := range item.Resources {
		if resource.Provider == item.Provider && resource.Kind == kind && resource.ID == id {
			return
		}
	}
	item.Resources = append(item.Resources, ResourceReference{
		Provider: item.Provider, Component: component, Kind: kind,
		Name: name, ID: id, URL: resourceURL,
	})
}

type ProgressState string

const (
	ProgressActivity  ProgressState = "activity"
	ProgressQueued    ProgressState = "queued"
	ProgressRunning   ProgressState = "running"
	ProgressCompleted ProgressState = "completed"
	ProgressFailed    ProgressState = "failed"
)

// ProgressUpdate describes either a provider-level activity message or the
// state of one packaged component. Component updates let browser clients show
// every validation item without parsing human-readable log lines.
type ProgressUpdate struct {
	Message   string
	Component string
	State     ProgressState
	Current   int
	Total     int
}

// Reporter receives structured progress from a long-running lifecycle
// operation. The nil Reporter is safe and simply discards updates.
type Reporter func(update ProgressUpdate)

func (r Reporter) step(message string) {
	if r != nil {
		r(ProgressUpdate{Message: message, State: ProgressActivity})
	}
}

func (r Reporter) component(component string, state ProgressState, message string, current, total int) {
	if r != nil {
		r(ProgressUpdate{Message: message, Component: component, State: state, Current: current, Total: total})
	}
}

type receiptFile struct {
	Receipts []struct {
		Platform string `json:"platform"`
		Status   string `json:"status"`
	} `json:"receipts"`
}

func Validate(root string, components []string) ([]Item, error) {
	if root == "" {
		return nil, fmt.Errorf("package directory is required")
	}
	items := make([]Item, 0, len(components))
	for _, provider := range components {
		item, err := ValidateProvider(root, provider, nil)
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

func ValidateProvider(root, provider string, report Reporter) (Item, error) {
	if root == "" {
		return Item{}, fmt.Errorf("package directory is required")
	}
	report.step("Reading packaged " + providerName(provider) + " configuration")
	paths := providerPaths(root, provider)
	count := countFiles(paths)
	item := Item{Provider: provider, Name: providerName(provider), Source: strings.Join(relativePaths(root, paths), ", ")}
	if count == 0 {
		item.Status, item.Detail = StatusManual, "No provider-specific package assets were found."
		return item, nil
	}
	if readReceipts(root)[provider] {
		item.Status, item.Detail = StatusPresent, fmt.Sprintf("Validated receipt found; %d packaged files inspected.", count)
		item.Present = []string{"Validated provider receipt"}
		return item, nil
	}
	components, componentErr := providerComponentEntries(root, provider, count)
	if componentErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to read packaged provider configuration: "+componentErr.Error()
		return item, nil
	}
	if provider == "salesforce" {
		return validateSalesforce(root, item, report)
	}
	if provider == "box" {
		report.step("Inspecting existing Box configuration in the tenant")
		return validateBox(root, item, components, report)
	}
	item.Status = StatusManual
	item.Detail = fmt.Sprintf("%d packaged configuration files require provider validation. A native Box Dispatch deploy adapter is not available yet.", count)
	item.Missing = components
	return item, nil
}

func PlanProvider(root, provider string) (Item, error) {
	if root == "" {
		return Item{}, fmt.Errorf("package directory is required")
	}
	paths := providerPaths(root, provider)
	count := countFiles(paths)
	item := Item{
		Provider: provider,
		Name:     providerName(provider),
		Source:   strings.Join(relativePaths(root, paths), ", "),
		Status:   StatusPending,
		Detail:   "Waiting to validate packaged configuration.",
	}
	if provider == "salesforce" {
		manifest, err := solution.Load(root)
		if err != nil {
			return item, err
		}
		item.ComponentOrder = []string{"Managed Package"}
		for _, requirement := range manifest.Salesforce.RequiredPackages {
			item.Planned = append(item.Planned, salesforcePackageComponent(requirement))
		}
		for _, requirement := range manifest.Salesforce.RequiredPermissionSets {
			item.Planned = append(item.Planned, salesforcePermissionSetComponent(requirement))
		}
	}
	if count == 0 {
		if len(item.Planned) == 0 {
			item.Planned = []string{"Provider configuration"}
		}
		return item, nil
	}
	components, err := providerComponentEntries(root, provider, count)
	if err != nil {
		return item, err
	}
	item.Planned = append(item.Planned, components...)
	if provider == "box" {
		manifest, err := solution.Load(root)
		if err != nil {
			return item, err
		}
		item.ComponentOrder = append([]string(nil), manifest.Box.ComponentOrder...)
	}
	return item, nil
}

func providerComponentEntries(root, provider string, count int) ([]string, error) {
	if provider == "box" {
		manifest, err := solution.Load(root)
		if err != nil {
			return nil, err
		}
		settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
		if err != nil {
			return nil, err
		}
		entries, err := boxComponentEntries(filepath.Join(root, "config", "box"), manifest, settings.Box.Components)
		if err != nil {
			return nil, err
		}
		if manifest.CapabilityEnabled("Sample Content", settings.Box.Components) {
			for _, file := range manifest.Box.SampleContent {
				entries = append(entries, "Sample Content:"+filepath.Base(file.Source))
			}
		}
		slices.SortStableFunc(entries, func(a, b string) int { return manifest.Rank(a) - manifest.Rank(b) })
		return entries, nil
	}
	if provider == "salesforce" {
		entries, err := salesforcePlannedComponents(root)
		if err != nil {
			return nil, err
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}
	componentType := map[string]string{
		"salesforce": "Salesforce metadata",
		"databricks": "DatabricksAsset",
		"aws":        "AgentCoreSpecification",
	}[provider]
	if componentType == "" {
		componentType = "ProviderAsset"
	}
	entries := make([]string, count)
	for i := range entries {
		entries[i] = fmt.Sprintf("%s:file-%d", componentType, i+1)
	}
	return entries, nil
}

// salesforcePlannedComponents inventories the source package without contacting
// an org or invoking Salesforce CLI. The shared API-native source reader keeps
// planning, validation, and deployment on one canonical component identity.
func salesforcePlannedComponents(root string) ([]string, error) {
	project := findSalesforceProject(root)
	if project == "" {
		return nil, nil
	}
	inventory, _, err := salesforceapi.InventorySource(project)
	entries := make([]string, 0, len(inventory))
	for component := range inventory {
		entries = append(entries, component)
	}
	slices.Sort(entries)
	return entries, err
}

func boxComponentEntries(root string, manifest solution.Manifest, selection solution.ComponentSelection) ([]string, error) {
	entries := []string{}
	arrays := []struct {
		file, key, componentType string
	}{
		{"ai-agent-specs", "agents", "AI Agent"},
		{"automate-workflows", "workflows", "Automate Workflow"},
		{"metadata-templates", "templates", "Metadata Template"},
	}
	for _, spec := range arrays {
		if !manifest.CapabilityEnabled(spec.componentType, selection) {
			continue
		}
		data, err := readBoxConfigObject(root, spec.file)
		if err != nil {
			return nil, err
		}
		var resources []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			TemplateKey string `json:"templateKey"`
			DisplayName string `json:"displayName"`
		}
		if err := json.Unmarshal(data[spec.key], &resources); err != nil {
			return nil, fmt.Errorf("parse %s: %w", spec.file, err)
		}
		for _, resource := range resources {
			name := firstNonEmpty(resource.Name, resource.DisplayName, resource.Key, resource.TemplateKey)
			entries = append(entries, spec.componentType+":"+name)
		}
	}
	objects := []struct {
		file, componentType string
	}{
		{"extract-field-prompts", "Extract Configuration"},
	}
	if manifest.CapabilityEnabled("Doc Gen Template", selection) {
		for _, file := range manifest.Box.SampleContent {
			if strings.EqualFold(filepath.Ext(file.Source), ".docx") {
				entries = append(entries, "Doc Gen Template:"+filepath.Base(file.Source))
			}
		}
	}
	for _, spec := range objects {
		if !manifest.CapabilityEnabled(spec.componentType, selection) {
			continue
		}
		data, err := readBoxConfigObject(root, spec.file)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(data))
		for key := range data {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			entries = append(entries, spec.componentType+":"+key)
		}
	}
	if manifest.CapabilityEnabled(manifest.Box.Workspace.ComponentType, selection) {
		entries = append(entries, manifest.Box.Workspace.ComponentType+":"+manifest.Box.Workspace.DisplayName)
	}
	for _, capability := range manifest.Box.Capabilities {
		if !manifest.CapabilityEnabled(capability.ComponentType, selection) || capability.Source == "" || capability.DisplayName == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(capability.Source))); err == nil {
			entries = append(entries, capability.ComponentType+":"+capability.DisplayName)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return entries, nil
}

type boxTarget struct {
	ParentFolderID string `json:"parentFolderId"`
	WorkspaceName  string `json:"workspaceName"`
	AllowRoot      bool   `json:"allowRootFolder"`
}

type boxMetadataField struct {
	Type        string   `json:"type"`
	Key         string   `json:"key"`
	DisplayName string   `json:"displayName"`
	Options     []string `json:"options"`
}

type boxMetadataTemplate struct {
	TemplateKey string             `json:"templateKey"`
	DisplayName string             `json:"displayName"`
	Fields      []boxMetadataField `json:"fields"`
}

func validateBox(root string, item Item, components []string, report Reporter) (Item, error) {
	item.Missing = append([]string(nil), components...)
	manifest, err := solution.Load(root)
	if err != nil {
		return item, err
	}
	item.ComponentOrder = append([]string(nil), manifest.Box.ComponentOrder...)
	settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
	if err != nil {
		return item, err
	}
	selection := settings.Box.Components
	if len(components) == 0 {
		item.Status, item.Detail = StatusPresent, "No Box configuration categories are enabled for this deployment."
		item.Missing = nil
		return item, nil
	}
	for index, component := range components {
		report.component(component, ProgressQueued, "Queued for Box validation", index, len(components))
	}
	workspace := manifest.Box.Workspace
	workspaceComponent := workspace.ComponentType + ":" + workspace.DisplayName
	target, err := loadBoxTarget(root, workspace.Name)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item, nil
	}
	resolvedWorkspaceName, err := solution.ResolveDeploymentName(target.WorkspaceName, settings.Box)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item, nil
	}
	target.WorkspaceName = resolvedWorkspaceName
	api, err := newBoxAPI()
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item, nil
	}
	ctx := context.Background()
	folderTargetConfigured := target.ParentFolderID != ""
	folderEnabled := manifest.CapabilityEnabled(workspace.ComponentType, selection)
	lookupParentID := target.ParentFolderID
	if lookupParentID == "" {
		lookupParentID = "0"
	}
	workspaceID := ""
	report.component(workspaceComponent, ProgressRunning, "Inspecting the Box workspace", componentIndex(components, workspaceComponent), len(components))
	if folderTargetConfigured {
		present, inspectErr := boxFolderStructureExists(ctx, api, target, workspace.Children)
		if inspectErr != nil {
			item.Status, item.Detail = StatusFailed, "Unable to inspect the Box folder hierarchy: "+inspectErr.Error()
			return item, nil
		}
		if folderEnabled {
			classifyBoxComponent(&item, workspaceComponent, present, !present)
			report.component(workspaceComponent, ProgressCompleted, validationResultMessage(present, !present), componentIndex(components, workspaceComponent)+1, len(components))
		}
	} else {
		readOnlyRootTarget := target
		readOnlyRootTarget.ParentFolderID = "0"
		present, inspectErr := boxFolderStructureExists(ctx, api, readOnlyRootTarget, workspace.Children)
		if inspectErr != nil {
			item.Status, item.Detail = StatusFailed, "Unable to inspect the Box root for an existing workspace: "+inspectErr.Error()
			return item, nil
		}
		if folderEnabled {
			if present {
				classifyBoxComponent(&item, workspaceComponent, true, false)
			} else {
				classifyBoxComponent(&item, workspaceComponent, false, true)
			}
			report.component(workspaceComponent, ProgressCompleted, validationResultMessage(present, !present), componentIndex(components, workspaceComponent)+1, len(components))
		}
	}
	if id, found, inspectErr := api.findFolder(ctx, lookupParentID, target.WorkspaceName); inspectErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to locate the Box workspace: "+inspectErr.Error()
		return item, nil
	} else if found {
		workspaceID = id
	}
	if manifest.CapabilityEnabled("Metadata Template", selection) {
		templates, templateErr := readBoxMetadataTemplates(root)
		if templateErr != nil {
			return item, templateErr
		}
		templateKeys := make([]string, 0, len(templates))
		for _, template := range templates {
			component := "Metadata Template:" + firstNonEmpty(template.DisplayName, template.TemplateKey)
			report.component(component, ProgressRunning, "Inspecting the Box metadata template", componentIndex(components, component), len(components))
			templateKeys = append(templateKeys, template.TemplateKey)
		}
		existingTemplates, templateErr := api.metadataTemplateKeys(ctx, templateKeys)
		if templateErr != nil {
			item.Status, item.Detail = StatusFailed, "Unable to inspect Box metadata templates: "+templateErr.Error()
			return item, nil
		}
		for _, template := range templates {
			component := "Metadata Template:" + firstNonEmpty(template.DisplayName, template.TemplateKey)
			present := existingTemplates[template.TemplateKey]
			classifyBoxComponent(&item, component, present, !present)
			report.component(component, ProgressCompleted, validationResultMessage(present, !present), componentIndex(components, component)+1, len(components))
		}
	}
	if manifest.CapabilityEnabled("Sample Content", selection) {
		for _, file := range manifest.Box.SampleContent {
			component := "Sample Content:" + filepath.Base(file.Source)
			report.component(component, ProgressRunning, "Inspecting the Box file", componentIndex(components, component), len(components))
			source := filepath.Join(root, filepath.FromSlash(file.Source))
			if stat, statErr := os.Stat(source); statErr != nil || stat.IsDir() {
				item.Status, item.Detail = StatusFailed, "Sample content source is missing: "+file.Source
				classifyBoxComponent(&item, component, false, false)
				return item, nil
			}
			present := false
			if workspaceID != "" {
				folderID, found, inspectErr := api.findFolder(ctx, workspaceID, file.TargetFolder)
				if inspectErr != nil {
					item.Status, item.Detail = StatusFailed, "Unable to inspect sample content folder: "+inspectErr.Error()
					return item, nil
				}
				if found {
					_, remoteSHA, exists, inspectErr := api.fileDigest(ctx, folderID, filepath.Base(file.Source))
					if inspectErr != nil {
						item.Status, item.Detail = StatusFailed, "Unable to inspect Box sample content: "+inspectErr.Error()
						return item, nil
					}
					// A file already in Box counts as present only when its content
					// matches the package. When the SHA-1 differs it is treated as
					// deployable so the deploy step uploads a new version; when Box
					// reports no hash, fall back to presence alone.
					present = exists
					if exists && remoteSHA != "" {
						if localSHA, shaErr := localFileSHA1(source); shaErr == nil && localSHA != remoteSHA {
							present = false
							report.step("Detected changed sample content " + filepath.Base(file.Source))
						}
					}
				}
			}
			classifyBoxComponent(&item, component, present, !present)
			report.component(component, ProgressCompleted, validationResultMessage(present, !present), componentIndex(components, component)+1, len(components))
		}
	}
	if publicErr := validateBoxPublicAdapters(ctx, root, manifest, selection, api, workspaceID, &item, report, components); publicErr != nil {
		item.Status, item.Detail = StatusFailed, publicErr.Error()
		return item, nil
	}
	for _, component := range item.Missing {
		if slices.Contains(item.DeployableComponents, component) {
			continue
		}
		componentType := strings.SplitN(component, ":", 2)[0]
		capability, configured := manifest.Capability(componentType)
		if !configured {
			continue
		}
		if capability.API == "public" && capability.Handler == "" {
			item.AdapterPending = append(item.AdapterPending, component)
		} else if capability.API == "experimental" {
			item.Experimental = append(item.Experimental, component)
		}
	}
	slices.SortStableFunc(item.Present, func(a, b string) int { return manifest.Rank(a) - manifest.Rank(b) })
	slices.SortStableFunc(item.Missing, func(a, b string) int { return manifest.Rank(a) - manifest.Rank(b) })
	item.Deployable = len(item.DeployableComponents) > 0
	if item.Deployable {
		item.Status = StatusMissing
		item.Detail = fmt.Sprintf("%d Box components already exist; %d can be deployed now; %d remain manual.", len(item.Present), len(item.DeployableComponents), len(item.Missing)-len(item.DeployableComponents))
	} else {
		item.Status = StatusManual
		item.Detail = fmt.Sprintf("%d Box components already exist; %d later-stage components require manual review.", len(item.Present), len(item.Missing))
		if !folderTargetConfigured && slices.Contains(item.Missing, workspaceComponent) {
			item.Detail += " The workspace was not found at the Box root; select a parent folder to check or deploy it."
		}
	}
	reportValidationResults(report, components, item.Present, item.Missing)
	return item, nil
}

func reportValidationResults(report Reporter, components, present, missing []string) {
	presentSet := make(map[string]bool, len(present))
	for _, component := range present {
		presentSet[component] = true
	}
	missingSet := make(map[string]bool, len(missing))
	for _, component := range missing {
		missingSet[component] = true
	}
	ordered := append([]string(nil), components...)
	slices.Sort(ordered)
	for index, component := range ordered {
		report.component(component, ProgressRunning, "Checking "+component, index, len(ordered))
		message := "Validation complete"
		switch {
		case presentSet[component]:
			message = "Already present"
		case missingSet[component]:
			message = "Ready to deploy"
		}
		report.component(component, ProgressCompleted, message, index+1, len(ordered))
	}
}

func componentIndex(components []string, component string) int {
	index := slices.Index(components, component)
	if index < 0 {
		return 0
	}
	return index
}

func validationResultMessage(present, deployable bool) string {
	if present {
		return "Already present"
	}
	if deployable {
		return "Ready to deploy"
	}
	return "Manual review required"
}

func classifyBoxComponent(item *Item, component string, present, deployable bool) {
	item.Present = slices.DeleteFunc(item.Present, func(value string) bool { return value == component })
	item.Missing = slices.DeleteFunc(item.Missing, func(value string) bool { return value == component })
	item.DeployableComponents = slices.DeleteFunc(item.DeployableComponents, func(value string) bool { return value == component })
	if present {
		item.Present = append(item.Present, component)
		return
	}
	item.Missing = append(item.Missing, component)
	if deployable {
		item.DeployableComponents = append(item.DeployableComponents, component)
	}
}

func loadBoxTarget(root, defaultWorkspaceName string) (boxTarget, error) {
	target := boxTarget{
		ParentFolderID: strings.TrimSpace(os.Getenv("BOX_PARENT_FOLDER_ID")),
		WorkspaceName:  strings.TrimSpace(os.Getenv("BOX_WORKSPACE_NAME")),
		AllowRoot:      strings.EqualFold(strings.TrimSpace(os.Getenv("BOX_ALLOW_ROOT_FOLDER")), "true"),
	}
	path := filepath.Join(root, "config", "runtime", "demo-environment.json")
	if data, err := os.ReadFile(path); err == nil {
		var runtime struct {
			Box boxTarget `json:"box"`
		}
		if err := json.Unmarshal(data, &runtime); err != nil {
			return target, fmt.Errorf("parse %s: %w", path, err)
		}
		if target.ParentFolderID == "" {
			target.ParentFolderID = strings.TrimSpace(runtime.Box.ParentFolderID)
		}
		if target.WorkspaceName == "" {
			target.WorkspaceName = strings.TrimSpace(runtime.Box.WorkspaceName)
		}
		if !target.AllowRoot {
			target.AllowRoot = runtime.Box.AllowRoot
		}
	} else if !os.IsNotExist(err) {
		return target, err
	}
	if target.WorkspaceName == "" {
		target.WorkspaceName = defaultWorkspaceName
	}
	if target.ParentFolderID == "0" && !target.AllowRoot {
		return target, fmt.Errorf("Box root-folder deployment is blocked; set BOX_ALLOW_ROOT_FOLDER=true to explicitly allow parent folder 0")
	}
	return target, nil
}

func boxFolderStructureExists(ctx context.Context, api boxAPI, target boxTarget, children []string) (bool, error) {
	workspaceID, found, err := api.findFolder(ctx, target.ParentFolderID, target.WorkspaceName)
	if err != nil || !found {
		return false, err
	}
	for _, folder := range children {
		if _, found, err := api.findFolder(ctx, workspaceID, folder); err != nil || !found {
			return false, err
		}
	}
	return true, nil
}

func readBoxMetadataTemplates(root string) ([]boxMetadataTemplate, error) {
	data, err := readBoxConfigObject(filepath.Join(root, "config", "box"), "metadata-templates")
	if err != nil {
		return nil, err
	}
	var templates []boxMetadataTemplate
	if err := json.Unmarshal(data["templates"], &templates); err != nil {
		return nil, fmt.Errorf("parse metadata-templates: %w", err)
	}
	return templates, nil
}

func readJSONObject(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return object, nil
}

// readBoxConfigObject reads a Box config payload, preferring the migrated BCL
// artifact (<base>.bcl) and falling back to legacy JSON (<base>.json). base is
// the file stem, e.g. "ai-agent-specs". BCL is the packaged format now; the JSON
// fallback keeps older packages and test fixtures working.
func readBoxConfigObject(dir, base string) (map[string]json.RawMessage, error) {
	bclPath := filepath.Join(dir, base+".bcl")
	if _, err := os.Stat(bclPath); err == nil {
		return bcl.ConfigObject(bclPath)
	}
	return readJSONObject(filepath.Join(dir, base+".json"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "unnamed"
}

func DeployMissing(root string, items []Item) ([]Item, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return nil, err
	}
	results := append([]Item(nil), items...)
	for i := range results {
		if results[i].Status != StatusMissing || !results[i].Deployable {
			continue
		}
		results[i] = deployProvider(root, results[i], settings, nil)
	}
	return results, nil
}

func DeployProvider(root string, item Item, report Reporter) (Item, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return item, err
	}
	return deployProvider(root, item, settings, report), nil
}

func deployProvider(root string, item Item, settings config.ConnectionSettings, report Reporter) Item {
	if item.Status != StatusMissing || !item.Deployable {
		return item
	}
	if item.Provider == "box" {
		return deployBoxFoundation(root, item, report)
	}
	if item.Provider != "salesforce" {
		return item
	}
	if settings.HasSalesforceREST() {
		return deploySalesforceREST(root, item, settings, report)
	}
	project := findSalesforceProject(root)
	if settings.SalesforceAlias == "" {
		item.Status, item.Detail = StatusFailed, "No Salesforce alias is selected."
		return item
	}
	report.step("Checking Salesforce org status and expiration")
	orgInfo, inspectErr := salesforceorg.Inspect(settings.SalesforceAlias, time.Now())
	if inspectErr != nil {
		item.Status = StatusFailed
		item.Detail = "Salesforce deployment stopped before sending metadata: " + inspectErr.Error()
		if failure, ok := inspectErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item
	}
	manifest, manifestErr := solution.Load(root)
	if manifestErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to read Salesforce deployment prerequisites: "+manifestErr.Error()
		return item
	}
	if packageErr := ensureSalesforcePackages(settings.SalesforceAlias, manifest.Salesforce.RequiredPackages, report); packageErr != nil {
		item.Status = StatusFailed
		item.Detail = "Salesforce deployment stopped before sending metadata: " + packageErr.Error()
		if failure, ok := packageErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item
	}
	report.step("Building Salesforce UI bundles")
	if buildErr := buildSalesforceUIBundles(project); buildErr != nil {
		item.Status, item.Detail = StatusFailed, buildErr.Error()
		return item
	}
	metadata := missingSalesforceMetadata(item.Missing)
	deployID := ""
	if len(metadata) > 0 {
		report.step(fmt.Sprintf("Deploying %d missing Salesforce metadata components", len(metadata)))
		cmd := exec.Command("sf", salesforceMetadataDeployArgs(settings.SalesforceAlias, metadata)...)
		cmd.Dir = project
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			item.Status = StatusFailed
			item.Detail, item.Diagnostic = salesforceErrorDetails(output, runErr)
			return item
		}
		var deployResponse struct {
			Result struct {
				ID string `json:"id"`
			} `json:"result"`
		}
		_ = decodeSalesforceJSON(output, &deployResponse)
		deployID = deployResponse.Result.ID
	} else {
		report.step("Salesforce metadata is already present; skipping metadata deployment")
	}
	instanceURL := ""
	orgID := ""
	orgOutput, orgErr := exec.Command("sf", "org", "display", "--target-org", settings.SalesforceAlias, "--json").Output()
	if orgErr == nil {
		var orgResponse struct {
			Result struct {
				ID          string `json:"id"`
				InstanceURL string `json:"instanceUrl"`
				Username    string `json:"username"`
			} `json:"result"`
		}
		if json.Unmarshal(orgOutput, &orgResponse) == nil {
			orgID = orgResponse.Result.ID
			instanceURL = strings.TrimRight(orgResponse.Result.InstanceURL, "/")
			addResource(&item, "Salesforce org", "organization", orgResponse.Result.Username, orgID, instanceURL)
		}
	}
	deployURL := ""
	if instanceURL != "" && deployID != "" {
		deployURL = instanceURL + "/" + deployID
	}
	if deployID != "" {
		addResource(&item, "Salesforce metadata", "metadata_deployment", "Salesforce metadata deployment", deployID, deployURL)
	}
	report.step("Assigning required Salesforce permission sets")
	if permissionErr := ensureSalesforcePermissionSets(settings.SalesforceAlias, orgInfo.Username, manifest.Salesforce.RequiredPermissionSets); permissionErr != nil {
		item.Status = StatusFailed
		item.Detail = "Salesforce metadata deployed, but required permission-set assignment failed: " + permissionErr.Error()
		if failure, ok := permissionErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item
	}
	item.Status, item.Detail = StatusPresent, "Salesforce metadata deployed successfully."
	item.Present = append(item.Present, item.Missing...)
	slices.Sort(item.Present)
	item.Missing = nil
	return item
}

func deployBoxFoundation(root string, item Item, report Reporter) Item {
	// Shared with teardown so both address the same resolved workspace.
	box, err := loadBoxContext(root)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	manifest, selection := box.manifest, box.selection
	target, api, ctx := box.target, box.api, box.ctx
	workspace := manifest.Box.Workspace
	deployed := []string{}
	folderComponent := workspace.ComponentType + ":" + workspace.DisplayName
	workspaceID := ""
	if slices.Contains(item.DeployableComponents, folderComponent) {
		report.step("Ensuring the Box workspace folder tree")
		var createErr error
		workspaceID, createErr = api.ensureFolder(ctx, target.ParentFolderID, target.WorkspaceName)
		if createErr != nil {
			item.Status, item.Detail = StatusFailed, createErr.Error()
			return item
		}
		addResource(&item, folderComponent, "folder", target.WorkspaceName, workspaceID, "https://app.box.com/folder/"+workspaceID)
		for _, folder := range workspace.Children {
			folderID, createErr := api.ensureFolder(ctx, workspaceID, folder)
			if createErr != nil {
				item.Status, item.Detail = StatusFailed, createErr.Error()
				return item
			}
			addResource(&item, folderComponent, "folder", folder, folderID, "https://app.box.com/folder/"+folderID)
		}
		deployed = append(deployed, folderComponent)
	}
	if workspaceID == "" {
		var found bool
		workspaceID, found, err = api.findFolder(ctx, target.ParentFolderID, target.WorkspaceName)
		if err != nil || !found {
			item.Status, item.Detail = StatusFailed, "Locate Box workspace before content upload"
			if err != nil {
				item.Detail += ": " + err.Error()
			}
			return item
		}
	}
	addResource(&item, folderComponent, "folder", target.WorkspaceName, workspaceID, "https://app.box.com/folder/"+workspaceID)
	for _, folder := range workspace.Children {
		folderID, found, inspectErr := api.findFolder(ctx, workspaceID, folder)
		if inspectErr != nil {
			item.Status, item.Detail = StatusFailed, "Inventory Box workspace folder "+folder+": "+inspectErr.Error()
			return item
		}
		if found {
			addResource(&item, folderComponent, "folder", folder, folderID, "https://app.box.com/folder/"+folderID)
		}
	}
	if manifest.CapabilityEnabled("Metadata Template", selection) {
		templates, templateErr := readBoxMetadataTemplates(root)
		if templateErr != nil {
			item.Status, item.Detail = StatusFailed, templateErr.Error()
			return item
		}
		for _, template := range templates {
			component := "Metadata Template:" + firstNonEmpty(template.DisplayName, template.TemplateKey)
			if !slices.Contains(item.DeployableComponents, component) {
				continue
			}
			report.step("Applying metadata template " + firstNonEmpty(template.DisplayName, template.TemplateKey))
			if createErr := api.createMetadataTemplate(ctx, template); createErr != nil {
				item.Status, item.Detail = StatusFailed, "Create "+component+": "+createErr.Error()
				return item
			}
			addResource(&item, component, "metadata_template", template.DisplayName, template.TemplateKey, "")
			deployed = append(deployed, component)
		}
	}
	if manifest.CapabilityEnabled("Sample Content", selection) {
		for _, file := range manifest.Box.SampleContent {
			component := "Sample Content:" + filepath.Base(file.Source)
			if !slices.Contains(item.DeployableComponents, component) {
				continue
			}
			source := filepath.Join(root, filepath.FromSlash(file.Source))
			if stat, statErr := os.Stat(source); statErr != nil || stat.IsDir() {
				item.Status, item.Detail = StatusFailed, "Sample content source is missing: "+file.Source
				return item
			}
			folderID, createErr := api.ensureFolder(ctx, workspaceID, file.TargetFolder)
			if createErr != nil {
				item.Status, item.Detail = StatusFailed, "Prepare sample content folder: "+createErr.Error()
				return item
			}
			fileID, remoteSHA, exists, inspectErr := api.fileDigest(ctx, folderID, filepath.Base(file.Source))
			if inspectErr != nil {
				item.Status, item.Detail = StatusFailed, "Inspect "+component+": "+inspectErr.Error()
				return item
			}
			localSHA, shaErr := localFileSHA1(source)
			if shaErr != nil {
				item.Status, item.Detail = StatusFailed, "Hash "+component+": "+shaErr.Error()
				return item
			}
			switch {
			case !exists:
				report.step("Uploading sample content " + filepath.Base(file.Source))
				fileID, inspectErr = api.uploadFile(ctx, folderID, source)
				if inspectErr != nil {
					item.Status, item.Detail = StatusFailed, "Deploy "+component+": "+inspectErr.Error()
					return item
				}
			case remoteSHA != "" && remoteSHA != localSHA:
				// The packaged file differs from the one in Box: replace it with a
				// new version rather than leaving stale content or failing.
				report.step("Updating changed sample content " + filepath.Base(file.Source))
				fileID, inspectErr = api.uploadFileVersion(ctx, folderID, source)
				if inspectErr != nil {
					item.Status, item.Detail = StatusFailed, "Update "+component+": "+inspectErr.Error()
					return item
				}
			default:
				report.step("Sample content unchanged " + filepath.Base(file.Source))
			}
			if fileID == "" {
				item.Status, item.Detail = StatusFailed, "Resolve deployed "+component+" ID"
				return item
			}
			addResource(&item, component, "file", filepath.Base(file.Source), fileID, "https://app.box.com/file/"+fileID)
			deployed = append(deployed, component)
		}
	}
	for _, file := range manifest.Box.SampleContent {
		fileID, found, inspectErr := findWorkspaceFile(ctx, api, workspaceID, file.TargetFolder, filepath.Base(file.Source))
		if inspectErr != nil {
			item.Status, item.Detail = StatusFailed, "Inventory Box sample content: "+inspectErr.Error()
			return item
		}
		if found {
			addResource(&item, "Sample Content:"+filepath.Base(file.Source), "file", filepath.Base(file.Source), fileID, "https://app.box.com/file/"+fileID)
		}
	}
	publicDeployed, publicResources, publicErr := deployBoxPublicAdapters(ctx, root, manifest, selection, api, workspaceID, item.DeployableComponents, report)
	if publicErr != nil {
		item.Status, item.Detail = StatusFailed, publicErr.Error()
		return item
	}
	deployed = append(deployed, publicDeployed...)
	item.Resources = append(item.Resources, publicResources...)
	for _, component := range deployed {
		classifyBoxComponent(&item, component, true, false)
	}
	item.Deployable = len(item.DeployableComponents) > 0
	if len(item.Missing) == 0 {
		item.Status, item.Detail = StatusPresent, "Box foundation deployed successfully."
	} else {
		item.Status, item.Detail = StatusManual, fmt.Sprintf("Deployed %d Box foundation components; %d later-stage components remain for manual review.", len(deployed), len(item.Missing))
	}
	return item
}

type salesforcePackage struct {
	Types []struct {
		Members []string `xml:"members"`
		Name    string   `xml:"name"`
	} `xml:"types"`
}

type salesforceRetrieve struct {
	Result struct {
		FileProperties []struct {
			FullName string `json:"fullName"`
			Type     string `json:"type"`
		} `json:"fileProperties"`
	} `json:"result"`
}

type salesforceObject struct {
	Fields []struct {
		FullName string `xml:"fullName"`
	} `xml:"fields"`
}

type installedSalesforcePackage struct {
	SubscriberPackageID            string `json:"SubscriberPackageId"`
	SubscriberPackageName          string `json:"SubscriberPackageName"`
	SubscriberPackageNamespace     string `json:"SubscriberPackageNamespace"`
	SubscriberPackageVersionID     string `json:"SubscriberPackageVersionId"`
	SubscriberPackageVersionName   string `json:"SubscriberPackageVersionName"`
	SubscriberPackageVersionNumber string `json:"SubscriberPackageVersionNumber"`
}

func validateSalesforce(root string, item Item, report Reporter) (Item, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return item, err
	}
	if settings.HasSalesforceREST() {
		return validateSalesforceREST(root, item, report, settings)
	}
	if settings.SalesforceAlias == "" {
		item.Status, item.Detail = StatusFailed, "No Salesforce alias is selected. Connect Salesforce before validation."
		return item, nil
	}
	project := findSalesforceProject(root)
	if project == "" {
		item.Status, item.Detail = StatusManual, "No Salesforce project was found in the package."
		return item, nil
	}
	report.step("Checking Salesforce org status and expiration")
	orgInfo, inspectErr := salesforceorg.Inspect(settings.SalesforceAlias, time.Now())
	if inspectErr != nil {
		item.Status = StatusFailed
		item.Detail = "Salesforce validation stopped before reading metadata: " + inspectErr.Error()
		if failure, ok := inspectErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item, nil
	}
	manifestContract, manifestErr := solution.Load(root)
	if manifestErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to read Salesforce deployment prerequisites: "+manifestErr.Error()
		return item, nil
	}
	item.ComponentOrder = []string{"Managed Package"}
	for index, requirement := range manifestContract.Salesforce.RequiredPackages {
		report.component(salesforcePackageComponent(requirement), ProgressRunning, "Checking installed package version", index, len(manifestContract.Salesforce.RequiredPackages))
	}
	report.step("Checking required Salesforce managed packages")
	installedPackages, packageErr := listInstalledSalesforcePackages(settings.SalesforceAlias)
	if packageErr != nil {
		item.Status = StatusFailed
		item.Detail = "Unable to inspect installed Salesforce packages: " + packageErr.Error()
		if failure, ok := packageErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item, nil
	}
	for index, requirement := range manifestContract.Salesforce.RequiredPackages {
		message := "Required version is installed"
		if len(missingSalesforcePackages([]solution.SalesforcePackageRequirement{requirement}, installedPackages)) > 0 {
			message = "Package installation required"
		}
		report.component(salesforcePackageComponent(requirement), ProgressCompleted, message, index+1, len(manifestContract.Salesforce.RequiredPackages))
	}
	for index, requirement := range manifestContract.Salesforce.RequiredPermissionSets {
		report.component(salesforcePermissionSetComponent(requirement), ProgressRunning, "Checking assignment for the deployment user", index, len(manifestContract.Salesforce.RequiredPermissionSets))
	}
	report.step("Checking required Salesforce permission sets")
	permissionInventory, permissionErr := readSalesforceUserPermissionInventory(settings.SalesforceAlias, orgInfo.Username)
	if permissionErr != nil {
		item.Status = StatusFailed
		item.Detail = "Unable to inspect Salesforce permission-set assignments: " + permissionErr.Error()
		if failure, ok := permissionErr.(*salesforceorg.Failure); ok {
			item.Diagnostic = failure.Diagnostic
		}
		return item, nil
	}
	for index, requirement := range manifestContract.Salesforce.RequiredPermissionSets {
		message := "Assigned to the deployment user"
		if !permissionInventory.Assigned[strings.ToLower(strings.TrimSpace(requirement.Name))] {
			message = "Assignment required"
		}
		report.component(salesforcePermissionSetComponent(requirement), ProgressCompleted, message, index+1, len(manifestContract.Salesforce.RequiredPermissionSets))
	}
	if !strings.EqualFold(permissionInventory.Profile, "System Administrator") {
		item.Status = StatusFailed
		item.Detail = fmt.Sprintf("The authenticated Salesforce deployment user %s has profile %q. Select a System Administrator connection so Dispatch can install prerequisites and assign the required permission sets.", orgInfo.Username, permissionInventory.Profile)
		return item, nil
	}
	if buildErr := buildSalesforceUIBundles(project); buildErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to prepare packaged Salesforce UI Bundles: "+buildErr.Error()
		return item, nil
	}
	temp, err := os.MkdirTemp("", "box-dispatch-salesforce-validate-")
	if err != nil {
		return item, err
	}
	defer os.RemoveAll(temp)
	manifestDir := filepath.Join(temp, "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return item, err
	}
	generate := exec.Command("sf", "project", "generate", "manifest", "--source-dir", "force-app", "--type", "package", "--output-dir", manifestDir, "--json")
	generate.Dir = project
	if output, runErr := generate.CombinedOutput(); runErr != nil {
		item.Status = StatusFailed
		summary, diagnostic := salesforceErrorDetails(output, runErr)
		item.Detail, item.Diagnostic = "Unable to inventory packaged Salesforce metadata: "+summary, diagnostic
		return item, nil
	}
	manifest := filepath.Join(manifestDir, "package.xml")
	expected, err := readSalesforceManifest(manifest)
	if err != nil {
		return item, err
	}
	metadataComponents := make([]string, 0, len(expected))
	for component := range expected {
		metadataComponents = append(metadataComponents, component)
	}
	slices.Sort(metadataComponents)
	for index, component := range metadataComponents {
		report.component(component, ProgressRunning, "Reading current Salesforce state", index, len(metadataComponents))
	}
	report.step(fmt.Sprintf("Retrieving Salesforce state for %d metadata components", len(metadataComponents)))
	retrievedDir := filepath.Join(temp, "retrieved")
	retrieve := exec.Command("sf", "project", "retrieve", "start", "--manifest", manifest, "--target-org", settings.SalesforceAlias, "--target-metadata-dir", retrievedDir, "--unzip", "--json")
	retrieve.Dir = project
	output, runErr := retrieve.CombinedOutput()
	if runErr != nil {
		item.Status = StatusFailed
		summary, diagnostic := salesforceErrorDetails(output, runErr)
		item.Detail, item.Diagnostic = "Unable to read Salesforce metadata: "+summary, diagnostic
		return item, nil
	}
	existing, err := readSalesforceInventory(output, retrievedDir)
	if err != nil {
		return item, err
	}
	missing := missingSalesforceComponents(expected, existing)
	if len(missing) > 0 {
		report.step("Checking missing Salesforce metadata for source-tracking conflicts")
		conflicts, previewErr := inspectSalesforceMetadataConflicts(project, settings.SalesforceAlias, missing)
		if previewErr != nil {
			item.Status = StatusFailed
			if failure, ok := previewErr.(*salesforceorg.Failure); ok {
				item.Detail = failure.Summary
				item.Diagnostic = failure.Diagnostic
			} else {
				item.Detail = previewErr.Error()
			}
			return item, nil
		}
		for _, component := range conflicts {
			existing[component] = true
		}
	}
	result := classifySalesforceInventory(item, expected, existing, settings.SalesforceAlias)
	reportValidationResults(report, metadataComponents, result.Present, result.Missing)
	result = addSalesforcePackageResults(result, manifestContract.Salesforce.RequiredPackages, installedPackages, settings.SalesforceAlias)
	return addSalesforcePermissionSetResults(result, manifestContract.Salesforce.RequiredPermissionSets, permissionInventory.Assigned, settings.SalesforceAlias), nil
}

func listInstalledSalesforcePackages(target string) ([]installedSalesforcePackage, error) {
	output, runErr := exec.Command("sf", "package", "installed", "list", "--target-org", target, "--json").CombinedOutput()
	if runErr != nil {
		return nil, salesforceorg.NewFailure("Salesforce CLI could not list installed packages for "+target+". Recheck the org connection and retry.", output, runErr)
	}
	var payload struct {
		Result []installedSalesforcePackage `json:"result"`
	}
	if parseErr := decodeSalesforceJSON(output, &payload); parseErr != nil {
		return nil, salesforceorg.NewFailure("Salesforce CLI returned an unreadable installed-package inventory for "+target+". Update the Salesforce CLI and retry.", output, parseErr)
	}
	return payload.Result, nil
}

func missingSalesforcePackages(required []solution.SalesforcePackageRequirement, installed []installedSalesforcePackage) []solution.SalesforcePackageRequirement {
	missing := make([]solution.SalesforcePackageRequirement, 0, len(required))
	for _, requirement := range required {
		satisfied := false
		for _, candidate := range installed {
			identityMatches := strings.EqualFold(strings.TrimSpace(candidate.SubscriberPackageNamespace), strings.TrimSpace(requirement.Namespace))
			if requirement.Namespace == "" {
				identityMatches = candidate.SubscriberPackageID == requirement.PackageID || strings.EqualFold(candidate.SubscriberPackageName, requirement.Name)
			}
			if !identityMatches {
				continue
			}
			if requirement.VersionID == "" || candidate.SubscriberPackageVersionID == requirement.VersionID ||
				salesforcePackageVersionAtLeast(candidate.SubscriberPackageVersionNumber, requirement.VersionNumber) {
				satisfied = true
			}
			break
		}
		if !satisfied {
			missing = append(missing, requirement)
		}
	}
	return missing
}

func salesforcePackageVersionAtLeast(installed, required string) bool {
	installedParts, installedOK := parseSalesforcePackageVersion(installed)
	requiredParts, requiredOK := parseSalesforcePackageVersion(required)
	if !installedOK || !requiredOK {
		return false
	}
	partCount := max(len(installedParts), len(requiredParts))
	for index := 0; index < partCount; index++ {
		installedPart, requiredPart := 0, 0
		if index < len(installedParts) {
			installedPart = installedParts[index]
		}
		if index < len(requiredParts) {
			requiredPart = requiredParts[index]
		}
		if installedPart != requiredPart {
			return installedPart > requiredPart
		}
	}
	return true
}

func parseSalesforcePackageVersion(version string) ([]int, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, false
	}
	parts := strings.Split(version, ".")
	parsed := make([]int, len(parts))
	for index, part := range parts {
		if part == "" {
			return nil, false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil, false
		}
		parsed[index] = value
	}
	return parsed, true
}

func salesforcePackageComponent(requirement solution.SalesforcePackageRequirement) string {
	name := strings.TrimSpace(requirement.Name)
	if name == "" {
		name = strings.TrimSpace(requirement.Namespace)
	}
	version := firstNonEmpty(requirement.VersionName, requirement.VersionNumber)
	if version != "unnamed" {
		name += " " + version
	}
	return "Managed Package:" + strings.TrimSpace(name)
}

func salesforcePermissionSetComponent(requirement solution.SalesforcePermissionSetRequirement) string {
	label := firstNonEmpty(requirement.Label, requirement.Name)
	return "Permission Set Assignment:" + strings.TrimSpace(label)
}

type salesforceUserPermissionInventory struct {
	Profile  string
	Assigned map[string]bool
}

func readSalesforceUserPermissionInventory(target, username string) (salesforceUserPermissionInventory, error) {
	query := "SELECT Profile.Name, (SELECT PermissionSet.Name, PermissionSet.NamespacePrefix FROM PermissionSetAssignments) FROM User WHERE Username = '" + escapeSOQLString(username) + "'"
	output, runErr := exec.Command("sf", "data", "query", "--query", query, "--target-org", target, "--json").CombinedOutput()
	if runErr != nil {
		return salesforceUserPermissionInventory{}, salesforceorg.NewFailure("Salesforce CLI could not inspect permission sets for the authenticated deployment user. Recheck the org connection and retry.", output, runErr)
	}
	return decodeSalesforceUserPermissionInventory(output, username)
}

func decodeSalesforceUserPermissionInventory(output []byte, username string) (salesforceUserPermissionInventory, error) {
	var payload struct {
		Result struct {
			Records []struct {
				Profile struct {
					Name string `json:"Name"`
				} `json:"Profile"`
				PermissionSetAssignments struct {
					Records []struct {
						PermissionSet struct {
							Name            string `json:"Name"`
							NamespacePrefix string `json:"NamespacePrefix"`
						} `json:"PermissionSet"`
					} `json:"records"`
				} `json:"PermissionSetAssignments"`
			} `json:"records"`
		} `json:"result"`
	}
	if parseErr := decodeSalesforceJSON(output, &payload); parseErr != nil {
		return salesforceUserPermissionInventory{}, salesforceorg.NewFailure("Salesforce CLI returned an unreadable permission-set inventory. Update the Salesforce CLI and retry.", output, parseErr)
	}
	if len(payload.Result.Records) != 1 {
		return salesforceUserPermissionInventory{}, &salesforceorg.Failure{Summary: "Salesforce did not return exactly one authenticated deployment user for " + username + ". Reconnect the intended org and retry."}
	}
	record := payload.Result.Records[0]
	assigned := map[string]bool{}
	for _, assignment := range record.PermissionSetAssignments.Records {
		name := strings.TrimSpace(assignment.PermissionSet.Name)
		if namespace := strings.TrimSpace(assignment.PermissionSet.NamespacePrefix); namespace != "" {
			name = namespace + "__" + name
		}
		if name != "" {
			assigned[strings.ToLower(name)] = true
		}
	}
	return salesforceUserPermissionInventory{Profile: strings.TrimSpace(record.Profile.Name), Assigned: assigned}, nil
}

func escapeSOQLString(value string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
}

func missingSalesforcePermissionSets(required []solution.SalesforcePermissionSetRequirement, assigned map[string]bool) []solution.SalesforcePermissionSetRequirement {
	missing := make([]solution.SalesforcePermissionSetRequirement, 0, len(required))
	for _, requirement := range required {
		if !assigned[strings.ToLower(strings.TrimSpace(requirement.Name))] {
			missing = append(missing, requirement)
		}
	}
	return missing
}

func addSalesforcePermissionSetResults(item Item, required []solution.SalesforcePermissionSetRequirement, assigned map[string]bool, alias string) Item {
	missing := missingSalesforcePermissionSets(required, assigned)
	for _, requirement := range required {
		component := salesforcePermissionSetComponent(requirement)
		if assigned[strings.ToLower(strings.TrimSpace(requirement.Name))] {
			if !slices.Contains(item.Present, component) {
				item.Present = append(item.Present, component)
			}
			continue
		}
		if !slices.Contains(item.Missing, component) {
			item.Missing = append(item.Missing, component)
		}
		if !slices.Contains(item.DeployableComponents, component) {
			item.DeployableComponents = append(item.DeployableComponents, component)
		}
	}
	slices.Sort(item.Present)
	slices.Sort(item.Missing)
	slices.Sort(item.DeployableComponents)
	if len(missing) > 0 {
		item.Status = StatusMissing
		item.Deployable = true
		item.Detail = fmt.Sprintf("%d components already exist; %d deployment steps remain for Salesforce org %s. Dispatch installs managed packages first, deploys metadata, then assigns required permission sets to the authenticated System Administrator.", len(item.Present), len(item.Missing), alias)
	}
	return item
}

func ensureSalesforcePermissionSets(target, username string, required []solution.SalesforcePermissionSetRequirement) error {
	if len(required) == 0 {
		return nil
	}
	inventory, err := readSalesforceUserPermissionInventory(target, username)
	if err != nil {
		return err
	}
	if !strings.EqualFold(inventory.Profile, "System Administrator") {
		return &salesforceorg.Failure{Summary: fmt.Sprintf("The authenticated Salesforce deployment user %s has profile %q, not System Administrator. Select the intended administrator connection and retry.", username, inventory.Profile)}
	}
	missing := missingSalesforcePermissionSets(required, inventory.Assigned)
	if len(missing) == 0 {
		return nil
	}
	args := []string{"org", "assign", "permset", "--target-org", target, "--on-behalf-of", username, "--json"}
	for _, requirement := range missing {
		args = append(args, "--name", requirement.Name)
	}
	output, runErr := exec.Command("sf", args...).CombinedOutput()
	if runErr != nil {
		summary, diagnostic := salesforceErrorDetails(output, runErr)
		return &salesforceorg.Failure{Summary: "Unable to assign required Salesforce permission sets: " + summary, Diagnostic: diagnostic}
	}
	verified, err := readSalesforceUserPermissionInventory(target, username)
	if err != nil {
		return err
	}
	if remaining := missingSalesforcePermissionSets(required, verified.Assigned); len(remaining) > 0 {
		return &salesforceorg.Failure{Summary: "Salesforce CLI completed permission-set assignment, but " + firstNonEmpty(remaining[0].Label, remaining[0].Name) + " is still not assigned to the authenticated System Administrator. Review the full Salesforce CLI diagnostic and retry."}
	}
	return nil
}

func addMissingSalesforcePackages(item Item, missing []solution.SalesforcePackageRequirement, alias string) Item {
	if len(missing) == 0 {
		return item
	}
	for _, requirement := range missing {
		component := salesforcePackageComponent(requirement)
		item.Missing = append(item.Missing, component)
		item.DeployableComponents = append(item.DeployableComponents, component)
	}
	slices.Sort(item.Missing)
	slices.Sort(item.DeployableComponents)
	item.Status = StatusMissing
	item.Deployable = true
	description := make([]string, 0, len(missing))
	for _, requirement := range missing {
		label := firstNonEmpty(requirement.Name, requirement.Namespace)
		if requirement.VersionNumber != "" {
			label += " " + requirement.VersionNumber
		} else if requirement.VersionName != "" {
			label += " " + requirement.VersionName
		}
		description = append(description, label)
	}
	item.Detail = fmt.Sprintf("%d metadata components already exist; %d deployment steps remain for Salesforce org %s. Dispatch will install %s before metadata deployment.", len(item.Present), len(item.Missing), alias, strings.Join(description, ", "))
	return item
}

func addSalesforcePackageResults(item Item, required []solution.SalesforcePackageRequirement, installed []installedSalesforcePackage, alias string) Item {
	missing := missingSalesforcePackages(required, installed)
	for _, requirement := range required {
		if len(missingSalesforcePackages([]solution.SalesforcePackageRequirement{requirement}, installed)) != 0 {
			continue
		}
		component := salesforcePackageComponent(requirement)
		if !slices.Contains(item.Present, component) {
			item.Present = append(item.Present, component)
		}
	}
	slices.Sort(item.Present)
	return addMissingSalesforcePackages(item, missing, alias)
}

func ensureSalesforcePackages(target string, required []solution.SalesforcePackageRequirement, report Reporter) error {
	if len(required) == 0 {
		return nil
	}
	installed, err := listInstalledSalesforcePackages(target)
	if err != nil {
		return err
	}
	missing := missingSalesforcePackages(required, installed)
	for _, requirement := range missing {
		if strings.TrimSpace(requirement.VersionID) == "" {
			return &salesforceorg.Failure{Summary: "Required Salesforce package " + firstNonEmpty(requirement.Name, requirement.Namespace) + " does not declare an installable 04t version ID in dispatch.bcl."}
		}
		report.step("Installing " + strings.TrimPrefix(salesforcePackageComponent(requirement), "Managed Package:"))
		args := salesforcePackageInstallArgs(requirement, target)
		output, runErr := exec.Command("sf", args...).CombinedOutput()
		if runErr != nil {
			summary, diagnostic := salesforceErrorDetails(output, runErr)
			return &salesforceorg.Failure{Summary: "Unable to install " + firstNonEmpty(requirement.Name, requirement.Namespace) + ": " + summary, Diagnostic: diagnostic}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	installed, err = listInstalledSalesforcePackages(target)
	if err != nil {
		return err
	}
	if remaining := missingSalesforcePackages(required, installed); len(remaining) > 0 {
		return &salesforceorg.Failure{Summary: "Salesforce CLI completed the package install, but " + firstNonEmpty(remaining[0].Name, remaining[0].Namespace) + " is still not present at the required version. Review the full Salesforce CLI diagnostic and retry."}
	}
	return nil
}

func salesforcePackageInstallArgs(requirement solution.SalesforcePackageRequirement, target string) []string {
	securityType := strings.TrimSpace(requirement.SecurityType)
	if securityType == "" {
		securityType = "AdminsOnly"
	}
	return []string{
		"package", "install",
		"--package", requirement.VersionID,
		"--target-org", target,
		"--security-type", securityType,
		"--wait", "30",
		"--no-prompt",
		"--json",
	}
}

func missingSalesforceMetadata(missing []string) []string {
	metadata := make([]string, 0, len(missing))
	for _, component := range missing {
		metadataType, _, ok := strings.Cut(component, ":")
		if !ok {
			continue
		}
		switch metadataType {
		case "Managed Package", "Permission Set Assignment":
			continue
		default:
			metadata = append(metadata, component)
		}
	}
	slices.Sort(metadata)
	return metadata
}

func missingSalesforceComponents(expected, existing map[string]bool) []string {
	missing := make([]string, 0, len(expected))
	for component := range expected {
		if !existing[component] {
			missing = append(missing, component)
		}
	}
	slices.Sort(missing)
	return missing
}

func salesforceMetadataDeployArgs(target string, metadata []string) []string {
	args := []string{"project", "deploy", "start", "--target-org", target, "--json"}
	for _, component := range metadata {
		args = append(args, "--metadata", component)
	}
	return args
}

func salesforceMetadataPreviewArgs(target string, metadata []string) []string {
	args := []string{"project", "deploy", "preview", "--target-org", target, "--json"}
	for _, component := range metadata {
		args = append(args, "--metadata", component)
	}
	return args
}

func inspectSalesforceMetadataConflicts(project, target string, metadata []string) ([]string, error) {
	cmd := exec.Command("sf", salesforceMetadataPreviewArgs(target, metadata)...)
	cmd.Dir = project
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		// NonSourceTrackedOrgError means this is a persistent org or a scratch org
		// without source tracking. Since preview only detects source-tracking conflicts,
		// and those don't apply here, return an empty conflict list instead of failing.
		if strings.Contains(string(output), "NonSourceTrackedOrgError") {
			return nil, nil
		}
		return nil, salesforceorg.NewFailure("Unable to preview missing Salesforce metadata. Recheck the org connection and retry.", output, runErr)
	}
	conflicts, parseErr := readSalesforceMetadataConflicts(output)
	if parseErr != nil {
		return nil, salesforceorg.NewFailure("Salesforce CLI returned an unreadable metadata preview. Update the Salesforce CLI and retry.", output, parseErr)
	}
	return conflicts, nil
}

func readSalesforceMetadataConflicts(output []byte) ([]string, error) {
	var preview struct {
		Result struct {
			Conflicts []struct {
				Type     string `json:"type"`
				FullName string `json:"fullName"`
			} `json:"conflicts"`
		} `json:"result"`
	}
	if err := decodeSalesforceJSON(output, &preview); err != nil {
		return nil, err
	}
	conflicts := make([]string, 0, len(preview.Result.Conflicts))
	for _, conflict := range preview.Result.Conflicts {
		if strings.TrimSpace(conflict.Type) == "" || strings.TrimSpace(conflict.FullName) == "" {
			continue
		}
		conflicts = append(conflicts, conflict.Type+":"+conflict.FullName)
	}
	slices.Sort(conflicts)
	return conflicts, nil
}

func readSalesforceManifest(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg salesforcePackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse Salesforce manifest: %w", err)
	}
	result := map[string]bool{}
	for _, metadataType := range pkg.Types {
		for _, member := range metadataType.Members {
			result[metadataType.Name+":"+member] = true
		}
	}
	return result, nil
}

func readSalesforceInventory(output []byte, retrievedDir string) (map[string]bool, error) {
	var payload salesforceRetrieve
	if err := decodeSalesforceJSON(output, &payload); err != nil {
		return nil, fmt.Errorf("parse Salesforce inventory: %w", err)
	}
	result := map[string]bool{}
	for _, property := range payload.Result.FileProperties {
		if property.Type != "Package" {
			result[property.Type+":"+property.FullName] = true
		}
	}
	err := filepath.WalkDir(retrievedDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".object") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var object salesforceObject
		if unmarshalErr := xml.Unmarshal(data, &object); unmarshalErr != nil {
			return unmarshalErr
		}
		objectName := strings.TrimSuffix(entry.Name(), ".object")
		for _, field := range object.Fields {
			result["CustomField:"+objectName+"."+field.FullName] = true
		}
		return nil
	})
	return result, err
}

// decodeSalesforceJSON tolerates notices written before a JSON response. The
// Salesforce CLI currently prints update warnings to stderr even with --json;
// CombinedOutput places that warning before the JSON payload.
func decodeSalesforceJSON(output []byte, target any) error {
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return fmt.Errorf("Salesforce CLI output did not contain JSON")
	}
	return json.NewDecoder(bytes.NewReader(output[start:])).Decode(target)
}

func classifySalesforceInventory(item Item, expected, existing map[string]bool, alias string) Item {
	for component := range expected {
		if existing[component] {
			item.Present = append(item.Present, component)
		} else {
			item.Missing = append(item.Missing, component)
		}
	}
	slices.Sort(item.Present)
	slices.Sort(item.Missing)
	if len(item.Missing) == 0 {
		item.Status = StatusPresent
		item.Detail = fmt.Sprintf("All %d packaged metadata components already exist in Salesforce org %s. Nothing will be deployed.", len(item.Present), alias)
		return item
	}
	item.Status = StatusMissing
	item.Deployable = true
	item.DeployableComponents = append([]string(nil), item.Missing...)
	item.Detail = fmt.Sprintf("%d components already exist; %d need deployment to Salesforce org %s.", len(item.Present), len(item.Missing), alias)
	return item
}

type uiBundleConfig struct {
	OutputDir string `json:"outputDir"`
}

func buildSalesforceUIBundles(project string) error {
	if ignoreErr := ensureSalesforceProjectForceIgnore(project); ignoreErr != nil {
		return fmt.Errorf("prepare Salesforce deploy exclusions: %w", ignoreErr)
	}
	root := filepath.Join(project, "force-app", "main", "default", "uiBundles")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Salesforce UI Bundles: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle := filepath.Join(root, entry.Name())
		data, readErr := os.ReadFile(filepath.Join(bundle, "ui-bundle.json"))
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("read %s UI Bundle configuration: %w", entry.Name(), readErr)
		}
		var cfg uiBundleConfig
		if json.Unmarshal(data, &cfg) != nil || strings.TrimSpace(cfg.OutputDir) == "" {
			return fmt.Errorf("%s/ui-bundle.json does not define a valid outputDir", entry.Name())
		}
		outputDir := filepath.Join(bundle, filepath.FromSlash(cfg.OutputDir))
		if files, statErr := os.ReadDir(outputDir); statErr == nil && len(files) > 0 {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(bundle, "package.json")); statErr != nil {
			return fmt.Errorf("%s UI Bundle requires generated %s but has no package.json build contract", entry.Name(), cfg.OutputDir)
		}
		install := exec.Command("npm", "ci", "--no-audit", "--no-fund")
		install.Dir = bundle
		if output, runErr := install.CombinedOutput(); runErr != nil {
			return fmt.Errorf("prepare %s UI Bundle dependencies: %s", entry.Name(), summarizeCommandOutput(output, runErr))
		}
		build := exec.Command("npm", "run", "build")
		build.Dir = bundle
		if output, runErr := build.CombinedOutput(); runErr != nil {
			return fmt.Errorf("build %s UI Bundle: %s", entry.Name(), summarizeCommandOutput(output, runErr))
		}
		if files, statErr := os.ReadDir(outputDir); statErr != nil || len(files) == 0 {
			return fmt.Errorf("build completed but %s UI Bundle output is still missing: %s", entry.Name(), outputDir)
		}
	}
	return nil
}

// ensureSalesforceProjectForceIgnore mirrors the exclusion included by
// Salesforce's official project generator. UI Bundle dependencies are installed
// under force-app for local builds, but node_modules must not be packed into the
// Metadata API request (which has a 50 MB request limit).
func ensureSalesforceProjectForceIgnore(project string) error {
	path := filepath.Join(project, ".forceignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "node_modules/" {
			return nil
		}
	}
	if len(data) == 0 {
		data = []byte("# Exclude local UI Bundle build dependencies from Salesforce deploys.\n")
	} else if data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, []byte("node_modules/\n")...)
	return os.WriteFile(path, data, 0o644)
}

func summarizeSalesforceError(output []byte, runErr error) string {
	summary, _ := salesforceErrorDetails(output, runErr)
	return summary
}

func salesforceErrorDetails(output []byte, runErr error) (string, string) {
	return salesforceorg.CLIErrorDetails(output, runErr)
}

func summarizeCommandOutput(output []byte, runErr error) string {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	detail := strings.TrimSpace(strings.Join(lines, "\n"))
	if detail == "" {
		detail = runErr.Error()
	}
	return detail
}

func readReceipts(root string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(root, "config", "runtime", "validation-receipts.json"))
	if err != nil {
		return map[string]bool{}
	}
	var file receiptFile
	if json.Unmarshal(data, &file) != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, receipt := range file.Receipts {
		if strings.EqualFold(receipt.Status, "passed") {
			result[normalizeProvider(receipt.Platform)] = true
		}
	}
	return result
}

func providerPaths(root, provider string) []string {
	switch provider {
	case "box":
		return existing(root, "config/box")
	case "salesforce":
		paths := existing(root, "config/salesforce", "config/agentforce")
		if project := findSalesforceProject(root); project != "" {
			paths = append(paths, project)
		}
		return paths
	case "databricks":
		return existing(root, "config/databricks", "databricks", "infrastructure/databricks")
	case "aws":
		return existing(root, "config/agentcore", "infrastructure/agentcore", "infrastructure/aws")
	default:
		return nil
	}
}

func existing(root string, paths ...string) []string {
	result := []string{}
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Stat(full); err == nil {
			result = append(result, full)
		}
	}
	return result
}

func findSalesforceProject(root string) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "sfdx-project.json" {
			found = filepath.Dir(path)
		}
		return nil
	})
	return found
}

func countFiles(paths []string) int {
	count := 0
	for _, root := range paths {
		_ = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				count++
			}
			return nil
		})
	}
	return count
}

func relativePaths(root string, paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err == nil {
			result = append(result, rel)
		}
	}
	slices.Sort(result)
	return result
}

func normalizeProvider(name string) string {
	switch strings.ToLower(name) {
	case "salesforce", "agentforce":
		return "salesforce"
	case "agentcore", "aws", "aws bedrock agentcore":
		return "aws"
	default:
		return strings.ToLower(name)
	}
}

func providerName(provider string) string {
	switch provider {
	case "box":
		return "Box configuration"
	case "salesforce":
		return "Salesforce metadata"
	case "databricks":
		return "Databricks assets"
	case "aws":
		return "AWS Bedrock AgentCore specifications"
	default:
		return provider
	}
}
