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
var profileNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

const defaultProfile = "default"

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

type DeployedArtifact struct {
	Provider         string `json:"provider"`
	ArtifactType     string `json:"artifactType"`
	ProviderObjectID string `json:"providerObjectId"`
	EnterpriseID     string `json:"enterpriseId,omitempty"`
	ArtifactName     string `json:"artifactName,omitempty"`
	Scenario         string `json:"scenario"`
	Source           string `json:"source"`
	CreatedAt        string `json:"createdAt"`
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
	return PathsForProfile(resolveProfile(""))
}

func PathsForProfile(profile string) RuntimePaths {
	root, _ := os.Getwd()
	profileName := resolveProfile(profile)
	configDir := filepath.Join(resolveConfigBaseDir(), "box-dispatch", profileName)
	return RuntimePaths{
		Root:              root,
		ExampleConfig:     filepath.Join(configDir, "environment.example.bcl"),
		RuntimeConfig:     filepath.Join(configDir, "environment.json"),
		BootstrapState:    filepath.Join(configDir, "bootstrap-state.json"),
		GeneratedDir:      filepath.Join(configDir, "generated"),
		ValidationReceipt: filepath.Join(configDir, "validation-receipts.json"),
		SourceState:       filepath.Join(configDir, "source-state.json"),
	}
}

func resolveProfile(profile string) string {
	name := strings.TrimSpace(profile)
	if name == "" {
		name = strings.TrimSpace(os.Getenv("BOX_DISPATCH_PROFILE"))
	}
	if name == "" {
		name = defaultProfile
	}
	name = strings.TrimSpace(profileNameRe.ReplaceAllString(name, "-"))
	if strings.Trim(name, "-") == "" {
		return defaultProfile
	}
	return strings.Trim(name, "-")
}

func resolveConfigBaseDir() string {
	base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if base != "" {
		return base
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config")
	}
	return filepath.Join(os.TempDir(), ".config")
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
	return LoadRuntimeConfigFromPaths(Paths())
}

func LoadRuntimeConfigFromPaths(paths RuntimePaths) (*RuntimeConfig, error) {
	cfg := RuntimeConfig{
		Scenarios: make(map[string]Scenario),
		Providers: make(map[string]ProviderConfig),
	}
	runtimeConfig := paths.RuntimeConfig
	if _, err := os.Stat(runtimeConfig); err != nil {
		return nil, fmt.Errorf("%s is missing. Run `box-dispatch setup` to create it from %s.", paths.RuntimeConfig, paths.ExampleConfig)
	}
	if err := ReadJSON(runtimeConfig, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadOrInitRuntimeConfig(force bool) (*RuntimeConfig, error) {
	return LoadOrInitRuntimeConfigFromPaths(Paths(), force)
}

func LoadOrInitRuntimeConfigFromPaths(paths RuntimePaths, force bool) (*RuntimeConfig, error) {
	exampleConfig := paths.ExampleConfig
	if _, err := os.Stat(paths.RuntimeConfig); err == nil && !force {
		return LoadRuntimeConfigFromPaths(paths)
	}
	if _, err := os.Stat(exampleConfig); err != nil {
		return nil, fmt.Errorf("missing %s", paths.ExampleConfig)
	}
	example := RuntimeConfig{}
	if err := ReadJSON(exampleConfig, &example); err != nil {
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
	return LoadBootstrapStateFromPaths(Paths())
}

func LoadBootstrapStateFromPaths(paths RuntimePaths) (*BootstrapState, error) {
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
	return PersistBootstrapStateFromPaths(Paths(), state)
}

func PersistBootstrapStateFromPaths(paths RuntimePaths, state *BootstrapState) error {
	state.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	return WriteJSON(paths.BootstrapState, state)
}

func LoadSourceState() (*SourceState, error) {
	return LoadSourceStateFromPaths(Paths())
}

func LoadSourceStateFromPaths(paths RuntimePaths) (*SourceState, error) {
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
	return PersistSourceStateFromPaths(Paths(), state)
}

func PersistSourceStateFromPaths(paths RuntimePaths, state *SourceState) error {
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
