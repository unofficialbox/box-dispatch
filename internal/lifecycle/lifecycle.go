package lifecycle

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/config"
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
		item, err := ValidateProvider(root, provider)
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

func ValidateProvider(root, provider string) (Item, error) {
	if root == "" {
		return Item{}, fmt.Errorf("package directory is required")
	}
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
		return validateSalesforce(root, item)
	}
	if provider == "box" {
		return validateBox(root, item, components)
	}
	item.Status = StatusManual
	item.Detail = fmt.Sprintf("%d packaged configuration files require provider validation. A native Windlass deploy adapter is not available yet.", count)
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
	if count == 0 {
		item.Planned = []string{"Provider configuration"}
		return item, nil
	}
	components, err := providerComponentEntries(root, provider, count)
	if err != nil {
		return item, err
	}
	item.Planned = components
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

func boxComponentEntries(root string, manifest solution.Manifest, selection solution.ComponentSelection) ([]string, error) {
	entries := []string{}
	arrays := []struct {
		file, key, componentType string
	}{
		{"ai-agent-specs.json", "agents", "AI Agent"},
		{"automate-workflows.json", "workflows", "Automate Workflow"},
		{"https-connectors.json", "connectors", "HTTPS Connector"},
		{"metadata-templates.json", "templates", "Metadata Template"},
	}
	for _, spec := range arrays {
		if !manifest.CapabilityEnabled(spec.componentType, selection) {
			continue
		}
		data, err := readJSONObject(filepath.Join(root, spec.file))
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
		{"extract-field-prompts.json", "Extract Configuration"},
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
		data, err := readJSONObject(filepath.Join(root, spec.file))
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
	if _, err := os.Stat(filepath.Join(root, "folder-template.md")); err == nil && manifest.CapabilityEnabled(manifest.Box.Workspace.ComponentType, selection) {
		entries = append(entries, manifest.Box.Workspace.ComponentType+":"+manifest.Box.Workspace.DisplayName)
	} else if !os.IsNotExist(err) {
		return nil, err
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

func validateBox(root string, item Item, components []string) (Item, error) {
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
	if folderTargetConfigured {
		present, inspectErr := boxFolderStructureExists(ctx, api, target, workspace.Children)
		if inspectErr != nil {
			item.Status, item.Detail = StatusFailed, "Unable to inspect the Box folder hierarchy: "+inspectErr.Error()
			return item, nil
		}
		if folderEnabled {
			classifyBoxComponent(&item, workspaceComponent, present, !present)
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
		}
	}
	if manifest.CapabilityEnabled("Sample Content", selection) {
		for _, file := range manifest.Box.SampleContent {
			component := "Sample Content:" + filepath.Base(file.Source)
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
					present, inspectErr = api.fileExists(ctx, folderID, filepath.Base(file.Source))
					if inspectErr != nil {
						item.Status, item.Detail = StatusFailed, "Unable to inspect Box sample content: "+inspectErr.Error()
						return item, nil
					}
				}
			}
			classifyBoxComponent(&item, component, present, !present)
		}
	}
	if publicErr := validateBoxPublicAdapters(ctx, root, manifest, selection, api, workspaceID, &item); publicErr != nil {
		item.Status, item.Detail = StatusFailed, publicErr.Error()
		return item, nil
	}
	if privateErr := validateBoxPrivateAdapters(root, manifest, settings.Box, selection, &item); privateErr != nil {
		item.Status, item.Detail = StatusFailed, privateErr.Error()
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
	return item, nil
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
	data, err := readJSONObject(filepath.Join(root, "config", "box", "metadata-templates.json"))
	if err != nil {
		return nil, err
	}
	var templates []boxMetadataTemplate
	if err := json.Unmarshal(data["templates"], &templates); err != nil {
		return nil, fmt.Errorf("parse metadata-templates.json: %w", err)
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
		results[i] = deployProvider(root, results[i], settings)
	}
	return results, nil
}

func DeployProvider(root string, item Item) (Item, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return item, err
	}
	return deployProvider(root, item, settings), nil
}

func deployProvider(root string, item Item, settings config.ConnectionSettings) Item {
	if item.Status != StatusMissing || !item.Deployable {
		return item
	}
	if item.Provider == "box" {
		return deployBoxFoundation(root, item)
	}
	if item.Provider != "salesforce" {
		return item
	}
	project := findSalesforceProject(root)
	if settings.SalesforceAlias == "" {
		item.Status, item.Detail = StatusFailed, "No Salesforce alias is selected."
		return item
	}
	if buildErr := buildSalesforceUIBundles(project); buildErr != nil {
		item.Status, item.Detail = StatusFailed, buildErr.Error()
		return item
	}
	cmd := exec.Command("sf", "project", "deploy", "start", "--source-dir", "force-app", "--target-org", settings.SalesforceAlias, "--json")
	cmd.Dir = project
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		item.Status = StatusFailed
		item.Detail = summarizeSalesforceError(output, runErr)
		return item
	}
	var deployResponse struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(output, &deployResponse)
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
	if instanceURL != "" && deployResponse.Result.ID != "" {
		deployURL = instanceURL + "/" + deployResponse.Result.ID
	}
	addResource(&item, "Salesforce metadata", "metadata_deployment", "Salesforce metadata deployment", deployResponse.Result.ID, deployURL)
	item.Status, item.Detail = StatusPresent, "Salesforce metadata deployed successfully."
	item.Present = append(item.Present, item.Missing...)
	slices.Sort(item.Present)
	item.Missing = nil
	return item
}

func deployBoxFoundation(root string, item Item) Item {
	manifest, err := solution.Load(root)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	workspace := manifest.Box.Workspace
	settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	selection := settings.Box.Components
	target, err := loadBoxTarget(root, workspace.Name)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	resolvedWorkspaceName, err := solution.ResolveDeploymentName(target.WorkspaceName, settings.Box)
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	target.WorkspaceName = resolvedWorkspaceName
	api, err := newBoxAPI()
	if err != nil {
		item.Status, item.Detail = StatusFailed, err.Error()
		return item
	}
	ctx := context.Background()
	deployed := []string{}
	folderComponent := workspace.ComponentType + ":" + workspace.DisplayName
	if target.ParentFolderID == "" {
		target.ParentFolderID = "0"
	}
	workspaceID := ""
	if slices.Contains(item.DeployableComponents, folderComponent) {
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
			exists, inspectErr := api.fileExists(ctx, folderID, filepath.Base(file.Source))
			if inspectErr != nil {
				item.Status, item.Detail = StatusFailed, "Inspect "+component+": "+inspectErr.Error()
				return item
			}
			if !exists {
				if uploadErr := api.uploadFile(ctx, folderID, source); uploadErr != nil {
					item.Status, item.Detail = StatusFailed, "Deploy "+component+": "+uploadErr.Error()
					return item
				}
			}
			fileID, found, fileErr := api.findFile(ctx, folderID, filepath.Base(file.Source))
			if fileErr != nil || !found {
				item.Status, item.Detail = StatusFailed, "Resolve deployed "+component+" ID"
				if fileErr != nil {
					item.Detail += ": " + fileErr.Error()
				}
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
	publicDeployed, publicResources, publicErr := deployBoxPublicAdapters(ctx, root, manifest, selection, api, workspaceID, item.DeployableComponents)
	if publicErr != nil {
		item.Status, item.Detail = StatusFailed, publicErr.Error()
		return item
	}
	deployed = append(deployed, publicDeployed...)
	item.Resources = append(item.Resources, publicResources...)
	privateDeployed, privateResources, privateErr := deployBoxPrivateAdapters(root, manifest, settings.Box, selection, item.DeployableComponents, workspaceID, item.Resources)
	if privateErr != nil {
		item.Status, item.Detail = StatusFailed, privateErr.Error()
		return item
	}
	deployed = append(deployed, privateDeployed...)
	item.Resources = append(item.Resources, privateResources...)
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

func validateSalesforce(root string, item Item) (Item, error) {
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return item, err
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
	if buildErr := buildSalesforceUIBundles(project); buildErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to prepare packaged Salesforce UI Bundles: "+buildErr.Error()
		return item, nil
	}
	temp, err := os.MkdirTemp("", "windlass-salesforce-validate-")
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
		item.Status, item.Detail = StatusFailed, "Unable to inventory packaged Salesforce metadata: "+summarizeSalesforceError(output, runErr)
		return item, nil
	}
	manifest := filepath.Join(manifestDir, "package.xml")
	expected, err := readSalesforceManifest(manifest)
	if err != nil {
		return item, err
	}
	retrievedDir := filepath.Join(temp, "retrieved")
	retrieve := exec.Command("sf", "project", "retrieve", "start", "--manifest", manifest, "--target-org", settings.SalesforceAlias, "--target-metadata-dir", retrievedDir, "--unzip", "--json")
	retrieve.Dir = project
	output, runErr := retrieve.CombinedOutput()
	if runErr != nil {
		item.Status, item.Detail = StatusFailed, "Unable to read Salesforce metadata: "+summarizeSalesforceError(output, runErr)
		return item, nil
	}
	existing, err := readSalesforceInventory(output, retrievedDir)
	if err != nil {
		return item, err
	}
	return classifySalesforceInventory(item, expected, existing, settings.SalesforceAlias), nil
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
	if err := json.Unmarshal(output, &payload); err != nil {
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

func summarizeSalesforceError(output []byte, runErr error) string {
	var payload struct {
		Name    string   `json:"name"`
		Message string   `json:"message"`
		Actions []string `json:"actions"`
	}
	if json.Unmarshal(output, &payload) == nil && payload.Message != "" {
		parts := []string{}
		if payload.Name != "" {
			parts = append(parts, payload.Name)
		}
		parts = append(parts, payload.Message)
		if len(payload.Actions) > 0 {
			parts = append(parts, "Next: "+strings.Join(payload.Actions, " "))
		}
		return strings.Join(parts, ": ")
	}
	return summarizeCommandOutput(output, runErr)
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
		return "Salesforce + Agentforce metadata"
	case "databricks":
		return "Databricks assets"
	case "aws":
		return "AWS Bedrock AgentCore specifications"
	default:
		return provider
	}
}
