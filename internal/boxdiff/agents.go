package boxdiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

// AIAgent is a provider-ID-free view of a Box AI Studio custom agent. An
// Extract capability is part of that custom-agent definition; it is not a Box
// Automate Extract Agent.
type AIAgent struct {
	Key                string         `json:"key"`
	Name               string         `json:"name"`
	Product            string         `json:"product"`
	Capabilities       []string       `json:"capabilities"`
	AccessState        string         `json:"accessState"`
	AllowedEntityTypes []string       `json:"allowedEntityTypes,omitempty"`
	Configuration      map[string]any `json:"configuration"`
}

type AIAgentSnapshot struct {
	Environment Environment `json:"environment"`
	Agents      []AIAgent   `json:"agents"`
}

type AIAgentDiff struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Product      string   `json:"product"`
	Capabilities []string `json:"capabilities"`
	Status       string   `json:"status"`
	Changes      []Change `json:"changes"`
	Left         *AIAgent `json:"left,omitempty"`
	Right        *AIAgent `json:"right,omitempty"`
}

type AIAgentComparison struct {
	StartedAt   string        `json:"startedAt"`
	CompletedAt string        `json:"completedAt"`
	Left        Environment   `json:"left"`
	Right       Environment   `json:"right"`
	Summary     Summary       `json:"summary"`
	Agents      []AIAgentDiff `json:"agents"`
}

func NormalizeAIAgent(agent *schemas.AiSingleAgentResponseFull) AIAgent {
	if agent == nil {
		return AIAgent{Configuration: map[string]any{}}
	}
	normalized := AIAgent{
		Key:           strings.ToLower(strings.TrimSpace(agent.Name)),
		Name:          strings.TrimSpace(agent.Name),
		Product:       "AI Studio custom agent",
		AccessState:   strings.TrimSpace(agent.AccessState),
		Configuration: map[string]any{},
	}
	capabilities := make([]string, 0, 3)
	if agent.Ask != nil {
		capabilities = append(capabilities, "Ask")
		normalized.Configuration["ask"] = normalizeAgentSection(agent.Ask)
	}
	if agent.Extract != nil {
		capabilities = append(capabilities, "Extract")
		normalized.Configuration["extract"] = normalizeAgentSection(agent.Extract)
	}
	if agent.TextGen != nil {
		capabilities = append(capabilities, "Text generation")
		normalized.Configuration["textGen"] = normalizeAgentSection(agent.TextGen)
	}
	normalized.Capabilities = capabilities
	for _, entity := range agent.AllowedEntities {
		switch {
		case entity.User != nil:
			normalized.AllowedEntityTypes = append(normalized.AllowedEntityTypes, "user")
		case entity.Group != nil:
			normalized.AllowedEntityTypes = append(normalized.AllowedEntityTypes, "group")
		default:
			normalized.AllowedEntityTypes = append(normalized.AllowedEntityTypes, "unknown")
		}
	}
	sort.Strings(normalized.AllowedEntityTypes)
	return normalized
}

func normalizeAgentSection(section any) map[string]any {
	data, err := json.Marshal(section)
	if err != nil {
		return map[string]any{}
	}
	var normalized map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return map[string]any{}
	}
	removeAgentResponseFields(normalized)
	return normalized
}

func removeAgentResponseFields(value any) {
	switch current := value.(type) {
	case map[string]any:
		delete(current, "warnings")
		for _, child := range current {
			removeAgentResponseFields(child)
		}
	case []any:
		for _, child := range current {
			removeAgentResponseFields(child)
		}
	}
}

func CompareAIAgentSnapshots(startedAt time.Time, left, right AIAgentSnapshot, completedAt time.Time) (AIAgentComparison, error) {
	if strings.TrimSpace(left.Environment.EnterpriseID) == "" || strings.TrimSpace(right.Environment.EnterpriseID) == "" {
		return AIAgentComparison{}, fmt.Errorf("both live Box connections must report an enterprise ID")
	}
	if left.Environment.EnterpriseID == right.Environment.EnterpriseID {
		return AIAgentComparison{}, fmt.Errorf("select Box connections from two different enterprises")
	}
	comparison := AIAgentComparison{
		StartedAt: startedAt.UTC().Format(time.RFC3339Nano), CompletedAt: completedAt.UTC().Format(time.RFC3339Nano),
		Left: left.Environment, Right: right.Environment, Agents: []AIAgentDiff{},
	}
	leftByKey := agentsByKey(left.Agents)
	rightByKey := agentsByKey(right.Agents)
	keys := make([]string, 0, len(leftByKey)+len(rightByKey))
	seen := map[string]bool{}
	for key := range leftByKey {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range rightByKey {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		leftAgent, onLeft := leftByKey[key]
		rightAgent, onRight := rightByKey[key]
		diff := AIAgentDiff{Key: key, Changes: []Change{}}
		switch {
		case !onRight:
			diff.Name, diff.Product, diff.Capabilities, diff.Status = leftAgent.Name, leftAgent.Product, leftAgent.Capabilities, "left_only"
			diff.Left = aiAgentPointer(leftAgent)
			comparison.Summary.LeftOnly++
		case !onLeft:
			diff.Name, diff.Product, diff.Capabilities, diff.Status = rightAgent.Name, rightAgent.Product, rightAgent.Capabilities, "right_only"
			diff.Right = aiAgentPointer(rightAgent)
			comparison.Summary.RightOnly++
		default:
			diff.Name = firstNonEmpty(leftAgent.Name, rightAgent.Name, key)
			diff.Product = firstNonEmpty(leftAgent.Product, rightAgent.Product)
			diff.Capabilities = append([]string(nil), leftAgent.Capabilities...)
			if len(diff.Capabilities) == 0 {
				diff.Capabilities = append([]string(nil), rightAgent.Capabilities...)
			}
			diff.Left = aiAgentPointer(leftAgent)
			diff.Right = aiAgentPointer(rightAgent)
			diff.Changes = compareAIAgent(leftAgent, rightAgent)
			if len(diff.Changes) == 0 {
				diff.Status = "equal"
				comparison.Summary.Equal++
			} else {
				diff.Status = "changed"
				comparison.Summary.Changed++
			}
		}
		comparison.Agents = append(comparison.Agents, diff)
	}
	return comparison, nil
}

func compareAIAgent(left, right AIAgent) []Change {
	leftValues := flattenAIAgent(left)
	rightValues := flattenAIAgent(right)
	paths := make([]string, 0, len(leftValues)+len(rightValues))
	seen := map[string]bool{}
	for path := range leftValues {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range rightValues {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	changes := make([]Change, 0)
	for _, path := range paths {
		before, beforeOK := leftValues[path]
		after, afterOK := rightValues[path]
		if beforeOK == afterOK && reflect.DeepEqual(before, after) {
			continue
		}
		changes = append(changes, Change{Path: path, Before: before, After: after})
	}
	return changes
}

func flattenAIAgent(agent AIAgent) map[string]any {
	values := map[string]any{
		"name":               agent.Name,
		"product":            agent.Product,
		"capabilities":       agent.Capabilities,
		"accessState":        agent.AccessState,
		"allowedEntityTypes": agent.AllowedEntityTypes,
	}
	flattenAgentValue("configuration", agent.Configuration, values)
	return values
}

func flattenAgentValue(path string, value any, values map[string]any) {
	switch current := value.(type) {
	case map[string]any:
		if len(current) == 0 {
			values[path] = current
			return
		}
		for key, child := range current {
			flattenAgentValue(path+"."+key, child, values)
		}
	default:
		values[path] = current
	}
}

func agentsByKey(agents []AIAgent) map[string]AIAgent {
	result := make(map[string]AIAgent, len(agents))
	for _, agent := range agents {
		if agent.Key != "" {
			result[agent.Key] = agent
		}
	}
	return result
}

func aiAgentPointer(agent AIAgent) *AIAgent { return &agent }
