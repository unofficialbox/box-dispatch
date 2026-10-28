package webapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/boxdiff"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

const (
	aiAgentPromotionTemplateID          = "ai-agent-promotion"
	metadataTemplatePromotionTemplateID = "metadata-template-promotion"
)

type aiAgentPromotionAssembler func(string, boxdiff.AIAgentComparison, []boxdiff.AIAgent, time.Time) (config.SolutionPlan, error)
type metadataTemplatePromotionAssembler func(string, boxdiff.Comparison, []boxdiff.MetadataTemplate, time.Time) (config.SolutionPlan, error)

type aiAgentPromotionRequest struct {
	Name              string   `json:"name"`
	LeftConnectionID  string   `json:"leftConnectionId"`
	RightConnectionID string   `json:"rightConnectionId"`
	AgentKeys         []string `json:"agentKeys"`
}

type metadataTemplatePromotionRequest struct {
	Name              string   `json:"name"`
	LeftConnectionID  string   `json:"leftConnectionId"`
	RightConnectionID string   `json:"rightConnectionId"`
	TemplateKeys      []string `json:"templateKeys"`
}

func (r metadataTemplatePromotionRequest) normalized() metadataTemplatePromotionRequest {
	r.Name = normalizeDeploymentName(r.Name)
	r.LeftConnectionID = strings.TrimSpace(r.LeftConnectionID)
	r.RightConnectionID = strings.TrimSpace(r.RightConnectionID)
	for index := range r.TemplateKeys {
		r.TemplateKeys[index] = strings.TrimSpace(r.TemplateKeys[index])
	}
	return r
}

func (r metadataTemplatePromotionRequest) validate() error {
	if r.Name == "" {
		return fmt.Errorf("enter a deployment name")
	}
	if utf8.RuneCountInString(r.Name) > 80 {
		return fmt.Errorf("deployment name must be 80 characters or fewer")
	}
	if r.LeftConnectionID == "" || r.RightConnectionID == "" {
		return fmt.Errorf("select two saved Box connections")
	}
	if r.LeftConnectionID == r.RightConnectionID {
		return fmt.Errorf("select two different Box connections")
	}
	if len(r.TemplateKeys) == 0 {
		return fmt.Errorf("select at least one changed or source-only metadata template")
	}
	seen := map[string]bool{}
	for _, key := range r.TemplateKeys {
		if key == "" {
			return fmt.Errorf("metadata template keys cannot be empty")
		}
		if seen[key] {
			return fmt.Errorf("metadata template %q is selected more than once", key)
		}
		seen[key] = true
	}
	return nil
}

func (r aiAgentPromotionRequest) normalized() aiAgentPromotionRequest {
	r.Name = normalizeDeploymentName(r.Name)
	r.LeftConnectionID = strings.TrimSpace(r.LeftConnectionID)
	r.RightConnectionID = strings.TrimSpace(r.RightConnectionID)
	for index := range r.AgentKeys {
		r.AgentKeys[index] = strings.ToLower(strings.TrimSpace(r.AgentKeys[index]))
	}
	return r
}

func (r aiAgentPromotionRequest) validate() error {
	if r.Name == "" {
		return fmt.Errorf("enter a deployment name")
	}
	if utf8.RuneCountInString(r.Name) > 80 {
		return fmt.Errorf("deployment name must be 80 characters or fewer")
	}
	if r.LeftConnectionID == "" || r.RightConnectionID == "" {
		return fmt.Errorf("select two saved Box connections")
	}
	if r.LeftConnectionID == r.RightConnectionID {
		return fmt.Errorf("select two different Box connections")
	}
	if len(r.AgentKeys) == 0 {
		return fmt.Errorf("select at least one source-only AI agent")
	}
	seen := map[string]bool{}
	for _, key := range r.AgentKeys {
		if key == "" {
			return fmt.Errorf("AI agent keys cannot be empty")
		}
		if seen[key] {
			return fmt.Errorf("AI agent %q is selected more than once", key)
		}
		seen[key] = true
	}
	return nil
}

func selectPromotionAgents(comparison boxdiff.AIAgentComparison, keys []string) ([]boxdiff.AIAgent, error) {
	available := make(map[string]boxdiff.AIAgent, len(comparison.Agents))
	for _, diff := range comparison.Agents {
		if diff.Status == "left_only" && diff.Left != nil && supportedAIAgentPromotion(*diff.Left) {
			available[strings.ToLower(diff.Key)] = *diff.Left
		}
	}
	agents := make([]boxdiff.AIAgent, 0, len(keys))
	for _, key := range keys {
		agent, ok := available[strings.ToLower(key)]
		if !ok {
			return nil, fmt.Errorf("AI agent %q is no longer eligible for promotion; rerun the live comparison", key)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func selectPromotionTemplates(comparison boxdiff.Comparison, keys []string) ([]boxdiff.MetadataTemplate, error) {
	available := make(map[string]boxdiff.MetadataTemplate, len(comparison.Templates))
	for _, diff := range comparison.Templates {
		if supportedMetadataTemplateDiffPromotion(diff) {
			available[diff.Key] = *diff.Left
		}
	}
	templates := make([]boxdiff.MetadataTemplate, 0, len(keys))
	for _, key := range keys {
		template, ok := available[key]
		if !ok {
			return nil, fmt.Errorf("metadata template %q is no longer eligible for promotion; rerun the live comparison", key)
		}
		templates = append(templates, template)
	}
	return templates, nil
}

func supportedMetadataTemplateDiffPromotion(diff boxdiff.MetadataTemplateDiff) bool {
	if diff.Left == nil || !supportedMetadataTemplatePromotion(*diff.Left) {
		return false
	}
	if diff.Status == "left_only" {
		return true
	}
	if diff.Status != "changed" || diff.Right == nil {
		return false
	}
	desiredFields := make(map[string]boxdiff.MetadataTemplateField, len(diff.Left.Fields))
	for _, field := range diff.Left.Fields {
		desiredFields[field.Key] = field
	}
	for _, current := range diff.Right.Fields {
		desired, ok := desiredFields[current.Key]
		if !ok || desired.Type != current.Type {
			return false
		}
		desiredOptions := make(map[string]bool, len(desired.Options))
		for _, option := range desired.Options {
			desiredOptions[option] = true
		}
		for _, option := range current.Options {
			if !desiredOptions[option] {
				return false
			}
		}
	}
	return true
}

func supportedMetadataTemplatePromotion(template boxdiff.MetadataTemplate) bool {
	for _, field := range template.Fields {
		if field.TaxonomyKey != "" || len(field.SelectableLevels) > 0 {
			return false
		}
		switch field.Type {
		case "string", "float", "date", "enum", "multiSelect":
		default:
			// Taxonomy fields reference enterprise-specific objects and cannot be
			// copied safely from one Box enterprise to another.
			return false
		}
	}
	return true
}

func supportedAIAgentPromotion(agent boxdiff.AIAgent) bool {
	if len(agent.AllowedEntityTypes) > 0 {
		// The live diff intentionally strips user/group IDs. Promoting an agent
		// without those principals would silently broaden its audience.
		return false
	}
	_, ok := aiAgentPromotionMode(agent)
	return ok
}

func aiAgentPromotionMode(agent boxdiff.AIAgent) (string, bool) {
	if len(agent.Capabilities) != 1 {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(agent.Capabilities[0])) {
	case "ask":
		return "ask", true
	case "extract":
		return "extract", true
	default:
		return "", false
	}
}

func assembleAIAgentPromotion(name string, comparison boxdiff.AIAgentComparison, agents []boxdiff.AIAgent, now time.Time) (plan config.SolutionPlan, err error) {
	stamp := now.UTC().Format("20060102-150405.000000000")
	destination := filepath.Join(config.Paths().Root, ".box-dispatch", "web-packages", aiAgentPromotionTemplateID+"-"+stamp)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return config.SolutionPlan{}, fmt.Errorf("create AI agent promotion package: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()

	enabled := true
	manifest := solution.Manifest{
		SchemaVersion:    "1.0",
		TemplateID:       aiAgentPromotionTemplateID,
		DeploymentConfig: ".dispatch/deployment.bcl",
		Box: solution.BoxManifest{
			DeploymentDefaults: solution.DefaultDeploymentSettings().Box,
			ComponentOrder:     []string{"AI Agent"},
			Capabilities: []solution.Capability{{
				ID: "ai_agent", ComponentType: "AI Agent", EnabledByDefault: &enabled,
				API: "public", Handler: "box.ai-agent", Operations: []string{"validate", "deploy"},
			}},
		},
	}
	if err := solution.WriteManifest(destination, manifest); err != nil {
		return config.SolutionPlan{}, err
	}
	if err := solution.WriteDeploymentSettings(destination, manifest.DeploymentConfig, solution.DefaultDeploymentSettings()); err != nil {
		return config.SolutionPlan{}, err
	}

	payload := make([]any, 0, len(agents))
	for _, agent := range agents {
		mode, ok := aiAgentPromotionMode(agent)
		if !ok {
			return config.SolutionPlan{}, fmt.Errorf("AI Studio custom agent %q has unsupported capabilities", agent.Name)
		}
		payload = append(payload, map[string]any{
			"key": agent.Key, "name": agent.Name, "mode": mode,
			"accessState": agent.AccessState, "configuration": agent.Configuration,
		})
	}
	doc := bcl.NewDocument(aiAgentPromotionTemplateID, "generic", "box-ai-agent-promotion", now.UTC().Format(time.RFC3339), map[string]any{
		"sourceEnterpriseId":      comparison.Left.EnterpriseID,
		"destinationEnterpriseId": comparison.Right.EnterpriseID,
	})
	doc.Resources = []bcl.BCLResource{{
		Name: "config_box_ai-agent-specs", Type: "config_file", TerraformType: "null_resource", Provider: "generic",
		Config: map[string]any{
			"artifact_name": "ai-agent-specs", "artifact_type": "config", "provider": "generic",
			"provider_object_id": "box/ai-agent-specs", "enterprise_id": comparison.Left.EnterpriseID,
			"created_at": now.UTC().Format(time.RFC3339), "agents": payload,
		},
		Metadata: map[string]any{
			"artifactName": "ai-agent-specs", "artifactType": "config_file", "provider": "generic",
			"providerObjectId": "box/ai-agent-specs", "enterpriseId": comparison.Left.EnterpriseID,
			"scenario": aiAgentPromotionTemplateID, "createdAt": now.UTC().Format(time.RFC3339),
		},
	}}
	if err := bcl.WriteBCL(filepath.Join(destination, "config", "box", "ai-agent-specs.bcl"), doc); err != nil {
		return config.SolutionPlan{}, err
	}

	return config.SolutionPlan{
		Name: name, TemplateID: aiAgentPromotionTemplateID, Template: "AI agent promotion",
		Repository: "box://enterprise/" + comparison.Left.EnterpriseID + "/ai-agents",
		Components: []string{"box"}, Strategy: solution.StrategyReuse, PackagePath: destination,
	}, nil
}

func assembleMetadataTemplatePromotion(name string, comparison boxdiff.Comparison, templates []boxdiff.MetadataTemplate, now time.Time) (plan config.SolutionPlan, err error) {
	stamp := now.UTC().Format("20060102-150405.000000000")
	destination := filepath.Join(config.Paths().Root, ".box-dispatch", "web-packages", metadataTemplatePromotionTemplateID+"-"+stamp)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return config.SolutionPlan{}, fmt.Errorf("create metadata template promotion package: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()

	enabled := true
	manifest := solution.Manifest{
		SchemaVersion:    "1.0",
		TemplateID:       metadataTemplatePromotionTemplateID,
		DeploymentConfig: ".dispatch/deployment.bcl",
		Box: solution.BoxManifest{
			DeploymentDefaults: solution.DefaultDeploymentSettings().Box,
			ComponentOrder:     []string{"Metadata Template"},
			Capabilities: []solution.Capability{{
				ID: "metadata_template", ComponentType: "Metadata Template", EnabledByDefault: &enabled,
				API: "public", Handler: "box.metadata-template", Operations: []string{"validate", "deploy"},
			}},
		},
	}
	if err := solution.WriteManifest(destination, manifest); err != nil {
		return config.SolutionPlan{}, err
	}
	if err := solution.WriteDeploymentSettings(destination, manifest.DeploymentConfig, solution.DefaultDeploymentSettings()); err != nil {
		return config.SolutionPlan{}, err
	}

	payload := make([]any, 0, len(templates))
	for _, template := range templates {
		fields := make([]any, 0, len(template.Fields))
		for _, field := range template.Fields {
			fields = append(fields, map[string]any{
				"type": field.Type, "key": field.Key, "displayName": field.DisplayName,
				"description": field.Description, "hidden": field.Hidden, "options": field.Options,
			})
		}
		payload = append(payload, map[string]any{
			"templateKey": template.Key, "displayName": template.DisplayName,
			"hidden": template.Hidden, "copyInstanceOnItemCopy": template.CopyInstanceOnItemCopy,
			"fields": fields,
		})
	}
	doc := bcl.NewDocument(metadataTemplatePromotionTemplateID, "generic", "box-metadata-template-promotion", now.UTC().Format(time.RFC3339), map[string]any{
		"sourceEnterpriseId":      comparison.Left.EnterpriseID,
		"destinationEnterpriseId": comparison.Right.EnterpriseID,
	})
	doc.Resources = []bcl.BCLResource{{
		Name: "config_box_metadata-templates", Type: "config_file", TerraformType: "null_resource", Provider: "generic",
		Config: map[string]any{
			"artifact_name": "metadata-templates", "artifact_type": "config", "provider": "generic",
			"provider_object_id": "box/metadata-templates", "enterprise_id": comparison.Left.EnterpriseID,
			"created_at": now.UTC().Format(time.RFC3339), "templates": payload,
		},
		Metadata: map[string]any{
			"artifactName": "metadata-templates", "artifactType": "config_file", "provider": "generic",
			"providerObjectId": "box/metadata-templates", "enterpriseId": comparison.Left.EnterpriseID,
			"scenario": metadataTemplatePromotionTemplateID, "createdAt": now.UTC().Format(time.RFC3339),
		},
	}}
	if err := bcl.WriteBCL(filepath.Join(destination, "config", "box", "metadata-templates.bcl"), doc); err != nil {
		return config.SolutionPlan{}, err
	}

	return config.SolutionPlan{
		Name: name, TemplateID: metadataTemplatePromotionTemplateID, Template: "Metadata template promotion",
		Repository: "box://enterprise/" + comparison.Left.EnterpriseID + "/metadata-templates",
		Components: []string{"box"}, Strategy: solution.StrategyReuse, PackagePath: destination,
	}, nil
}
