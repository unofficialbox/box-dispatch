package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"github.com/unofficialbox/box-dispatch/internal/providers"
)

const defaultImportScenario = "clm"

type importReport struct {
	source    string
	format    string
	scenario  string
	providers []string
}

func (e *Engine) Import(source string, scenario string, format string, force bool) (*model.CommandReport, int) {
	report := model.NewReport("import")
	start := time.Now()
	cfg, payload, err := e.importConfigFromPath(source, scenario, format)
	if err != nil {
		report.AddPhase("import.parse", string(model.PhaseFailed), elapsedMs(start), err)
		return &report, 2
	}

	if !force {
		if _, err := os.Stat(e.Paths.RuntimeConfig); err == nil {
			report.AddPhase("import.runtime", string(model.PhaseFailed), elapsedMs(start), fmt.Errorf("runtime config already exists: %s (use --force)", e.Paths.RuntimeConfig))
			return &report, 2
		}
	}

	payloadSource := payload.source
	payloadFormat := payload.format
	if payload.scenario != "" {
		cfg.ActiveScenario = payload.scenario
	}
	if len(payload.providers) > 0 {
		report.Validation["providerOrder"] = payload.providers
	}
	report.Validation["importSource"] = payloadSource
	report.Validation["importFormat"] = payloadFormat
	report.Validation["runtimeConfig"] = e.Paths.RuntimeConfig

	if err := config.WriteJSON(e.Paths.RuntimeConfig, cfg); err != nil {
		report.AddPhase("import.write", string(model.PhaseFailed), elapsedMs(start), err)
		return &report, 2
	}

	report.AddPhase("import.write", string(model.PhasePassed), elapsedMs(start), nil)
	return &report, 0
}

func (e *Engine) importConfigFromPath(source string, scenario string, format string) (*config.RuntimeConfig, importReport, error) {
	stats, err := os.Stat(source)
	if err != nil {
		return nil, importReport{}, err
	}
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))

	switch normalizedFormat {
	case "", "bcl":
		if stats.IsDir() {
			return e.importFromBCLDirectory(source, scenario)
		}
		name := strings.ToLower(filepath.Base(source))
		if !strings.HasSuffix(name, ".bcl") {
			return nil, importReport{}, fmt.Errorf("bcl format requires a .bcl file path, got: %s", source)
		}
		return e.importFromBCLFile(source, scenario)
	default:
		return nil, importReport{}, fmt.Errorf("unsupported import format %q", format)
	}
}

func (e *Engine) importFromBCLFile(path string, scenario string) (*config.RuntimeConfig, importReport, error) {
	doc, err := bcl.LoadBCL(path)
	if err != nil {
		return nil, importReport{}, err
	}
	cfg, err := buildConfigFromArtifacts(
		bcl.ExtractArtifactsFromBCL(doc),
		scenario,
		doc.Scenario,
		filepath.Base(path),
	)
	if err != nil {
		return nil, importReport{}, err
	}
	cfg = normalizeConfigForCLI(cfg)
	return &cfg, importReport{
		source:    path,
		format:    "bcl",
		scenario:  cfg.ActiveScenario,
		providers: orderedMapKeys(cfg.Providers),
	}, nil
}

func (e *Engine) importFromBCLDirectory(path string, scenario string) (*config.RuntimeConfig, importReport, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, importReport{}, err
	}

	var allArtifacts []config.DeployedArtifact
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, "artifacts.bcl") {
			continue
		}
		payloadPath := filepath.Join(path, entry.Name())
		artifacts, err := artifactsFromBCLFile(payloadPath)
		if err != nil {
			return nil, importReport{}, err
		}
		allArtifacts = append(allArtifacts, artifacts...)
	}
	if len(allArtifacts) == 0 {
		return nil, importReport{}, fmt.Errorf("no importable artifacts found in %s", path)
	}

	cfg, err := buildConfigFromArtifacts(allArtifacts, scenario, "", path)
	if err != nil {
		return nil, importReport{}, err
	}
	return &cfg, importReport{
		source:    path,
		format:    "bcl",
		scenario:  cfg.ActiveScenario,
		providers: orderedMapKeys(cfg.Providers),
	}, nil
}

func artifactsFromBCLFile(path string) ([]config.DeployedArtifact, error) {
	doc, err := bcl.LoadBCL(path)
	if err != nil {
		return nil, err
	}
	return bcl.ExtractArtifactsFromBCL(doc), nil
}

func normalizeConfigForCLI(cfg config.RuntimeConfig) config.RuntimeConfig {
	if cfg.Scenarios == nil {
		cfg.Scenarios = map[string]config.Scenario{}
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.ProviderConfig{}
	}
	if cfg.ActiveScenario == "" {
		for key := range cfg.Scenarios {
			cfg.ActiveScenario = key
			break
		}
	}
	if cfg.ActiveScenario == "" {
		cfg.ActiveScenario = defaultImportScenario
	}
	for scenarioName, scenarioCfg := range cfg.Scenarios {
		providers := make([]string, 0, len(scenarioCfg.Providers))
		seen := map[string]struct{}{}
		for _, provider := range scenarioCfg.Providers {
			key := strings.TrimSpace(provider)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			if _, ok := cfg.Providers[key]; !ok {
				continue
			}
			seen[key] = struct{}{}
			providers = append(providers, key)
		}
		if len(providers) == 0 {
			delete(cfg.Scenarios, scenarioName)
			continue
		}
		scenarioCfg.Providers = providers
		cfg.Scenarios[scenarioName] = scenarioCfg
	}

	valid := map[string]config.ProviderConfig{}
	for key, providerCfg := range cfg.Providers {
		if key == "" {
			continue
		}
		providerCfg.Variables = sanitizeVariables(providerCfg.Variables)
		for token := range providerCfg.Env {
			providerCfg.Env[strings.TrimSpace(token)] = providerCfg.Env[token]
		}
		providerCfg.Env = dropEmptyTokenEntries(providerCfg.Env)
		providerCfg = ensureRequiredTokensForProvider(key, providerCfg)
		valid[key] = providerCfg
	}
	cfg.Providers = valid
	return cfg
}

func buildConfigFromArtifacts(artifacts []config.DeployedArtifact, explicitScenario, fallbackScenario, importSource string) (config.RuntimeConfig, error) {
	if len(artifacts) == 0 {
		return config.RuntimeConfig{}, fmt.Errorf("no artifacts found in %s", importSource)
	}

	grouped := map[string][]config.DeployedArtifact{}
	scenarios := map[string]struct{}{}
	for _, artifact := range artifacts {
		key := strings.TrimSpace(artifact.Provider)
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], artifact)
		if scenario := strings.TrimSpace(artifact.Scenario); scenario != "" {
			scenarios[scenario] = struct{}{}
		}
	}

	if len(grouped) == 0 {
		return config.RuntimeConfig{}, fmt.Errorf("no usable provider artifacts found in %s", importSource)
	}

	targetScenario := strings.TrimSpace(explicitScenario)
	if targetScenario == "" {
		targetScenario = strings.TrimSpace(fallbackScenario)
	}
	if targetScenario == "" {
		for name := range scenarios {
			targetScenario = name
			break
		}
	}
	if targetScenario == "" {
		targetScenario = defaultImportScenario
	}

	providerList := orderedProviderKeysFromArtifacts(grouped)

	providersCfg := map[string]config.ProviderConfig{}
	for _, providerKey := range providerList {
		providersCfg[providerKey] = buildProviderConfigFromArtifacts(providerKey, grouped[providerKey])
	}

	cfg := config.RuntimeConfig{
		ActiveScenario: targetScenario,
		Scenarios: map[string]config.Scenario{
			targetScenario: {
				DisplayName: strings.Title(targetScenario) + " deployment",
				Providers:   providerList,
				Notes: []string{
					"Imported from deployment artifacts: " + importSource,
				},
			},
		},
		Providers: providersCfg,
	}

	cfg = normalizeConfigForCLI(cfg)
	return cfg, nil
}

func buildProviderConfigFromArtifacts(provider string, artifacts []config.DeployedArtifact) config.ProviderConfig {
	spec := providers.Specs[provider]
	providerCfg := config.ProviderConfig{
		DisplayName: strings.TrimSpace(spec.DisplayName),
		Env:         map[string]string{},
		Variables:   map[string]interface{}{"source": "artifact-import", "importedAt": time.Now().UTC().Format(time.RFC3339)},
	}
	if providerCfg.DisplayName == "" {
		providerCfg.DisplayName = provider
	}

	for _, artifact := range artifacts {
		switch provider {
		case "box":
			if artifact.ArtifactType == "box.folder" && artifact.ProviderObjectID != "" {
				providerCfg.Env["BOX_FOLDER_ID"] = artifact.ProviderObjectID
			}
			if artifact.ArtifactType == "box.enterprise" && artifact.ProviderObjectID != "" {
				providerCfg.Env["BOX_ENTERPRISE_ID"] = artifact.ProviderObjectID
			}
		case "salesforce-agentforce":
			if artifact.ArtifactType == "salesforce.org" && artifact.ProviderObjectID != "" {
				providerCfg.Env["SF_ORG_ID"] = artifact.ProviderObjectID
			}
		}
	}
	providerCfg = ensureRequiredTokensForProvider(provider, providerCfg)
	return providerCfg
}

func ensureRequiredTokensForProvider(provider string, cfg config.ProviderConfig) config.ProviderConfig {
	spec := providers.Specs[provider]
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	for _, token := range spec.RequiredTokens {
		if _, ok := cfg.Env[token]; ok {
			continue
		}
		cfg.Env[token] = "${" + token + "}"
	}
	return cfg
}

func sanitizeVariables(vars map[string]interface{}) map[string]interface{} {
	if vars == nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	for key, value := range vars {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func dropEmptyTokenEntries(tokens map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range tokens {
		t := strings.TrimSpace(value)
		if key == "" || t == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func validateScenarioPresence(cfg config.RuntimeConfig, scenario string) error {
	if scenario == "" {
		return nil
	}
	if _, ok := cfg.Scenarios[scenario]; !ok {
		return fmt.Errorf("scenario %q not found in import payload", scenario)
	}
	return nil
}

func orderedProviderKeys(configured map[string]config.ProviderConfig, scenarios map[string]config.Scenario) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, scenarioCfg := range scenarios {
		for _, provider := range scenarioCfg.Providers {
			if provider == "" {
				continue
			}
			if _, ok := configured[provider]; !ok {
				continue
			}
			if _, existed := seen[provider]; existed {
				continue
			}
			seen[provider] = struct{}{}
			out = append(out, provider)
		}
	}
	sort.Strings(out)
	return out
}

func orderedProviderKeysFromArtifacts(grouped map[string][]config.DeployedArtifact) []string {
	out := make([]string, 0, len(grouped))
	for key := range grouped {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func orderedMapKeys(input map[string]config.ProviderConfig) []string {
	out := make([]string, 0, len(input))
	for key := range input {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
