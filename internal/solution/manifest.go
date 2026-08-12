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

	"github.com/unofficialbox/box-dispatch/internal/bcl"
)

//go:embed manifests/*.bcl
var bundled embed.FS

type Manifest struct {
	SchemaVersion    string             `json:"schema_version"`
	TemplateID       string             `json:"template_id"`
	DeploymentConfig string             `json:"deployment_config"`
	Box              BoxManifest        `json:"box"`
	Salesforce       SalesforceManifest `json:"salesforce,omitempty"`
}

const (
	ManifestFile = "dispatch.bcl"

	defaultDeploymentConfig = ".dispatch/deployment.bcl"
	manifestProvider        = "box-dispatch"
	manifestContext         = "solution-manifest"
	manifestMetadataKey     = "manifest"
	deploymentContext       = "deployment-settings"
	deploymentMetadataKey   = "settings"
)

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

// SalesforceManifest declares prerequisites that must exist before packaged
// Salesforce metadata can compile. Keeping these in the solution contract lets
// validation show the install as deployment work instead of discovering the
// missing namespace after a doomed metadata request.
type SalesforceManifest struct {
	RequiredPackages       []SalesforcePackageRequirement       `json:"required_packages,omitempty"`
	RequiredPermissionSets []SalesforcePermissionSetRequirement `json:"required_permission_sets,omitempty"`
}

type SalesforcePackageRequirement struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace,omitempty"`
	PackageID     string `json:"package_id,omitempty"`
	VersionID     string `json:"version_id"`
	VersionName   string `json:"version_name,omitempty"`
	VersionNumber string `json:"version_number,omitempty"`
	SecurityType  string `json:"security_type,omitempty"`
}

// SalesforcePermissionSetRequirement declares a permission set that Dispatch
// assigns to the authenticated Salesforce deployment user after metadata is
// available. Name is the API name accepted by `sf org assign permset`; Label is
// the customer-facing checklist text.
type SalesforcePermissionSetRequirement struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
}

func clmSalesforcePermissionSets() []SalesforcePermissionSetRequirement {
	return []SalesforcePermissionSetRequirement{
		{Name: "box__Box_Admin_All_Licenses", Label: "Box Admin (All Licenses)"},
		{Name: "box__Docgen_Template_Manager", Label: "Box Doc Gen Template Manager"},
		{Name: "box__Box_Sign_Admin", Label: "Box Sign"},
		{Name: "CLM_Box_Automate_Integration", Label: "CLM Box Automate Integration"},
		{Name: "CLM_Demo_Operator", Label: "CLM Demo Operator"},
	}
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
	bclPath := filepath.Join(root, ManifestFile)
	if _, err := os.Stat(bclPath); err == nil {
		manifest, loadErr := loadBCLManifestFile(bclPath)
		if loadErr != nil {
			return Manifest{}, loadErr
		}
		return preparePackageManifest(root, manifest)
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}

	templateID, err := packageTemplateID(root)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := LoadBundled(templateID)
	if err != nil {
		return Manifest{}, fmt.Errorf("solution package requires %s: %w", ManifestFile, err)
	}
	return preparePackageManifest(root, manifest)
}

func preparePackageManifest(root string, manifest Manifest) (Manifest, error) {
	if manifest.DeploymentConfig == "" {
		manifest.DeploymentConfig = defaultDeploymentConfig
	}
	if err := ensureDeploymentConfig(root, manifest.DeploymentConfig, manifest.DefaultDeploymentSettings()); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ManifestExists(root string) (bool, error) {
	if _, err := os.Stat(filepath.Join(root, ManifestFile)); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

func EnsureDeploymentConfig(root, reference string) error {
	return ensureDeploymentConfig(root, reference, DefaultDeploymentSettings())
}

func ensureDeploymentConfig(root, reference string, settings DeploymentSettings) error {
	path, err := packageRelativePath(root, reference)
	if err != nil {
		return err
	}
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
			GlobalStrategy:      StrategyReuse,
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
	doc, err := bcl.LoadBCL(path)
	if err != nil {
		return DeploymentSettings{}, err
	}
	return deploymentSettingsFromDocument(doc, path)
}

func WriteDeploymentSettings(root, reference string, settings DeploymentSettings) error {
	path, err := packageRelativePath(root, reference)
	if err != nil {
		return err
	}
	doc, err := deploymentSettingsDocument(settings)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	defer os.Remove(temporary)
	if err := bcl.WriteBCL(temporary, doc); err != nil {
		return fmt.Errorf("create deployment configuration: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("create deployment configuration: %w", err)
	}
	return nil
}

func packageRelativePath(root, reference string) (string, error) {
	reference = filepath.Clean(filepath.FromSlash(reference))
	if filepath.IsAbs(reference) || reference == ".." || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("deployment_config must be a package-relative path")
	}
	if !strings.EqualFold(filepath.Ext(reference), ".bcl") {
		return "", fmt.Errorf("deployment_config must reference a .bcl file")
	}
	return filepath.Join(root, reference), nil
}

func WriteBundled(root, templateID string) error {
	manifest, err := LoadBundled(templateID)
	if err != nil {
		return err
	}
	if err := WriteManifest(root, manifest); err != nil {
		return err
	}
	_, err = preparePackageManifest(root, manifest)
	return err
}

// WriteManifest persists the normalized package contract. Package builders use
// this after loading a cloned template so capabilities without public APIs
// cannot re-enter the product through stale upstream configuration.
func WriteManifest(root string, manifest Manifest) error {
	manifest = normalizeCapabilityCatalog(manifest)
	if manifest.DeploymentConfig == "" {
		manifest.DeploymentConfig = defaultDeploymentConfig
	}
	if _, err := packageRelativePath(root, manifest.DeploymentConfig); err != nil {
		return err
	}
	doc, err := manifestDocument(manifest)
	if err != nil {
		return err
	}
	path := filepath.Join(root, ManifestFile)
	temporary := path + ".tmp"
	defer os.Remove(temporary)
	if err := bcl.WriteBCL(temporary, doc); err != nil {
		return fmt.Errorf("write solution manifest: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("write solution manifest: %w", err)
	}
	for _, obsolete := range []string{"dispatch.json", ".dispatch/deployment.json"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(obsolete))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove obsolete JSON configuration %s: %w", obsolete, err)
		}
	}
	return nil
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
	if !configured || !capability.CanDeploy() {
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
	source := "bundled " + templateID + " manifest"
	data, err := bundled.ReadFile("manifests/" + templateID + ".bcl")
	if err != nil {
		return Manifest{}, err
	}
	return parseBCLManifest(data, source)
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

func decodeManifestPayload(data []byte, source string) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", source, err)
	}
	if manifest.SchemaVersion == "" || manifest.TemplateID == "" {
		return Manifest{}, fmt.Errorf("%s is missing schema_version or template_id", source)
	}
	return normalizeCapabilityCatalog(manifest), nil
}

func loadBCLManifestFile(path string) (Manifest, error) {
	doc, err := bcl.LoadBCL(path)
	if err != nil {
		return Manifest{}, err
	}
	return manifestFromDocument(doc, path)
}

func parseBCLManifest(data []byte, source string) (Manifest, error) {
	doc, err := bcl.ParseBCL(data, source)
	if err != nil {
		return Manifest{}, err
	}
	return manifestFromDocument(doc, source)
}

func manifestFromDocument(doc bcl.BCLDocument, source string) (Manifest, error) {
	if doc.Provider != manifestProvider || doc.Context != manifestContext {
		return Manifest{}, fmt.Errorf("parse %s: expected provider %q and context %q", source, manifestProvider, manifestContext)
	}
	payload, ok := doc.Metadata[manifestMetadataKey]
	if !ok {
		return Manifest{}, fmt.Errorf("parse %s: missing metadata.%s", source, manifestMetadataKey)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", source, err)
	}
	manifest, err := decodeManifestPayload(data, source)
	if err != nil {
		return Manifest{}, err
	}
	if doc.Scenario != "" && doc.Scenario != manifest.TemplateID {
		return Manifest{}, fmt.Errorf("parse %s: scenario %q does not match template_id %q", source, doc.Scenario, manifest.TemplateID)
	}
	return manifest, nil
}

func manifestDocument(manifest Manifest) (bcl.BCLDocument, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return bcl.BCLDocument{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return bcl.BCLDocument{}, err
	}
	return bcl.NewDocument(manifest.TemplateID, manifestProvider, manifestContext, "", map[string]any{
		manifestMetadataKey: payload,
	}), nil
}

func deploymentSettingsFromDocument(doc bcl.BCLDocument, source string) (DeploymentSettings, error) {
	if doc.Provider != manifestProvider || doc.Context != deploymentContext {
		return DeploymentSettings{}, fmt.Errorf("parse %s: expected provider %q and context %q", source, manifestProvider, deploymentContext)
	}
	payload, ok := doc.Metadata[deploymentMetadataKey]
	if !ok {
		return DeploymentSettings{}, fmt.Errorf("parse %s: missing metadata.%s", source, deploymentMetadataKey)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return DeploymentSettings{}, fmt.Errorf("parse %s: %w", source, err)
	}
	settings := DefaultDeploymentSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return DeploymentSettings{}, fmt.Errorf("parse %s: %w", source, err)
	}
	if settings.Box.Components.Mode == "" {
		settings.Box.Components.Mode = "defaults"
	}
	if settings.Box.Components.Selections == nil {
		settings.Box.Components.Selections = map[string]bool{}
	}
	return settings, nil
}

func deploymentSettingsDocument(settings DeploymentSettings) (bcl.BCLDocument, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return bcl.BCLDocument{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return bcl.BCLDocument{}, err
	}
	return bcl.NewDocument("", manifestProvider, deploymentContext, "", map[string]any{
		deploymentMetadataKey: payload,
	}), nil
}

// normalizeCapabilityCatalog preserves unsupported entries as documentation
// metadata while removing them from deployment defaults. UI visibility is a
// separate BCL-backed Dispatch preference and never grants deploy support.
func normalizeCapabilityCatalog(manifest Manifest) Manifest {
	// Older CLM manifests predate explicit Salesforce prerequisites. Migrate
	// them in memory so existing zero-user packages gain the same contract as
	// newly generated packages and persist it on their next package build.
	if manifest.TemplateID == "clm" && len(manifest.Salesforce.RequiredPackages) == 0 {
		manifest.Salesforce.RequiredPackages = []SalesforcePackageRequirement{{
			Name:          "Box for Salesforce",
			Namespace:     "box",
			PackageID:     "033700000004yvWAAQ",
			VersionID:     "04tKi000000gPNZIA2",
			VersionName:   "5.43",
			VersionNumber: "5.43.0.1",
			SecurityType:  "AdminsOnly",
		}}
	}
	if manifest.TemplateID == "clm" && len(manifest.Salesforce.RequiredPermissionSets) == 0 {
		manifest.Salesforce.RequiredPermissionSets = clmSalesforcePermissionSets()
	}
	capabilityIDs := map[string]bool{}
	for _, capability := range manifest.Box.Capabilities {
		if !capability.CanDeploy() {
			continue
		}
		capabilityIDs[manifest.CapabilityID(capability)] = true
	}

	for id := range manifest.Box.DeploymentDefaults.ComponentStrategies {
		if !capabilityIDs[id] {
			delete(manifest.Box.DeploymentDefaults.ComponentStrategies, id)
		}
	}
	for id := range manifest.Box.DeploymentDefaults.Components.Selections {
		if !capabilityIDs[id] {
			delete(manifest.Box.DeploymentDefaults.Components.Selections, id)
		}
	}
	return manifest
}

// CanDeploy reports whether selecting the capability can lead to a supported
// public deployment. Empty operations remain compatible with older core
// capabilities whose non-empty handler already represented deploy support.
func (capability Capability) CanDeploy() bool {
	if !strings.EqualFold(strings.TrimSpace(capability.API), "public") || strings.TrimSpace(capability.Handler) == "" {
		return false
	}
	if len(capability.Operations) == 0 {
		return true
	}
	for _, operation := range capability.Operations {
		if strings.EqualFold(strings.TrimSpace(operation), "deploy") {
			return true
		}
	}
	return false
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
