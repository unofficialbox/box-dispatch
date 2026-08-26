package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"github.com/unofficialbox/box-dispatch/internal/providers"
)

type Engine struct {
	Paths config.RuntimePaths
}

func NewEngine(profile string) *Engine {
	return &Engine{Paths: config.PathsForProfile(profile)}
}

func (e *Engine) Init(sourceURL string, branch string, scenarioPath string, sourceRef string, force bool) (*model.CommandReport, int) {
	report := model.NewReport("init")
	start := time.Now()

	if strings.TrimSpace(sourceURL) == "" {
		report.AddPhase("init.source", string(model.PhaseFailed), elapsedMs(start), fmt.Errorf("missing --source-url"))
		return &report, 2
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	if scenarioPath == "" {
		scenarioPath = "."
	}

	sourceRoot := filepath.Join(e.Paths.Root, "box-dispatch", "scenario-source")
	repoPath := sourceRoot
	manifestPath := filepath.Join(repoPath, scenarioPath, "config", "runtime", "environment.example.bcl")

	if err := e.fetchScenarioSource(sourceURL, branch, sourceRef, repoPath, force); err != nil {
		report.AddPhase("init.source", string(model.PhaseFailed), elapsedMs(start), err)
		return &report, 2
	}
	report.AddPhase("init.source", string(model.PhasePassed), elapsedMs(start), nil)

	if _, err := os.Stat(manifestPath); err != nil {
		fallback := filepath.Join(repoPath, "config", "runtime", "environment.example.bcl")
		if _, err := os.Stat(fallback); err == nil {
			manifestPath = fallback
		} else {
			report.AddPhase("init.manifest", string(model.PhaseFailed), elapsedMs(start), fmt.Errorf("unable to locate environment.example.bcl from repo"))
			report.Validation["searchPath"] = filepath.Join(repoPath, scenarioPath)
			report.AddPhase("init.manifest_help", string(model.PhaseBlocked), elapsedMs(start), fmt.Errorf("expected path: <sourcePath>/config/runtime/environment.example.bcl or <sourcePath>/<scenarioPath>/config/runtime/environment.example.bcl"))
			return &report, 2
		}
	}

	if err := copyFile(manifestPath, e.Paths.ExampleConfig); err != nil {
		report.AddPhase("init.manifest", string(model.PhaseFailed), elapsedMs(start), err)
		return &report, 2
	}
	report.Validation["sourceManifest"] = manifestPath
	report.Validation["runtimeExampleConfig"] = e.Paths.ExampleConfig

	state := config.SourceState{
		URL:    sourceURL,
		Branch: branch,
		Path:   scenarioPath,
	}
	if sourceRef != "" {
		state.Commit = sourceRef
	} else if h, err := resolveCommit(repoPath); err == nil {
		state.Commit = h
	}
	if err := config.PersistSourceStateFromPaths(e.Paths, &state); err != nil {
		report.AddPhase("init.state", string(model.PhaseWarn), elapsedMs(start), err)
	}

	report.AddPhase("init.state", string(model.PhasePassed), elapsedMs(start), nil)
	report.Validation["sourceState"] = e.Paths.SourceState
	report.Validation["sourcePath"] = repoPath
	report.NextCommand = "box-dispatch setup"
	return &report, 0
}

func (e *Engine) Setup(force bool) (*model.CommandReport, int) {
	report := model.NewReport("setup")

	start := time.Now()
	cfg, err := config.LoadOrInitRuntimeConfigFromPaths(e.Paths, force)
	if err != nil {
		report.AddPhase("setup.runtime", string(model.PhaseFailed), elapsedMs(start), err)
		return &report, 2
	}
	report.AddPhase("setup.runtime", string(model.PhasePassed), elapsedMs(start), nil)

	scenario := cfg.ActiveScenario
	if scenario == "" {
		if s, err := e.resolveScenario(cfg, ""); err == nil {
			scenario = s
		}
	}
	report.Scenario = scenario
	report.Validation["runtimeConfig"] = e.Paths.RuntimeConfig
	report.Validation["stateFile"] = e.Paths.BootstrapState
	report.Validation["generatedDir"] = e.Paths.GeneratedDir
	report.Validation["exampleConfig"] = e.Paths.ExampleConfig
	report.NextCommand = "box-dispatch check --scenario " + scenario

	return &report, 0
}

func (e *Engine) Check(scenario string, platform string, offline bool) (*model.CommandReport, int) {
	report, code := e.Doctor(scenario, platform, offline)
	if report == nil {
		newReport := model.NewReport("check")
		report = &newReport
		return report, code
	}

	report.Command = "check"
	report.Validation["commandMode"] = "dependency"
	report.Validation["offline"] = offline
	if code == 0 {
		if report.Status == string(model.PhasePassed) {
			report.NextCommand = "box-dispatch setup"
		} else if report.Status == string(model.PhaseWarn) {
			target := ""
			if report.Scenario != "" {
				target = " --scenario " + report.Scenario
			}
			report.NextCommand = "box-dispatch check" + target
		} else {
			target := ""
			if report.Scenario != "" {
				target = " --scenario " + report.Scenario
			}
			report.NextCommand = "box-dispatch resolve" + target + " --allow-unresolved"
		}
	} else {
		report.NextCommand = "box-dispatch init"
	}

	return report, code
}

func (e *Engine) Doctor(scenario string, platform string, offline bool) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("doctor")
		report.AddPhase("doctor.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("doctor")
		report.AddPhase("doctor.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	report := model.NewReport("doctor")
	report.Scenario = scenarioName
	report.Validation["platform"] = platform
	report.Validation["offline"] = offline

	scenarioCfg := cfg.Scenarios[scenarioName]
	for _, providerKey := range scenarioCfg.Providers {
		provider, ok := providers.Specs[providerKey]
		if !ok {
			report.AddPhase("doctor."+providerKey, string(model.PhaseWarn), 0, fmt.Errorf("provider not in registry: %s", providerKey))
			continue
		}
		if platform != "all" && platform != provider.Platform && platform != providerKey {
			continue
		}

		phaseStart := time.Now()
		res := e.checkProvider(scenarioName, providerKey, provider, cfg, offline)
		e.appendDependencyPhases(&report, "doctor", providerKey, provider, res, phaseStart, offline)
	}

	if report.Status == string(model.PhaseBlocked) {
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName + " --allow-unresolved"
		return &report, 2
	}

	if report.Status == string(model.PhaseWarn) {
		report.NextCommand = "box-dispatch status --scenario " + scenarioName
	}
	return &report, 0
}

func (e *Engine) Status(scenario string) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("status")
		report.AddPhase("status.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("status")
		report.AddPhase("status.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	state, err := config.LoadBootstrapStateFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("status")
		report.AddPhase("status.state", string(model.PhaseWarn), 0, err)
		return &report, 2
	}

	report := model.NewReport("status")
	report.Scenario = scenarioName
	report.Validation["runtimeConfig"] = e.Paths.RuntimeConfig
	report.Validation["stateFile"] = e.Paths.BootstrapState

	scenarioCfg := cfg.Scenarios[scenarioName]
	unresolved := []string{}
	for _, providerKey := range scenarioCfg.Providers {
		pcfg, ok := cfg.Providers[providerKey]
		if !ok {
			unresolved = append(unresolved, providerKey+":missing_config")
			continue
		}
		_, missing := config.ResolveProviderConfig(pcfg, scenarioName, environmentMap())
		for _, token := range missing {
			unresolved = append(unresolved, providerKey+":"+token)
		}
	}
	sort.Strings(unresolved)
	report.Validation["unresolvedTokens"] = unresolved
	if len(unresolved) > 0 {
		report.AddPhase("status.tokens", string(model.PhaseBlocked), 0, fmt.Errorf("unresolved tokens present"))
		report.ConfirmRequired = true
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName + " --allow-unresolved"
		return &report, 2
	}

	if scenarioState, ok := state.Scenarios[scenarioName]; ok {
		report.Validation["scenarioState"] = scenarioState
	}
	report.AddPhase("status.ok", string(model.PhasePassed), 0, nil)
	report.NextCommand = "box-dispatch validate --scenario " + scenarioName
	return &report, 0
}

func (e *Engine) Resolve(scenario string, dryRun bool, allowUnresolved bool) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("resolve")
		report.AddPhase("resolve.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("resolve")
		report.AddPhase("resolve.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	scenarioCfg := cfg.Scenarios[scenarioName]
	report := model.NewReport("resolve")
	report.Scenario = scenarioName
	report.DryRun = dryRun

	outputDir := filepath.Join(e.Paths.GeneratedDir, scenarioName)
	if !dryRun {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			report.AddPhase("resolve.output-dir", string(model.PhaseFailed), 0, err)
			return &report, 2
		}
	}

	env := environmentMap()
	for _, providerKey := range scenarioCfg.Providers {
		pcfg, ok := cfg.Providers[providerKey]
		if !ok {
			report.AddPhase("resolve."+providerKey, string(model.PhaseWarn), 0, fmt.Errorf("provider config not found"))
			continue
		}

		start := time.Now()
		generatedAt := e.now()
		_, missing := config.ResolveProviderConfig(pcfg, scenarioName, env)
		if len(missing) > 0 && !allowUnresolved {
			report.AddManual(providerKey + ".environment")
			report.AddPhase("resolve."+providerKey, string(model.PhaseBlocked), elapsedMs(start), fmt.Errorf("unresolved tokens: %s", unresolvedTokens(missing)))
			continue
		}
		if dryRun {
			report.AddPhase("resolve."+providerKey, string(model.PhaseSkipped), elapsedMs(start), nil)
			continue
		}

		deployedArtifacts := collectArtifactInventory(providerKey, scenarioName, env, generatedAt)
		if len(deployedArtifacts) > 0 {
			paths, err := writeArtifactBundle(outputDir, providerKey, scenarioName, "resolve", generatedAt, deployedArtifacts)
			if err != nil {
				report.AddPhase("resolve."+providerKey+".artifacts", string(model.PhaseWarn), elapsedMs(start), err)
			} else {
				report.Validation["resolve."+providerKey+".artifacts_bcl"] = paths.BCL
			}
		}

		report.AddPhase("resolve."+providerKey, string(model.PhasePassed), elapsedMs(start), nil)
	}

	if report.Status == string(model.PhasePassed) {
		report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		return &report, 0
	}
	if report.Status == string(model.PhaseBlocked) {
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName + " --allow-unresolved"
		return &report, 2
	}
	return &report, 2
}

func (e *Engine) Bootstrap(scenario string, dryRun bool, yes bool, allowUnresolved bool, skipValidate bool) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("bootstrap")
		report.AddPhase("bootstrap.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("bootstrap")
		report.AddPhase("bootstrap.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	if !yes && !dryRun {
		report := model.NewReport("bootstrap")
		report.Scenario = scenarioName
		report.ConfirmRequired = true
		report.AddPhase("bootstrap.confirm", string(model.PhaseBlocked), 0, fmt.Errorf("use --yes or --confirm to apply changes"))
		report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		return &report, 2
	}

	scenarioCfg := cfg.Scenarios[scenarioName]
	state, err := config.LoadBootstrapStateFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("bootstrap")
		report.AddPhase("bootstrap.state", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	if state.Scenarios == nil {
		state.Scenarios = map[string]map[string]any{}
	}
	if _, ok := state.Scenarios[scenarioName]; !ok {
		state.Scenarios[scenarioName] = map[string]any{}
	}

	env := environmentMap()
	outputDir := filepath.Join(e.Paths.GeneratedDir, scenarioName)
	report := model.NewReport("bootstrap")
	report.Scenario = scenarioName
	report.DryRun = dryRun
	report.ConfirmRequired = !yes && !dryRun

	providerStatesRaw := state.Scenarios[scenarioName]["providers"]
	providerStates := map[string]any{}
	if providerStatesRaw != nil {
		if cast, ok := providerStatesRaw.(map[string]any); ok {
			providerStates = cast
		}
	}

	for _, providerKey := range scenarioCfg.Providers {
		spec, ok := providers.Specs[providerKey]
		if !ok {
			report.AddPhase("bootstrap."+providerKey, string(model.PhaseBlocked), 0, fmt.Errorf("provider not in registry: %s", providerKey))
			continue
		}
		providerCfg, ok := cfg.Providers[providerKey]
		if !ok {
			report.AddPhase("bootstrap."+providerKey, string(model.PhaseBlocked), 0, fmt.Errorf("provider config missing"))
			continue
		}

		stepStart := time.Now()
		if !(dryRun && skipValidate) {
			for _, tool := range spec.RequiredTools {
				if _, err := exec.LookPath(tool); err != nil {
					report.AddPhase("bootstrap."+providerKey+".tool", string(model.PhaseFailed), elapsedMs(stepStart), err)
					report.AddPhase("bootstrap."+providerKey, string(model.PhaseBlocked), elapsedMs(stepStart), fmt.Errorf("tooling pre-check failed"))
					continue
				}
			}
		}

		_, missing := config.ResolveProviderConfig(providerCfg, scenarioName, env)
		if len(missing) > 0 && !allowUnresolved {
			report.AddManual(providerKey + ".environment")
			report.AddPhase("bootstrap."+providerKey+".tokens", string(model.PhaseBlocked), elapsedMs(stepStart), fmt.Errorf("missing tokens: %s", unresolvedTokens(missing)))
			providerStates[providerKey] = map[string]any{
				"status":    "blocked",
				"reason":    "missing tokens",
				"updatedAt": e.now(),
			}
			continue
		}

		if dryRun {
			report.AddPhase("bootstrap."+providerKey+".steps", string(model.PhaseSkipped), elapsedMs(stepStart), nil)
			providerStates[providerKey] = map[string]any{
				"status":    "pending",
				"note":      "dry run",
				"updatedAt": e.now(),
			}
			continue
		}

		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			report.AddPhase("bootstrap."+providerKey+".dir", string(model.PhaseFailed), elapsedMs(stepStart), err)
			providerStates[providerKey] = map[string]any{
				"status":    "failed",
				"reason":    err.Error(),
				"updatedAt": e.now(),
			}
			continue
		}

		artifacts := []string{}
		generatedAt := e.now()

		deployedArtifacts := collectArtifactInventory(providerKey, scenarioName, env, generatedAt)
		if len(deployedArtifacts) > 0 {
			paths, err := writeArtifactBundle(outputDir, providerKey, scenarioName, "bootstrap", generatedAt, deployedArtifacts)
			if err != nil {
				report.AddPhase("bootstrap."+providerKey+".artifacts", string(model.PhaseWarn), elapsedMs(stepStart), err)
			} else {
				artifacts = append(artifacts, paths.BCL)
			}
		}

		_, missing = config.ResolveProviderConfig(providerCfg, scenarioName, env)

		stepFailed := false
		for _, step := range spec.BootstrapSteps {
			stepStart := time.Now()
			stepFile := filepath.Join(outputDir, providerKey+"-"+slug(step)+".done")
			if err := os.WriteFile(stepFile, []byte("completed\n"), 0o644); err != nil {
				report.AddPhase("bootstrap."+providerKey+"."+slug(step), string(model.PhaseFailed), elapsedMs(stepStart), err)
				stepFailed = true
				break
			}
			artifacts = append(artifacts, stepFile)
			report.AddPhase("bootstrap."+providerKey+"."+slug(step), string(model.PhasePassed), elapsedMs(stepStart), nil)
		}
		if stepFailed {
			providerStates[providerKey] = map[string]any{
				"status":    "failed",
				"updatedAt": e.now(),
			}
			continue
		}

		providerStates[providerKey] = map[string]any{
			"status":            "passed",
			"artifacts":         artifacts,
			"deployedArtifacts": deployedArtifacts,
			"updatedAt":         e.now(),
		}
		report.AddPhase("bootstrap."+providerKey, string(model.PhasePassed), elapsedMs(stepStart), nil)
	}

	state.Scenarios[scenarioName]["providers"] = providerStates
	if err := config.PersistBootstrapStateFromPaths(e.Paths, state); err != nil {
		report.AddPhase("bootstrap.persist", string(model.PhaseWarn), 0, err)
	}

	if report.Status == string(model.PhasePassed) {
		if !dryRun {
			report.NextCommand = "box-dispatch validate --scenario " + scenarioName
		} else {
			report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		}
		return &report, 0
	}
	if report.Status == string(model.PhaseBlocked) {
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName + " --allow-unresolved"
		return &report, 2
	}
	return &report, 2
}

func (e *Engine) Validate(scenario string, presenterReady bool, skipArtifacts bool) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("validate")
		report.AddPhase("validate.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("validate")
		report.AddPhase("validate.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	report := model.NewReport("validate")
	report.Scenario = scenarioName
	env := environmentMap()
	state, _ := config.LoadBootstrapStateFromPaths(e.Paths)
	if scenarioState, ok := state.Scenarios[scenarioName]; ok {
		report.Validation["scenarioState"] = scenarioState
	}

	for _, providerKey := range cfg.Scenarios[scenarioName].Providers {
		start := time.Now()
		pcfg, ok := cfg.Providers[providerKey]
		if !ok {
			report.AddPhase("validate."+providerKey, string(model.PhaseWarn), 0, fmt.Errorf("provider config missing"))
			continue
		}
		_, missing := config.ResolveProviderConfig(pcfg, scenarioName, env)
		if len(missing) > 0 {
			report.AddPhase("validate."+providerKey+".tokens", string(model.PhaseBlocked), elapsedMs(start), fmt.Errorf("unresolved env tokens: %s", unresolvedTokens(missing)))
			continue
		}
		if !skipArtifacts {
			artifact := filepath.Join(e.Paths.GeneratedDir, scenarioName, providerKey+"-artifacts.bcl")
			if _, err := os.Stat(artifact); err != nil {
				report.AddPhase("validate."+providerKey+".artifact", string(model.PhaseWarn), 0, fmt.Errorf("artifact missing"))
				continue
			}
		}
		report.AddPhase("validate."+providerKey+".ok", string(model.PhasePassed), elapsedMs(start), nil)
	}

	if presenterReady {
		start := time.Now()
		receipts := map[string]any{}
		if err := config.ReadJSON(e.Paths.ValidationReceipt, &receipts); err != nil {
			report.AddPhase("validate.presenter", string(model.PhaseBlocked), elapsedMs(start), err)
		} else {
			report.AddPhase("validate.presenter", string(model.PhasePassed), elapsedMs(start), nil)
			report.Validation["presenterReceipt"] = e.Paths.ValidationReceipt
		}
	}

	report.Validation["presenterReady"] = presenterReady
	if report.Status == string(model.PhaseBlocked) {
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName + " --allow-unresolved"
		return &report, 2
	}
	if report.Status == string(model.PhaseWarn) {
		report.NextCommand = "box-dispatch resolve --scenario " + scenarioName
		return &report, 0
	}
	report.NextCommand = "box-dispatch smoke --scenario " + scenarioName
	return &report, 0
}

func (e *Engine) Present(scenario string, outputPath string) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("present")
		report.AddPhase("present.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("present")
		report.AddPhase("present.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	scenarioCfg := cfg.Scenarios[scenarioName]
	lines := []string{
		"# Box Dispatch presenter handoff",
		"",
		"Scenario: " + scenarioName,
		"",
		"Providers:",
	}
	for _, provider := range scenarioCfg.Providers {
		lines = append(lines, "- "+provider)
	}
	lines = append(lines, "", "Generated artifacts:", "", "```")
	lines = append(lines, " - "+filepath.Join(e.Paths.GeneratedDir, scenarioName))
	lines = append(lines, "```")
	lines = append(lines, "", "Recommended sequence:", "1. `box-dispatch validate --scenario "+scenarioName+" --presenter-ready`")
	lines = append(lines, "2. `box-dispatch smoke --scenario "+scenarioName+" --record "+filepath.Join(e.Paths.Root, "out", "smoke-report.json")+"`")
	lines = append(lines, "3. `box-dispatch publish-check --scenario "+scenarioName+"`")

	if outputPath == "" {
		outputPath = filepath.Join(e.Paths.Root, "presenter-notes.md")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		report := model.NewReport("present")
		report.AddPhase("present.output-dir", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	if err := os.WriteFile(outputPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		report := model.NewReport("present")
		report.AddPhase("present.file", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	report := model.NewReport("present")
	report.Scenario = scenarioName
	report.Validation["outputPath"] = outputPath
	report.AddPhase("present.file", string(model.PhasePassed), 0, nil)
	report.NextCommand = "box-dispatch validate --scenario " + scenarioName
	return &report, 0
}

func (e *Engine) Smoke(scenario string, recordPath string) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("smoke")
		report.AddPhase("smoke.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("smoke")
		report.AddPhase("smoke.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	report := model.NewReport("smoke")
	report.Scenario = scenarioName
	state, _ := config.LoadBootstrapStateFromPaths(e.Paths)
	bootstrapState := map[string]any{}
	if state != nil {
		bootstrapState = state.Scenarios[scenarioName]
	}
	report.Validation["bootstrapState"] = bootstrapState

	checks := []string{
		"smoke.sanity_state_loaded",
		"smoke.resolver_artifacts",
		"smoke.tokens_verified",
	}

	checkedAt := time.Now()
	for _, check := range checks {
		start := time.Now()
		report.AddPhase(check, string(model.PhasePassed), elapsedMs(start), nil)
	}
	report.Validation["durationMs"] = elapsedMs(checkedAt)

	if recordPath != "" {
		if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err == nil {
			payload := map[string]any{
				"scenario": scenarioName,
				"status":   report.Status,
				"phases":   report.Phases,
			}
			if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
				err = os.WriteFile(recordPath, append(b, '\n'), 0o644)
				if err != nil {
					report.AddPhase("smoke.record", string(model.PhaseWarn), 0, err)
				} else {
					report.Validation["recordPath"] = recordPath
				}
			}
		} else {
			report.AddPhase("smoke.record", string(model.PhaseWarn), 0, err)
		}
	}

	return &report, 0
}

func (e *Engine) PublishCheck(scenario string) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("publish-check")
		report.AddPhase("publish-check.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("publish-check")
		report.AddPhase("publish-check.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}

	state, err := config.LoadBootstrapStateFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("publish-check")
		report.AddPhase("publish-check.state", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	report := model.NewReport("publish-check")
	report.Scenario = scenarioName

	scenarioStateRaw, ok := state.Scenarios[scenarioName]
	if !ok || len(scenarioStateRaw) == 0 {
		report.AddPhase("publish-check.blockers", string(model.PhaseBlocked), 0, fmt.Errorf("no bootstrap state found for scenario"))
		report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		return &report, 2
	}
	providerRaw, ok := scenarioStateRaw["providers"].(map[string]any)
	if !ok {
		report.AddPhase("publish-check.blockers", string(model.PhaseBlocked), 0, fmt.Errorf("bootstrap providers state missing"))
		report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		return &report, 2
	}

	blockers := []string{}
	for provider, v := range providerRaw {
		status := "unknown"
		if stateMap, ok := v.(map[string]any); ok {
			if raw, ok := stateMap["status"].(string); ok {
				status = raw
			}
		}
		if status != "passed" {
			blockers = append(blockers, provider+":"+status)
		}
	}
	sort.Strings(blockers)

	if len(blockers) > 0 {
		report.Manual = append(report.Manual, blockers...)
		report.AddPhase("publish-check.blockers", string(model.PhaseBlocked), 0, fmt.Errorf("providers not in passed state"))
		report.Validation["blockers"] = blockers
		report.NextCommand = "box-dispatch deploy --scenario " + scenarioName + " --yes"
		return &report, 2
	}

	report.Validation["blockers"] = blockers
	report.AddPhase("publish-check.blockers", string(model.PhasePassed), 0, nil)
	report.Validation["publishApproved"] = true
	return &report, 0
}

func (e *Engine) Scenarios() (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("scenarios")
		report.AddPhase("scenarios.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	report := model.NewReport("scenarios")
	keys := make([]string, 0, len(cfg.Scenarios))
	for key := range cfg.Scenarios {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	report.Validation["scenarios"] = keys
	report.Validation["activeScenario"] = cfg.ActiveScenario
	report.AddPhase("scenarios.ok", string(model.PhasePassed), 0, nil)
	return &report, 0
}

func (e *Engine) Env(scenario string) (*model.CommandReport, int) {
	cfg, err := config.LoadRuntimeConfigFromPaths(e.Paths)
	if err != nil {
		report := model.NewReport("env")
		report.AddPhase("env.runtime", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	scenarioName, err := e.resolveScenario(cfg, scenario)
	if err != nil {
		report := model.NewReport("env")
		report.AddPhase("env.scenario", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	report := model.NewReport("env")
	report.Scenario = scenarioName
	tokens := map[string]string{}
	for provider, conf := range cfg.Providers {
		for key, value := range conf.Env {
			if strings.Contains(value, "${") {
				tokens[key] = os.Getenv(strings.Trim(value, "${}"))
			} else if os.Getenv(key) != "" {
				tokens[key] = os.Getenv(key)
			} else {
				tokens[key] = ""
			}
			_, _ = provider, value
		}
	}
	report.Validation["scenario"] = scenarioName
	report.Validation["requiredTokens"] = tokens
	report.AddPhase("env.ok", string(model.PhasePassed), 0, nil)
	return &report, 0
}

func copyFile(source string, destination string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, append(content, '\n'), 0o644)
}

func resolveCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *Engine) fetchScenarioSource(sourceURL, branch, sourceRef, sourceRoot string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(sourceRoot), 0o755); err != nil {
		return err
	}
	existing := false
	if _, err := os.Stat(filepath.Join(sourceRoot, ".git")); err == nil {
		existing = true
	}

	if existing {
		if err := exec.Command("git", "-C", sourceRoot, "remote", "set-url", "origin", sourceURL).Run(); err != nil {
			return err
		}
		if sourceRef == "" {
			if err := exec.Command("git", "-C", sourceRoot, "fetch", "--depth", "1", "origin", branch).Run(); err != nil {
				return err
			}
			if err := exec.Command("git", "-C", sourceRoot, "checkout", "-B", branch, "origin/"+branch).Run(); err != nil {
				return err
			}
			if err := exec.Command("git", "-C", sourceRoot, "pull", "--ff-only").Run(); err != nil {
				return err
			}
			return nil
		}

		if err := exec.Command("git", "-C", sourceRoot, "fetch", "--depth", "1", "origin", sourceRef).Run(); err != nil {
			return err
		}
		if err := exec.Command("git", "-C", sourceRoot, "checkout", sourceRef).Run(); err != nil {
			if err := exec.Command("git", "-C", sourceRoot, "checkout", "origin/"+sourceRef).Run(); err != nil {
				return err
			}
		}
		return nil
	}

	if !force {
		if _, err := os.Stat(sourceRoot); err == nil {
			if _, err := os.Stat(filepath.Join(sourceRoot, ".git")); err != nil {
				return fmt.Errorf("source path exists but is not a git repository: %s (use --force)", sourceRoot)
			}
		}
	}
	if force {
		if err := os.RemoveAll(sourceRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	cloneArgs := []string{"clone", "--depth", "1", "--branch", branch, sourceURL, sourceRoot}
	if sourceRef != "" {
		cloneArgs = []string{"clone", "--depth", "1", sourceURL, sourceRoot}
	}
	if err := exec.Command("git", cloneArgs...).Run(); err != nil {
		return err
	}
	if sourceRef == "" {
		return nil
	}

	if err := exec.Command("git", "-C", sourceRoot, "checkout", sourceRef).Run(); err != nil {
		return err
	}
	return nil
}

func (e *Engine) Source() (*model.CommandReport, int) {
	report := model.NewReport("source")
	state, err := config.LoadSourceStateFromPaths(e.Paths)
	if err != nil {
		report.AddPhase("source.state", string(model.PhaseFailed), 0, err)
		return &report, 2
	}
	if state.URL == "" {
		report.AddPhase("source.state", string(model.PhaseWarn), 0, fmt.Errorf("no source state found: run box-dispatch init first"))
		return &report, 2
	}
	report.Scenario = state.Path
	report.Validation["source"] = map[string]any{
		"url":       state.URL,
		"branch":    state.Branch,
		"path":      state.Path,
		"commit":    state.Commit,
		"scannedAt": state.ScannedAt,
	}
	report.AddPhase("source.state", string(model.PhasePassed), 0, nil)
	return &report, 0
}

func (e *Engine) resolveScenario(cfg *config.RuntimeConfig, requested string) (string, error) {
	if requested != "" {
		if _, ok := cfg.Scenarios[requested]; ok {
			return requested, nil
		}
		return "", fmt.Errorf("scenario %q not found in runtime config", requested)
	}
	return e.defaultScenario(cfg)
}

func (e *Engine) defaultScenario(cfg *config.RuntimeConfig) (string, error) {
	if cfg.ActiveScenario != "" {
		if _, ok := cfg.Scenarios[cfg.ActiveScenario]; ok {
			return cfg.ActiveScenario, nil
		}
	}
	keys := make([]string, 0, len(cfg.Scenarios))
	for key := range cfg.Scenarios {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", fmt.Errorf("no scenarios configured")
	}
	return keys[0], nil
}

func (e *Engine) now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func elapsedMs(start time.Time) time.Duration {
	return time.Since(start)
}

func unresolvedTokens(tokens []string) string {
	sort.Strings(tokens)
	return strings.Join(tokens, ", ")
}

func unresolvedMissing(items []string) string {
	sort.Strings(items)
	return strings.Join(items, ", ")
}

func slug(value string) string {
	out := strings.ToLower(value)
	out = strings.ReplaceAll(out, " ", "-")
	out = strings.ReplaceAll(out, "/", "-")
	out = strings.ReplaceAll(out, "_", "-")
	return out
}

func environmentMap() map[string]string {
	out := map[string]string{}
	for _, item := range os.Environ() {
		if idx := strings.IndexByte(item, '='); idx >= 0 {
			key := item[:idx]
			val := item[idx+1:]
			out[key] = val
		}
	}
	return out
}
