package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/solution"
)

type PackageRequest struct {
	Repository    string
	Destination   string
	TemplateID    string
	Components    []string
	BoxComponents solution.ComponentSelection
	BoxStrategy   string
}

type PackageManifest struct {
	SchemaVersion string    `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Repository    string    `json:"repository"`
	TemplateID    string    `json:"template_id"`
	Components    []string  `json:"components"`
	Destination   string    `json:"destination"`
}

func Build(req PackageRequest) (PackageManifest, error) {
	if req.Repository == "" || req.Destination == "" {
		return PackageManifest{}, fmt.Errorf("repository and destination are required")
	}
	if !slices.Contains(req.Components, "box") {
		return PackageManifest{}, fmt.Errorf("Box must be included in every package")
	}
	if entries, err := os.ReadDir(req.Destination); err == nil && len(entries) > 0 {
		return PackageManifest{}, fmt.Errorf("destination is not empty: %s", req.Destination)
	} else if err != nil && !os.IsNotExist(err) {
		return PackageManifest{}, err
	}
	if err := os.MkdirAll(filepath.Dir(req.Destination), 0o755); err != nil {
		return PackageManifest{}, err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", req.Repository, req.Destination)
	// Run the clone non-interactively. Without this, a credential challenge for
	// github.com (private repo, proxy, rate-limit, or a machine-level credential
	// helper) makes git block on a "Username for 'https://github.com':" prompt.
	// Inside the full-screen shell that prompt is invisible and unanswerable, so
	// packaging appears to hang forever. Failing fast surfaces a real error.
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // git: never fall back to an interactive prompt
		"GCM_INTERACTIVE=Never", // Git Credential Manager: don't pop a dialog
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = "no output (git could not authenticate to " + req.Repository + " non-interactively)"
		}
		return PackageManifest{}, fmt.Errorf("clone template %s: %w: %s", req.Repository, err, trimmed)
	}
	if err := os.RemoveAll(filepath.Join(req.Destination, ".git")); err != nil {
		return PackageManifest{}, err
	}
	if err := pruneUnselected(req.Destination, req.Components); err != nil {
		return PackageManifest{}, err
	}
	if _, err := os.Stat(filepath.Join(req.Destination, "dispatch.json")); os.IsNotExist(err) {
		if writeErr := solution.WriteBundled(req.Destination, req.TemplateID); writeErr != nil && !solution.IsUnavailable(writeErr) {
			return PackageManifest{}, writeErr
		}
	}
	solutionManifest, loadErr := solution.Load(req.Destination)
	if loadErr != nil {
		return PackageManifest{}, loadErr
	}
	if writeErr := solution.WriteManifest(req.Destination, solutionManifest); writeErr != nil {
		return PackageManifest{}, writeErr
	}
	if pruneErr := pruneUnsupportedBoxAssets(req.Destination); pruneErr != nil {
		return PackageManifest{}, pruneErr
	}
	if req.BoxComponents.Mode != "" || req.BoxStrategy != "" {
		settings, loadErr := solution.ReadDeploymentSettings(req.Destination, solutionManifest.DeploymentConfig)
		if loadErr != nil {
			return PackageManifest{}, loadErr
		}
		if req.BoxComponents.Mode != "" {
			settings.Box.Components = req.BoxComponents
		}
		if req.BoxStrategy != "" {
			strategy, strategyErr := solution.NormalizeDeploymentStrategy(req.BoxStrategy)
			if strategyErr != nil {
				return PackageManifest{}, strategyErr
			}
			settings.Box.GlobalStrategy = strategy
			if strategy == solution.StrategyCreateNew {
				settings.Box.RunID = time.Now().UTC().Format("20060102T150405Z")
				if settings.Box.Naming.Suffix == "" {
					settings.Box.Naming.Suffix = "{run_id}"
				}
			} else {
				settings.Box.RunID = ""
			}
		}
		if writeErr := solution.WriteDeploymentSettings(req.Destination, solutionManifest.DeploymentConfig, settings); writeErr != nil {
			return PackageManifest{}, writeErr
		}
	}
	manifest := PackageManifest{
		SchemaVersion: "1.0", CreatedAt: time.Now(), Repository: req.Repository,
		TemplateID: req.TemplateID, Components: req.Components, Destination: req.Destination,
	}
	stateDir := filepath.Join(req.Destination, ".dispatch")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return PackageManifest{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return PackageManifest{}, err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "package.json"), append(data, '\n'), 0o600); err != nil {
		return PackageManifest{}, err
	}
	return manifest, nil
}

func pruneUnsupportedBoxAssets(root string) error {
	for _, reference := range []string{
		"config/box/form-definition.bcl",
		"config/box/form-definition.json",
		"config/box/box-app-blueprint.md",
		"config/box/https-connectors.bcl",
		"config/box/https-connectors.json",
	} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(reference))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func pruneUnselected(root string, components []string) error {
	if !slices.Contains(components, "salesforce") {
		for _, path := range []string{"config/salesforce", "config/agentforce"} {
			if err := os.RemoveAll(filepath.Join(root, path)); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.Contains(strings.ToLower(entry.Name()), "salesforce") {
				if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	if !slices.Contains(components, "databricks") {
		for _, path := range []string{"config/databricks", "databricks", "infrastructure/databricks"} {
			if err := os.RemoveAll(filepath.Join(root, path)); err != nil {
				return err
			}
		}
	}
	if !slices.Contains(components, "aws") {
		for _, path := range []string{"config/agentcore", "infrastructure/agentcore", "infrastructure/aws"} {
			if err := os.RemoveAll(filepath.Join(root, path)); err != nil {
				return err
			}
		}
	}
	return nil
}
