package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type DeploymentRecord struct {
	SchemaVersion       string                                `json:"schema_version"`
	DeploymentID        string                                `json:"deployment_id"`
	Name                string                                `json:"name,omitempty"`
	StartedAt           time.Time                             `json:"started_at"`
	CompletedAt         time.Time                             `json:"completed_at"`
	Duration            string                                `json:"duration"`
	PackageRoot         string                                `json:"package_root"`
	TemplateID          string                                `json:"template_id"`
	DeploymentConfig    string                                `json:"deployment_config"`
	Strategy            string                                `json:"strategy"`
	RunID               string                                `json:"run_id,omitempty"`
	Rollback            solution.RollbackSettings             `json:"rollback"`
	ComponentStrategies map[string]solution.ComponentStrategy `json:"component_strategies,omitempty"`
	Artifacts           map[string]string                     `json:"artifact_sha256"`
	Providers           []ProviderRecord                      `json:"providers"`
	SourcePath          string                                `json:"-"`
}

type ProviderRecord struct {
	Provider       string                        `json:"provider"`
	StatusBefore   lifecycle.Status              `json:"status_before"`
	StatusAfter    lifecycle.Status              `json:"status_after"`
	Detail         string                        `json:"detail"`
	Deployed       []string                      `json:"deployed,omitempty"`
	PresentBefore  []string                      `json:"present_before,omitempty"`
	PresentAfter   []string                      `json:"present_after,omitempty"`
	Remaining      []string                      `json:"remaining,omitempty"`
	AdapterPending []string                      `json:"adapter_pending,omitempty"`
	Experimental   []string                      `json:"experimental,omitempty"`
	Resources      []lifecycle.ResourceReference `json:"resources,omitempty"`
}

func ExportDeployment(root, name string, before, after []lifecycle.Item, startedAt, completedAt time.Time) (string, error) {
	if root == "" {
		return "", fmt.Errorf("package directory is required")
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	} else {
		completedAt = completedAt.UTC()
	}
	if startedAt.IsZero() {
		startedAt = completedAt
	} else {
		startedAt = startedAt.UTC()
	}
	manifest, err := solution.Load(root)
	if err != nil {
		return "", err
	}
	settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
	if err != nil {
		return "", err
	}
	id := completedAt.Format("20060102T150405Z")
	record := DeploymentRecord{
		SchemaVersion:       "1.0",
		DeploymentID:        id,
		Name:                name,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		Duration:            completedAt.Sub(startedAt).Round(time.Millisecond).String(),
		PackageRoot:         root,
		TemplateID:          manifest.TemplateID,
		DeploymentConfig:    manifest.DeploymentConfig,
		Strategy:            settings.Box.GlobalStrategy,
		RunID:               settings.Box.RunID,
		Rollback:            settings.Box.Rollback,
		ComponentStrategies: settings.Box.ComponentStrategies,
		Artifacts:           map[string]string{},
		Providers:           providerRecords(before, after),
	}
	for _, relative := range []string{solution.ManifestFile, manifest.DeploymentConfig, ".dispatch/package.json"} {
		if digest, digestErr := fileDigest(filepath.Join(root, filepath.FromSlash(relative))); digestErr == nil {
			record.Artifacts[relative] = digest
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	globalDirectory, err := globalAuditDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(globalDirectory, 0o700); err != nil {
		return "", err
	}
	rootDigest := sha256.Sum256([]byte(root))
	globalPath := filepath.Join(globalDirectory, "deployment-"+id+"-"+hex.EncodeToString(rootDigest[:4])+".json")
	globalTemporary := globalPath + ".tmp"
	if err := os.WriteFile(globalTemporary, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(globalTemporary, globalPath); err != nil {
		return "", err
	}
	return globalPath, nil
}

// FindDeployment returns a recorded deployment by its ID.
func FindDeployment(deploymentID string) (DeploymentRecord, error) {
	records, err := ListDeployments()
	if err != nil {
		return DeploymentRecord{}, err
	}
	for _, record := range records {
		if record.DeploymentID == deploymentID {
			return record, nil
		}
	}
	return DeploymentRecord{}, fmt.Errorf("no deployment recorded with id %s", deploymentID)
}

// LatestDeploymentFor returns the most recent deployment recorded for a package
// root, which is what a "reset this environment" action acts on.
func LatestDeploymentFor(root string) (DeploymentRecord, error) {
	records, err := ListDeployments()
	if err != nil {
		return DeploymentRecord{}, err
	}
	// ListDeployments is newest-first, so the first match is the latest.
	for _, record := range records {
		if record.PackageRoot == root {
			return record, nil
		}
	}
	return DeploymentRecord{}, fmt.Errorf("no deployment has been recorded for %s", root)
}

// DeployedResources flattens every resource a deployment recorded across all
// providers — the inventory a teardown deletes from.
func (r DeploymentRecord) DeployedResources() []lifecycle.ResourceReference {
	resources := []lifecycle.ResourceReference{}
	for _, provider := range r.Providers {
		resources = append(resources, provider.Resources...)
	}
	return resources
}

func ListDeployments() ([]DeploymentRecord, error) {
	directory, err := globalAuditDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []DeploymentRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]DeploymentRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var record DeploymentRecord
		if json.Unmarshal(data, &record) != nil {
			continue
		}
		record.SourcePath = path
		records = append(records, record)
	}
	slices.SortStableFunc(records, func(a, b DeploymentRecord) int {
		return b.CompletedAt.Compare(a.CompletedAt)
	})
	return records, nil
}

func globalAuditDirectory() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "dispatch", "audit"), nil
}

func providerRecords(before, after []lifecycle.Item) []ProviderRecord {
	records := make([]ProviderRecord, 0, len(after))
	for _, current := range after {
		previous := findProvider(before, current.Provider)
		records = append(records, ProviderRecord{
			Provider:       current.Provider,
			StatusBefore:   previous.Status,
			StatusAfter:    current.Status,
			Detail:         current.Detail,
			Deployed:       difference(current.Present, previous.Present),
			PresentBefore:  append([]string(nil), previous.Present...),
			PresentAfter:   append([]string(nil), current.Present...),
			Remaining:      append([]string(nil), current.Missing...),
			AdapterPending: append([]string(nil), current.AdapterPending...),
			Experimental:   append([]string(nil), current.Experimental...),
			Resources:      append([]lifecycle.ResourceReference(nil), current.Resources...),
		})
	}
	return records
}

func findProvider(items []lifecycle.Item, provider string) lifecycle.Item {
	for _, item := range items {
		if item.Provider == provider {
			return item
		}
	}
	return lifecycle.Item{Provider: provider}
}

func difference(values, baseline []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !slices.Contains(baseline, value) {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return result
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
