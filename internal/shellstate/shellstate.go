// Package shellstate persists local web application state (connection settings
// and the saved solution plan) as BCL, the single interchange format for
// box-dispatch. It lives outside internal/config because the BCL writer
// imports internal/config, so config cannot depend on bcl in return.
package shellstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

const (
	stateDirName          = ".dispatch"
	connectionSettingsBCL = "connection-settings.bcl"
	solutionPlanBCL       = "solution-plan.bcl"
	// Legacy JSON files (under the old .windlass state dir) read once for
	// forward migration to BCL.
	legacyStateDirName     = ".windlass"
	connectionSettingsJSON = "connections.json"
	solutionPlanJSON       = "solution.json"

	stateProvider         = "box-dispatch"
	connectionSettingsCtx = "connection-settings"
	solutionPlanContext   = "solution-plan"
)

func stateDir() string { return filepath.Join(config.Paths().Root, stateDirName) }

func statePath(name string) string { return filepath.Join(stateDir(), name) }

func legacyStatePath(name string) string {
	return filepath.Join(config.Paths().Root, legacyStateDirName, name)
}

// LoadConnectionSettings reads the BCL-persisted connection settings, migrating
// from the legacy JSON file when the BCL file is not yet present.
func LoadConnectionSettings() (config.ConnectionSettings, error) {
	var settings config.ConnectionSettings
	if err := readState(statePath(connectionSettingsBCL), legacyStatePath(connectionSettingsJSON), &settings); err != nil {
		return settings, fmt.Errorf("read connection settings: %w", err)
	}
	return settings, nil
}

// SaveConnectionSettings writes the connection settings as BCL.
func SaveConnectionSettings(settings config.ConnectionSettings) error {
	if err := writeState(statePath(connectionSettingsBCL), connectionSettingsCtx, settings); err != nil {
		return fmt.Errorf("write connection settings: %w", err)
	}
	return nil
}

// LoadSolutionPlan reads the BCL-persisted solution plan, migrating from the
// legacy JSON file when the BCL file is not yet present.
func LoadSolutionPlan() (config.SolutionPlan, error) {
	var plan config.SolutionPlan
	if err := readState(statePath(solutionPlanBCL), legacyStatePath(solutionPlanJSON), &plan); err != nil {
		return plan, fmt.Errorf("read solution plan: %w", err)
	}
	return plan, nil
}

// SaveSolutionPlan writes the solution plan as BCL.
func SaveSolutionPlan(plan config.SolutionPlan) error {
	if err := writeState(statePath(solutionPlanBCL), solutionPlanContext, plan); err != nil {
		return fmt.Errorf("write solution plan: %w", err)
	}
	return nil
}

// readState decodes state from the BCL file, falling back to a legacy JSON file
// for one-time migration. A missing file leaves out at its zero value.
func readState(bclPath, legacyJSONPath string, out any) error {
	if _, err := os.Stat(bclPath); err == nil {
		doc, err := bcl.LoadBCL(bclPath)
		if err != nil {
			return err
		}
		return fromMetadata(doc.Metadata, out)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if legacyJSONPath == "" {
		return nil
	}

	data, err := os.ReadFile(legacyJSONPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// writeState persists state as a BCL document carrying the fields under Metadata.
// It writes the canonical JSON payload form (which bcl.LoadBCL reads via its JSON
// path) rather than bcl.WriteBCL's HCL-locals form, which the loader does not
// round-trip.
func writeState(bclPath, context string, value any) error {
	metadata, err := toMetadata(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(bclPath), 0o700); err != nil {
		return err
	}
	doc := bcl.NewDocument("", stateProvider, context, time.Now().UTC().Format(time.RFC3339), metadata)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(bclPath, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(bclPath, 0o600)
}

// toMetadata renders a struct into a generic map using its json tags.
func toMetadata(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

// fromMetadata decodes a generic map back into a struct using its json tags.
func fromMetadata(metadata map[string]any, out any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
