package boxdiff

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

func TestNormalizeAIAgentRemovesProviderResponseFields(t *testing.T) {
	var agent schemas.AiSingleAgentResponseFull
	if err := json.Unmarshal([]byte(`{
		"id":"12345","type":"ai_agent","origin":"CUSTOM","name":"Contract Extractor","access_state":"enabled",
		"created_at":"2026-10-26T12:00:00Z","icon_reference":"98765",
		"allowed_entities":[{"type":"user","id":"111"},{"type":"group","id":"222"}],
		"extract":{"type":"ai_agent_extract","access_state":"enabled","description":"Extract contract fields","custom_instructions":"Return exact values","long_text":{"model":"openai__gpt_4_1","prompt_template":"Read the contract","warnings":["provider warning"]}}
	}`), &agent); err != nil {
		t.Fatal(err)
	}

	normalized := NormalizeAIAgent(&agent)
	if normalized.Key != "contract extractor" || normalized.Name != "Contract Extractor" || normalized.Kind != "Extract" {
		t.Fatalf("unexpected identity: %+v", normalized)
	}
	if len(normalized.AllowedEntityTypes) != 2 || normalized.AllowedEntityTypes[0] != "group" || normalized.AllowedEntityTypes[1] != "user" {
		t.Fatalf("allowed entity types = %#v", normalized.AllowedEntityTypes)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"12345", "98765", "111", "222", "created_at", "provider warning", "warnings"} {
		if strings.Contains(string(encoded), excluded) {
			t.Fatalf("normalized agent retained %q: %s", excluded, encoded)
		}
	}
	for _, expected := range []string{"Return exact values", "openai__gpt_4_1", "Read the contract"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("normalized agent omitted %q: %s", expected, encoded)
		}
	}
}

func TestCompareAIAgentSnapshotsFindsSemanticChanges(t *testing.T) {
	started := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	leftEnvironment := Environment{ConnectionID: "left", Alias: "Production", EnterpriseID: "EID-left"}
	rightEnvironment := Environment{ConnectionID: "right", Alias: "Sandbox", EnterpriseID: "EID-right"}
	left := AIAgentSnapshot{Environment: leftEnvironment, Agents: []AIAgent{
		{Key: "contract extractor", Name: "Contract Extractor", Kind: "Extract", AccessState: "enabled", Configuration: map[string]any{"extract": map[string]any{"description": "Extract terms", "long_text": map[string]any{"model": "model-a"}}}},
		{Key: "legal ask", Name: "Legal Ask", Kind: "Ask", AccessState: "enabled", Configuration: map[string]any{"ask": map[string]any{"description": "Answer legal questions"}}},
	}}
	right := AIAgentSnapshot{Environment: rightEnvironment, Agents: []AIAgent{
		{Key: "contract extractor", Name: "Contract Extractor", Kind: "Extract", AccessState: "enabled", Configuration: map[string]any{"extract": map[string]any{"description": "Extract terms", "long_text": map[string]any{"model": "model-b"}}}},
		{Key: "summary writer", Name: "Summary Writer", Kind: "Text generation", AccessState: "enabled", Configuration: map[string]any{"textGen": map[string]any{"description": "Draft summaries"}}},
	}}

	comparison, err := CompareAIAgentSnapshots(started, left, right, started.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Summary != (Summary{Changed: 1, LeftOnly: 1, RightOnly: 1}) {
		t.Fatalf("summary = %+v", comparison.Summary)
	}
	if len(comparison.Agents) != 3 || comparison.Agents[0].Key != "contract extractor" || comparison.Agents[0].Status != "changed" {
		t.Fatalf("agents = %+v", comparison.Agents)
	}
	if len(comparison.Agents[0].Changes) != 1 || comparison.Agents[0].Changes[0].Path != "configuration.extract.long_text.model" {
		t.Fatalf("changes = %+v", comparison.Agents[0].Changes)
	}
}

func TestCompareAIAgentSnapshotsRejectsSameEnterprise(t *testing.T) {
	environment := Environment{EnterpriseID: "same"}
	_, err := CompareAIAgentSnapshots(time.Now(), AIAgentSnapshot{Environment: environment}, AIAgentSnapshot{Environment: environment}, time.Now())
	if err == nil {
		t.Fatal("expected same-enterprise comparison to fail")
	}
}

func TestCompareAIAgentSnapshotsReturnsEmptyInventory(t *testing.T) {
	started := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	comparison, err := CompareAIAgentSnapshots(started,
		AIAgentSnapshot{Environment: Environment{EnterpriseID: "left"}},
		AIAgentSnapshot{Environment: Environment{EnterpriseID: "right"}},
		started.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"agents":[]`) {
		t.Fatalf("empty inventory must be an array: %s", encoded)
	}
}
