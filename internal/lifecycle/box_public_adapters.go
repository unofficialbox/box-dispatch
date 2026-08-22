package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type boxAIResource struct {
	Component    string
	Mode         string
	Name         string
	Description  string
	Instructions string
}

func validateBoxPublicAdapters(ctx context.Context, root string, manifest solution.Manifest, selection solution.ComponentSelection, api boxAPI, workspaceID string, item *Item, report Reporter, components []string) error {
	if manifest.CapabilityEnabled("Doc Gen Template", selection) {
		registered, err := api.docgenTemplateFileIDs(ctx)
		if err != nil {
			return fmt.Errorf("inspect Box Doc Gen templates: %w", err)
		}
		for _, file := range manifest.Box.SampleContent {
			if !strings.EqualFold(filepath.Ext(file.Source), ".docx") {
				continue
			}
			component := "Doc Gen Template:" + filepath.Base(file.Source)
			report.component(component, ProgressRunning, "Inspecting the Doc Gen template", componentIndex(components, component), len(components))
			fileID, found, err := findWorkspaceFile(ctx, api, workspaceID, file.TargetFolder, filepath.Base(file.Source))
			if err != nil {
				return fmt.Errorf("inspect %s: %w", component, err)
			}
			present := found && registered[fileID]
			deployable := found && !registered[fileID]
			if !found && manifest.CapabilityEnabled("Sample Content", selection) {
				source := filepath.Join(root, filepath.FromSlash(file.Source))
				if info, sourceErr := os.Stat(source); sourceErr == nil && !info.IsDir() {
					deployable = true
				}
			}
			classifyBoxComponent(item, component, present, deployable)
			report.component(component, ProgressCompleted, validationResultMessage(present, deployable), componentIndex(components, component)+1, len(components))
		}
	}

	agents, err := readBoxAIResources(root, manifest, selection)
	if err != nil {
		return err
	}
	if len(agents) > 0 {
		existing, err := api.aiAgentNames(ctx)
		if err != nil {
			return fmt.Errorf("inspect Box AI Studio agents: %w", err)
		}
		for _, agent := range agents {
			present := existing[agent.Name]
			report.component(agent.Component, ProgressRunning, "Inspecting the Box AI agent", componentIndex(components, agent.Component), len(components))
			classifyBoxComponent(item, agent.Component, present, !present)
			report.component(agent.Component, ProgressCompleted, validationResultMessage(present, !present), componentIndex(components, agent.Component)+1, len(components))
		}
	}

	if capability, enabled := enabledCapability(manifest, selection, "Box Hub"); enabled {
		component := "Box Hub:" + capability.DisplayName
		report.component(component, ProgressRunning, "Inspecting the Box hub", componentIndex(components, component), len(components))
		existing, err := api.hubTitles(ctx)
		switch {
		case isBoxPermissionError(err):
			// Hub listing is forbidden for this Box user or app. Without it we
			// cannot tell whether the hub already exists, and creating one blindly
			// would duplicate it, so leave it for manual handling.
			classifyBoxComponent(item, component, false, false)
			item.Detail = "Box Hub could not be inspected (permission denied); create or verify it manually."
		case err != nil:
			return fmt.Errorf("inspect Box Hubs: %w", err)
		default:
			classifyBoxComponent(item, component, existing[capability.DisplayName], !existing[capability.DisplayName])
		}
		present := slices.Contains(item.Present, component)
		deployable := slices.Contains(item.DeployableComponents, component)
		report.component(component, ProgressCompleted, validationResultMessage(present, deployable), componentIndex(components, component)+1, len(components))
	}

	if manifest.CapabilityEnabled("Automate Workflow", selection) && workspaceID != "" {
		existing, err := api.automateWorkflowNames(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("inspect Box Automate workflows: %w", err)
		}
		for _, component := range componentsOfType(item.Missing, "Automate Workflow") {
			name := strings.TrimPrefix(component, "Automate Workflow:")
			report.component(component, ProgressRunning, "Inspecting the Box Automate workflow", componentIndex(components, component), len(components))
			classifyBoxComponent(item, component, existing[name], false)
			report.component(component, ProgressCompleted, validationResultMessage(existing[name], false), componentIndex(components, component)+1, len(components))
		}
	}
	return nil
}

func deployBoxPublicAdapters(ctx context.Context, root string, manifest solution.Manifest, selection solution.ComponentSelection, api boxAPI, workspaceID string, deployable []string, report Reporter) ([]string, []ResourceReference, error) {
	deployed := []string{}
	resources := []ResourceReference{}
	if manifest.CapabilityEnabled("Doc Gen Template", selection) {
		for _, file := range manifest.Box.SampleContent {
			component := "Doc Gen Template:" + filepath.Base(file.Source)
			if !strings.EqualFold(filepath.Ext(file.Source), ".docx") || !slices.Contains(deployable, component) {
				continue
			}
			fileID, found, err := findWorkspaceFile(ctx, api, workspaceID, file.TargetFolder, filepath.Base(file.Source))
			if err != nil || !found {
				return deployed, resources, fmt.Errorf("locate %s before Doc Gen registration: %w", component, err)
			}
			report.step("Registering Doc Gen template " + filepath.Base(file.Source))
			if err := api.createDocgenTemplate(ctx, fileID); err != nil {
				return deployed, resources, fmt.Errorf("register %s: %w", component, err)
			}
			deployed = append(deployed, component)
			resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "docgen_template", Name: filepath.Base(file.Source), ID: fileID, URL: "https://app.box.com/file/" + fileID})
		}
	}
	agents, err := readBoxAIResources(root, manifest, selection)
	if err != nil {
		return deployed, resources, err
	}
	for _, agent := range agents {
		if !slices.Contains(deployable, agent.Component) {
			continue
		}
		report.step("Creating AI agent " + agent.Name)
		agentID, err := api.createAIAgent(ctx, agent.Mode, agent.Name, agent.Description, agent.Instructions)
		if err != nil {
			return deployed, resources, fmt.Errorf("create %s: %w", agent.Component, err)
		}
		deployed = append(deployed, agent.Component)
		// Recorded so teardown can delete the agent by ID.
		resources = append(resources, ResourceReference{Provider: "box", Component: agent.Component, Kind: "ai_agent", Name: agent.Name, ID: agentID})
	}
	if capability, enabled := enabledCapability(manifest, selection, "Box Hub"); enabled {
		component := "Box Hub:" + capability.DisplayName
		if slices.Contains(deployable, component) {
			report.step("Creating Box hub " + capability.DisplayName)
			description := "Solution hub provisioned by Box Dispatch from " + capability.Source
			hubID, err := api.createHub(ctx, capability.DisplayName, description)
			if err != nil {
				return deployed, resources, fmt.Errorf("create %s: %w", component, err)
			}
			deployed = append(deployed, component)
			resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "hub", Name: capability.DisplayName, ID: hubID})
		}
	}
	return deployed, resources, nil
}

func findWorkspaceFile(ctx context.Context, api boxAPI, workspaceID, folderName, fileName string) (string, bool, error) {
	if workspaceID == "" {
		return "", false, nil
	}
	folderID, found, err := api.findFolder(ctx, workspaceID, folderName)
	if err != nil || !found {
		return "", false, err
	}
	return api.findFile(ctx, folderID, fileName)
}

func readBoxAIResources(root string, manifest solution.Manifest, selection solution.ComponentSelection) ([]boxAIResource, error) {
	resources := []boxAIResource{}
	configRoot := filepath.Join(root, "config", "box")
	if manifest.CapabilityEnabled("AI Agent", selection) {
		object, err := readBoxConfigObject(configRoot, "ai-agent-specs")
		if err != nil {
			return nil, err
		}
		var agents []struct {
			Name         string `json:"name"`
			Instructions string `json:"instructions"`
		}
		if raw := object["agents"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &agents); err != nil {
				return nil, fmt.Errorf("parse ai-agent-specs: %w", err)
			}
		}
		for _, agent := range agents {
			resources = append(resources, boxAIResource{Component: "AI Agent:" + agent.Name, Mode: "ask", Name: agent.Name, Description: agent.Name, Instructions: agent.Instructions})
		}
	}
	if manifest.CapabilityEnabled("Extract Configuration", selection) {
		document, err := readBoxConfigObject(configRoot, "extract-field-prompts")
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(document))
		for key := range document {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			var value struct {
				Purpose string `json:"purpose"`
			}
			_ = json.Unmarshal(document[key], &value)
			name := "CLM " + humanizeIdentifier(key) + " Extract"
			resources = append(resources, boxAIResource{Component: "Extract Configuration:" + key, Mode: "extract", Name: name, Description: firstNonEmpty(value.Purpose, name), Instructions: string(document[key])})
		}
	}
	return resources, nil
}

func humanizeIdentifier(value string) string {
	var out []rune
	for i, r := range []rune(value) {
		if i > 0 && unicode.IsUpper(r) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	words := strings.Fields(string(out))
	for i := range words {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

func enabledCapability(manifest solution.Manifest, selection solution.ComponentSelection, componentType string) (solution.Capability, bool) {
	capability, found := manifest.Capability(componentType)
	return capability, found && manifest.CapabilityEnabled(componentType, selection)
}

func componentsOfType(components []string, componentType string) []string {
	prefix := componentType + ":"
	return slices.Collect(func(yield func(string) bool) {
		for _, component := range components {
			if strings.HasPrefix(component, prefix) && !yield(component) {
				return
			}
		}
	})
}
