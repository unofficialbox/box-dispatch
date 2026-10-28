package webapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/boxdiff"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

func TestAssembleAIAgentPromotionWritesMinimalPackage(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	agent := boxdiff.AIAgent{
		Key: "contract extractor", Name: "Contract Extractor", Product: "AI Studio custom agent", Capabilities: []string{"Extract"}, AccessState: "enabled",
		Configuration: map[string]any{"extract": map[string]any{"description": "Extract contract terms", "custom_instructions": "Return exact values", "long_text": map[string]any{"model": "model-a"}}},
	}
	comparison := boxdiff.AIAgentComparison{Left: boxdiff.Environment{EnterpriseID: "5105484"}, Right: boxdiff.Environment{EnterpriseID: "1509916197"}}
	plan, err := assembleAIAgentPromotion("Contract agents", comparison, []boxdiff.AIAgent{agent}, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TemplateID != aiAgentPromotionTemplateID || plan.Repository != "box://enterprise/5105484/ai-agents" || plan.Strategy != solution.StrategyReuse {
		t.Fatalf("plan = %#v", plan)
	}
	manifest, err := solution.Load(plan.PackagePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Box.Capabilities) != 1 || manifest.Box.Capabilities[0].ComponentType != "AI Agent" {
		t.Fatalf("capabilities = %#v", manifest.Box.Capabilities)
	}
	configPath := filepath.Join(plan.PackagePath, "config", "box", "ai-agent-specs.bcl")
	doc, err := bcl.LoadBCL(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Resources) != 1 {
		t.Fatalf("resources = %#v", doc.Resources)
	}
	object, err := bcl.ConfigObject(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var packagedAgents []struct {
		Name          string         `json:"name"`
		Mode          string         `json:"mode"`
		Configuration map[string]any `json:"configuration"`
	}
	if err := json.Unmarshal(object["agents"], &packagedAgents); err != nil {
		t.Fatal(err)
	}
	if len(packagedAgents) != 1 || packagedAgents[0].Name != "Contract Extractor" || packagedAgents[0].Mode != "extract" {
		t.Fatalf("packaged agents = %s", object["agents"])
	}
	encoded, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Contract Extractor", "custom_instructions", "model-a"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("agent package %s does not include %q", encoded, want)
		}
	}
	if _, err := os.Stat(filepath.Join(plan.PackagePath, ".dispatch", "deployment.bcl")); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleMetadataTemplatePromotionWritesMinimalPackage(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	template := boxdiff.MetadataTemplate{
		Key: "contracts", DisplayName: "Contracts", Hidden: true, CopyInstanceOnItemCopy: true,
		Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum", DisplayName: "Status", Description: "Contract status", Hidden: true, Options: []string{"Draft", "Approved"}}},
	}
	comparison := boxdiff.Comparison{Left: boxdiff.Environment{EnterpriseID: "5105484"}, Right: boxdiff.Environment{EnterpriseID: "1509916197"}}
	plan, err := assembleMetadataTemplatePromotion("Contract metadata", comparison, []boxdiff.MetadataTemplate{template}, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TemplateID != metadataTemplatePromotionTemplateID || plan.Repository != "box://enterprise/5105484/metadata-templates" || plan.Strategy != solution.StrategyReuse {
		t.Fatalf("plan = %#v", plan)
	}
	manifest, err := solution.Load(plan.PackagePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Box.Capabilities) != 1 || manifest.Box.Capabilities[0].ComponentType != "Metadata Template" {
		t.Fatalf("capabilities = %#v", manifest.Box.Capabilities)
	}
	configPath := filepath.Join(plan.PackagePath, "config", "box", "metadata-templates.bcl")
	object, err := bcl.ConfigObject(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var packaged []struct {
		TemplateKey            string `json:"templateKey"`
		Hidden                 bool   `json:"hidden"`
		CopyInstanceOnItemCopy bool   `json:"copyInstanceOnItemCopy"`
		Fields                 []struct {
			Description string `json:"description"`
			Hidden      bool   `json:"hidden"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(object["templates"], &packaged); err != nil {
		t.Fatal(err)
	}
	if len(packaged) != 1 || packaged[0].TemplateKey != "contracts" || !packaged[0].Hidden || !packaged[0].CopyInstanceOnItemCopy || len(packaged[0].Fields) != 1 || packaged[0].Fields[0].Description != "Contract status" || !packaged[0].Fields[0].Hidden {
		t.Fatalf("packaged templates = %s", object["templates"])
	}
}

func TestSelectPromotionTemplatesAcceptsPortableChangedAndSourceOnlyTemplates(t *testing.T) {
	changed := boxdiff.MetadataTemplate{Key: "contracts", DisplayName: "Contracts", Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum"}}}
	added := boxdiff.MetadataTemplate{Key: "retention", DisplayName: "Retention", Fields: []boxdiff.MetadataTemplateField{{Key: "years", Type: "float"}}}
	comparison := boxdiff.Comparison{Templates: []boxdiff.MetadataTemplateDiff{
		{Key: changed.Key, Status: "changed", Left: &changed, Right: &changed},
		{Key: added.Key, Status: "left_only", Left: &added},
	}}
	selected, err := selectPromotionTemplates(comparison, []string{"contracts", "retention"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Key != "contracts" || selected[1].Key != "retention" {
		t.Fatalf("selected templates = %#v", selected)
	}
}

func TestSelectPromotionTemplatesRejectsEnterpriseSpecificTemplates(t *testing.T) {
	for name, field := range map[string]boxdiff.MetadataTemplateField{
		"taxonomy key":      {Key: "class", Type: "string", TaxonomyKey: "enterprise-specific"},
		"selectable levels": {Key: "class", Type: "string", SelectableLevels: []int64{1, 2}},
	} {
		t.Run(name, func(t *testing.T) {
			taxonomy := boxdiff.MetadataTemplate{Key: "taxonomy", Fields: []boxdiff.MetadataTemplateField{field}}
			comparison := boxdiff.Comparison{Templates: []boxdiff.MetadataTemplateDiff{
				{Key: taxonomy.Key, Status: "left_only", Left: &taxonomy},
			}}
			if _, err := selectPromotionTemplates(comparison, []string{"taxonomy"}); err == nil {
				t.Fatal("enterprise-specific metadata template was accepted")
			}
		})
	}
}

func TestSelectPromotionTemplatesRejectsChangedTemplateThatRequiresDestructiveRemoval(t *testing.T) {
	desired := boxdiff.MetadataTemplate{Key: "contracts", Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum", Options: []string{"Draft"}}}}
	current := boxdiff.MetadataTemplate{Key: "contracts", Fields: []boxdiff.MetadataTemplateField{{Key: "status", Type: "enum", Options: []string{"Draft", "Legacy"}}}}
	comparison := boxdiff.Comparison{Templates: []boxdiff.MetadataTemplateDiff{{Key: desired.Key, Status: "changed", Left: &desired, Right: &current}}}
	if _, err := selectPromotionTemplates(comparison, []string{"contracts"}); err == nil {
		t.Fatal("destructive metadata template update was accepted")
	}
}

func TestSelectPromotionAgentsRejectsUnsupportedMode(t *testing.T) {
	agent := boxdiff.AIAgent{Key: "writer", Name: "Writer", Product: "AI Studio custom agent", Capabilities: []string{"Text generation"}}
	comparison := boxdiff.AIAgentComparison{Agents: []boxdiff.AIAgentDiff{{Key: agent.Key, Product: agent.Product, Capabilities: agent.Capabilities, Status: "left_only", Left: &agent}}}
	if _, err := selectPromotionAgents(comparison, []string{"writer"}); err == nil {
		t.Fatal("unsupported Text generation agent was accepted")
	}
}

func TestSelectPromotionAgentsRejectsRestrictedAudience(t *testing.T) {
	agent := boxdiff.AIAgent{Key: "legal", Name: "Legal", Product: "AI Studio custom agent", Capabilities: []string{"Ask"}, AllowedEntityTypes: []string{"group"}}
	comparison := boxdiff.AIAgentComparison{Agents: []boxdiff.AIAgentDiff{{Key: agent.Key, Product: agent.Product, Capabilities: agent.Capabilities, Status: "left_only", Left: &agent}}}
	if _, err := selectPromotionAgents(comparison, []string{"legal"}); err == nil {
		t.Fatal("agent with provider-specific audience principals was accepted")
	}
}
