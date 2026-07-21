package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var tokenRe = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

type Scenario struct {
	DisplayName string   `json:"displayName"`
	Providers   []string `json:"providers"`
	Notes       []string `json:"notes"`
}

type ProviderConfig struct {
	DisplayName string                 `json:"displayName"`
	Env         map[string]string      `json:"env"`
	Variables   map[string]interface{} `json:"variables"`
}

type RuntimeConfig struct {
	ActiveScenario string                    `json:"activeScenario"`
	Scenarios      map[string]Scenario       `json:"scenarios"`
	Providers      map[string]ProviderConfig `json:"providers"`
	Source         *ProjectSource            `json:"source,omitempty"`
}

type ProjectSource struct {
	URL       string `json:"url"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	Commit    string `json:"commit,omitempty"`
	ScannedAt string `json:"scannedAt,omitempty"`
}

type SourceState struct {
	URL       string `json:"url"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	Commit    string `json:"commit"`
	ScannedAt string `json:"scannedAt"`
}

type ProviderState struct {
	Status     string                 `json:"status"`
	UpdatedAt  string                 `json:"updatedAt"`
	Artifacts  []string               `json:"artifacts"`
	Note       string                 `json:"note,omitempty"`
	Validation map[string]interface{} `json:"validation,omitempty"`
}

type BootstrapState struct {
	GeneratedAt string                    `json:"generatedAt"`
	Scenarios   map[string]map[string]any `json:"scenarios"`
	Extra       map[string]interface{}    `json:"extra,omitempty"`
}

type RuntimePaths struct {
	Root              string
	ExampleConfig     string
	RuntimeConfig     string
	BootstrapState    string
	GeneratedDir      string
	ValidationReceipt string
	SourceState       string
}

func Paths() RuntimePaths {
	root, _ := os.Getwd()
	return RuntimePaths{
		Root:              root,
		ExampleConfig:     filepath.Join(root, "config", "runtime", "demo-environment.example.json"),
		RuntimeConfig:     filepath.Join(root, "config", "runtime", "demo-environment.json"),
		BootstrapState:    filepath.Join(root, "config", "runtime", "bootstrap-state.json"),
		GeneratedDir:      filepath.Join(root, "config", "runtime", "generated"),
		ValidationReceipt: filepath.Join(root, "config", "runtime", "validation-receipts.json"),
		SourceState:       filepath.Join(root, ".windlass", "source-state.json"),
	}
}

func ReadJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadRuntimeConfig() (*RuntimeConfig, error) {
	paths := Paths()
	cfg := RuntimeConfig{
		Scenarios: make(map[string]Scenario),
		Providers: make(map[string]ProviderConfig),
	}
	if _, err := os.Stat(paths.RuntimeConfig); err != nil {
		return nil, fmt.Errorf("%s is missing. Run `windlass setup` to create it from %s.", paths.RuntimeConfig, paths.ExampleConfig)
	}
	if err := ReadJSON(paths.RuntimeConfig, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadOrInitRuntimeConfig(force bool) (*RuntimeConfig, error) {
	paths := Paths()
	if _, err := os.Stat(paths.RuntimeConfig); err == nil && !force {
		return LoadRuntimeConfig()
	}
	if _, err := os.Stat(paths.ExampleConfig); err != nil {
		return nil, fmt.Errorf("missing %s", paths.ExampleConfig)
	}
	example := RuntimeConfig{}
	if err := ReadJSON(paths.ExampleConfig, &example); err != nil {
		return nil, err
	}
	if err := WriteJSON(paths.RuntimeConfig, example); err != nil {
		return nil, err
	}
	state := &BootstrapState{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Scenarios:   map[string]map[string]any{},
		Extra:       map[string]interface{}{},
	}
	if err := WriteJSON(paths.BootstrapState, state); err != nil {
		return nil, err
	}
	return &example, nil
}

func LoadBootstrapState() (*BootstrapState, error) {
	paths := Paths()
	state := &BootstrapState{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Scenarios:   map[string]map[string]any{},
		Extra:       map[string]interface{}{},
	}
	if _, err := os.Stat(paths.BootstrapState); err != nil {
		return state, nil
	}
	if err := ReadJSON(paths.BootstrapState, state); err != nil {
		return nil, err
	}
	if state.Scenarios == nil {
		state.Scenarios = map[string]map[string]any{}
	}
	return state, nil
}

func PersistBootstrapState(state *BootstrapState) error {
	paths := Paths()
	state.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	return WriteJSON(paths.BootstrapState, state)
}

func LoadSourceState() (*SourceState, error) {
	paths := Paths()
	state := &SourceState{}
	if _, err := os.Stat(paths.SourceState); err != nil {
		return state, nil
	}
	if err := ReadJSON(paths.SourceState, state); err != nil {
		return nil, err
	}
	return state, nil
}

func PersistSourceState(state *SourceState) error {
	paths := Paths()
	state.ScannedAt = time.Now().UTC().Format(time.RFC3339)
	return WriteJSON(paths.SourceState, state)
}

func ResolveValue(value any, env map[string]string, context map[string]string) (any, []string) {
	unresolved := []string{}
	switch typed := value.(type) {
	case string:
		out := tokenRe.ReplaceAllStringFunc(typed, func(token string) string {
			m := tokenRe.FindStringSubmatch(token)
			if len(m) != 2 {
				return token
			}
			key := m[1]
			if v, ok := context[key]; ok {
				return v
			}
			if v, ok := env[key]; ok {
				return v
			}
			unresolved = append(unresolved, key)
			return token
		})
		return out, unresolved
	case map[string]any:
		out := map[string]any{}
		for k, v := range typed {
			rv, miss := ResolveValue(v, env, context)
			out[k] = rv
			unresolved = append(unresolved, miss...)
		}
		return out, unresolved
	case []any:
		out := make([]any, 0, len(typed))
		for _, v := range typed {
			rv, miss := ResolveValue(v, env, context)
			out = append(out, rv)
			unresolved = append(unresolved, miss...)
		}
		return out, unresolved
	case ProviderConfig:
		out := map[string]any{
			"displayName": typed.DisplayName,
			"env":         typed.Env,
			"variables":   typed.Variables,
		}
		return ResolveValue(out, env, context)
	default:
		return value, nil
	}
}

func ResolveProviderConfig(pc ProviderConfig, scenario string, env map[string]string) (map[string]any, []string) {
	context := map[string]string{
		"SCENARIO_NAME": scenario,
	}
	raw := map[string]any{
		"displayName": pc.DisplayName,
		"env":         pc.Env,
		"variables":   pc.Variables,
	}
	resolved, missing := ResolveValue(raw, env, context)
	out := map[string]any{}
	if m, ok := resolved.(map[string]any); ok {
		out = m
	}
	uniq := uniqueStrings(missing)
	return out, uniq
}

func RequiredEnvKeys(cfg *RuntimeConfig) []string {
	seen := map[string]struct{}{}
	for _, provider := range cfg.Providers {
		for _, tokenExpr := range provider.Env {
			for _, m := range tokenRe.FindAllStringSubmatch(tokenExpr, -1) {
				if len(m) == 2 {
					seen[m[1]] = struct{}{}
				}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	return keys
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		n := strings.TrimSpace(v)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
