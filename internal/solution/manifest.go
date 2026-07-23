package solution

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed manifests/*.json
var bundled embed.FS

type Manifest struct {
	SchemaVersion    string      `json:"schema_version"`
	TemplateID       string      `json:"template_id"`
	DeploymentConfig string      `json:"deployment_config"`
	Box              BoxManifest `json:"box"`
}

const defaultDeploymentConfig = ".dispatch/deployment.json"

type DeploymentSettings struct {
	SchemaVersion string                `json:"schema_version"`
	Box           BoxDeploymentSettings `json:"box"`
}

type BoxDeploymentSettings struct {
	GlobalStrategy      string                       `json:"global_strategy"`
	RunID               string                       `json:"run_id,omitempty"`
	Naming              DeploymentNaming             `json:"naming"`
	Rollback            RollbackSettings             `json:"rollback"`
	ComponentStrategies map[string]ComponentStrategy `json:"component_strategies"`
	Components          ComponentSelection           `json:"components"`
}

const (
	StrategyInherit   = "inherit"
	StrategyCreateNew = "create_new"
	StrategyReuse     = "reuse"
)

func NormalizeDeploymentStrategy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case StrategyInherit, StrategyCreateNew, StrategyReuse:
		return value, nil
	default:
		return "", fmt.Errorf("deployment strategy must be inherit, create_new, or reuse")
	}
}

func ResolveDeploymentName(baseName string, settings BoxDeploymentSettings) (string, error) {
	strategy, err := NormalizeDeploymentStrategy(settings.GlobalStrategy)
	if err != nil {
		return "", err
	}
	baseName = strings.TrimSpace(baseName)
	if strategy != StrategyCreateNew {
		return baseName, nil
	}
	runID := strings.TrimSpace(settings.RunID)
	if runID == "" {
		return "", fmt.Errorf("create_new requires a stable deployment run_id; reconfigure the package before validation")
	}
	suffix := strings.TrimSpace(strings.ReplaceAll(settings.Naming.Suffix, "{run_id}", runID))
	if suffix == "" {
		suffix = runID
	}
	pattern := strings.TrimSpace(settings.Naming.Pattern)
	if pattern == "" {
		pattern = "{name} - {suffix}"
	}
	resolved := strings.ReplaceAll(pattern, "{name}", baseName)
	resolved = strings.ReplaceAll(resolved, "{suffix}", suffix)
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return "", fmt.Errorf("deployment naming pattern produced an empty resource name")
	}
	return resolved, nil
}

type RollbackSettings struct {
	Enabled   bool   `json:"enabled"`
	Directory string `json:"directory"`
	Retain    int    `json:"retain"`
}

type ComponentSelection struct {
	Mode       string          `json:"mode"`
	Selections map[string]bool `json:"selections,omitempty"`
}

type DeploymentNaming struct {
	Suffix  string `json:"suffix"`
	Pattern string `json:"pattern"`
}

type ComponentStrategy struct {
	Strategy string `json:"strategy"`
	Name     string `json:"name,omitempty"`
}

type BoxManifest struct {
	DeploymentDefaults BoxDeploymentSettings `json:"deployment_defaults"`
	ComponentOrder     []string              `json:"component_order"`
	Workspace          Workspace             `json:"workspace"`
	SampleContent      []SampleFile          `json:"sample_content,omitempty"`
	Capabilities       []Capability          `json:"capabilities"`
}

type SampleFile struct {
	Source       string `json:"source"`
	TargetFolder string `json:"target_folder"`
}

type Workspace struct {
	ComponentType string   `json:"component_type"`
	DisplayName   string   `json:"display_name"`
	Name          string   `json:"name"`
	Children      []string `json:"children"`
}

type Capability struct {
	ID               string   `json:"id,omitempty"`
	ComponentType    string   `json:"component_type"`
	DisplayName      string   `json:"display_name,omitempty"`
	Source           string   `json:"source,omitempty"`
	EnabledByDefault *bool    `json:"enabled_by_default,omitempty"`
	API              string   `json:"api"`
	Handler          string   `json:"handler,omitempty"`
	Template         string   `json:"template,omitempty"`
	Operations       []string `json:"operations,omitempty"`
}

func Load(root string) (Manifest, error) {
	path := filepath.Join(root, "dispatch.json")
	if data, err := os.ReadFile(path); err == nil {
		return loadPackageManifest(root, data, path)
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	templateID, err := packageTemplateID(root)
	if err != nil {
		return Manifest{}, err
	}
	data, err := bundled.ReadFile("manifests/" + templateID + ".json")
	if err != nil {
		return Manifest{}, fmt.Errorf("solution package requires dispatch.json: %w", err)
	}
	return loadPackageManifest(root, data, "bundled "+templateID+" manifest")
}

func loadPackageManifest(root string, data []byte, source string) (Manifest, error) {
	manifest, err := parse(data, source)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.DeploymentConfig == "" {
		manifest.DeploymentConfig = defaultDeploymentConfig
	}
	if err := ensureDeploymentConfig(root, manifest.DeploymentConfig, manifest.DefaultDeploymentSettings()); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func EnsureDeploymentConfig(root, reference string) error {
	return ensureDeploymentConfig(root, reference, DefaultDeploymentSettings())
}

func ensureDeploymentConfig(root, reference string, settings DeploymentSettings) error {
	reference = filepath.Clean(filepath.FromSlash(reference))
	if filepath.IsAbs(reference) || reference == ".." || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return fmt.Errorf("deployment_config must be a package-relative path")
	}
	path := filepath.Join(root, reference)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return WriteDeploymentSettings(root, reference, settings)
}

func DefaultDeploymentSettings() DeploymentSettings {
	return DeploymentSettings{
		SchemaVersion: "1.0",
		Box: BoxDeploymentSettings{
			GlobalStrategy:      "inherit",
			Naming:              DeploymentNaming{Pattern: "{name} - {suffix}"},
			Rollback:            RollbackSettings{Directory: ".dispatch/deployments", Retain: 10},
			ComponentStrategies: map[string]ComponentStrategy{},
			Components:          ComponentSelection{Mode: "defaults", Selections: map[string]bool{}},
		},
	}
}

func (m Manifest) DefaultDeploymentSettings() DeploymentSettings {
	settings := DefaultDeploymentSettings()
	defaults := m.Box.DeploymentDefaults
	if defaults.GlobalStrategy != "" {
		settings.Box.GlobalStrategy = defaults.GlobalStrategy
	}
	if defaults.Naming.Pattern != "" {
		settings.Box.Naming.Pattern = defaults.Naming.Pattern
	}
	if defaults.Naming.Suffix != "" {
		settings.Box.Naming.Suffix = defaults.Naming.Suffix
	}
	if defaults.Rollback.Directory != "" || defaults.Rollback.Enabled || defaults.Rollback.Retain != 0 {
		settings.Box.Rollback = defaults.Rollback
	}
	if defaults.ComponentStrategies != nil {
		settings.Box.ComponentStrategies = make(map[string]ComponentStrategy, len(defaults.ComponentStrategies))
		for id, strategy := range defaults.ComponentStrategies {
			settings.Box.ComponentStrategies[id] = strategy
		}
	}
	if defaults.Components.Mode != "" {
		settings.Box.Components = defaults.Components
		if settings.Box.Components.Selections == nil {
			settings.Box.Components.Selections = map[string]bool{}
		}
	}
	return settings
}

func ReadDeploymentSettings(root, reference string) (DeploymentSettings, error) {
	if err := EnsureDeploymentConfig(root, reference); err != nil {
		return DeploymentSettings{}, err
	}
	path, err := packageRelativePath(root, reference)
	if err != nil {
		return DeploymentSettings{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DeploymentSettings{}, err
	}
	settings := DefaultDeploymentSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return DeploymentSettings{}, fmt.Errorf("parse deployment configuration: %w", err)
	}
	if settings.Box.Components.Mode == "" {
		settings.Box.Components.Mode = "defaults"
	}
	if settings.Box.Components.Selections == nil {
		settings.Box.Components.Selections = map[string]bool{}
	}
	return settings, nil
}

func WriteDeploymentSettings(root, reference string, settings DeploymentSettings) error {
	path, err := packageRelativePath(root, reference)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("create deployment configuration: %w", err)
	}
	return nil
}

func packageRelativePath(root, reference string) (string, error) {
	reference = filepath.Clean(filepath.FromSlash(reference))
	if filepath.IsAbs(reference) || reference == ".." || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("deployment_config must be a package-relative path")
	}
	return filepath.Join(root, reference), nil
}

func WriteBundled(root, templateID string) error {
	data, err := bundled.ReadFile("manifests/" + templateID + ".json")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "dispatch.json"), data, 0o644); err != nil {
		return err
	}
	_, err = loadPackageManifest(root, data, "bundled "+templateID+" manifest")
	return err
}

func IsUnavailable(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func (m Manifest) Capability(componentType string) (Capability, bool) {
	for _, capability := range m.Box.Capabilities {
		if capability.ComponentType == componentType {
			return capability, true
		}
	}
	return Capability{}, false
}

func (m Manifest) CapabilityID(capability Capability) string {
	if capability.ID != "" {
		return capability.ID
	}
	value := strings.ToLower(capability.ComponentType)
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, value)
}

func (m Manifest) CapabilityEnabled(componentType string, selection ComponentSelection) bool {
	capability, configured := m.Capability(componentType)
	if !configured {
		return false
	}
	defaultEnabled := capability.EnabledByDefault == nil || *capability.EnabledByDefault
	switch selection.Mode {
	case "all":
		return true
	case "none":
		return false
	case "custom":
		return selection.Selections[m.CapabilityID(capability)]
	default:
		return defaultEnabled
	}
}

func LoadBundled(templateID string) (Manifest, error) {
	data, err := bundled.ReadFile("manifests/" + templateID + ".json")
	if err != nil {
		return Manifest{}, err
	}
	return parse(data, "bundled "+templateID+" manifest")
}

func (m Manifest) Rank(component string) int {
	componentType := strings.SplitN(component, ":", 2)[0]
	for index, configured := range m.Box.ComponentOrder {
		if componentType == configured {
			return index
		}
	}
	return len(m.Box.ComponentOrder)
}

func parse(data []byte, source string) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", source, err)
	}
	if manifest.SchemaVersion == "" || manifest.TemplateID == "" {
		return Manifest{}, fmt.Errorf("%s is missing schema_version or template_id", source)
	}
	return manifest, nil
}

func packageTemplateID(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".dispatch", "package.json"))
	if err != nil {
		return "", err
	}
	var pkg struct {
		TemplateID string `json:"template_id"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	if pkg.TemplateID == "" {
		return "", fmt.Errorf("package manifest is missing template_id")
	}
	return pkg.TemplateID, nil
}
