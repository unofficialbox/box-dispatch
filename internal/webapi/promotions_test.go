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
