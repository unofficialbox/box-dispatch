package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/unofficialbox/box-dispatch/internal/checker"
	"github.com/unofficialbox/box-dispatch/internal/engine"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"golang.org/x/term"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

const (
	commonCommandGroup   = "common"
	advancedCommandGroup = "advanced"
)

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "box-dispatch",
		Short: "Box Dispatch - solution shipping accelerators for Box and partners",
		Long:  "Run without a subcommand for the interactive launch experience.",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			platform, _ := cmd.Flags().GetString("platform")
			offline, _ := cmd.Flags().GetBool("offline")
			// The full-screen shell needs a terminal; anything scripted or
			// JSON-bound falls back to the plain connectivity report.
			if isTTY() && terminalSupportsFullScreen() && !asJSON(cmd) {
				return runLaunchShell()
			}
			return runConnectivityCheck(cmd, scenario, platform, offline, false)
		},
		SilenceUsage: true,
	}
	root.PersistentFlags().String("profile", "", "configuration profile (defaults to BOX_DISPATCH_PROFILE or default)")
	root.PersistentFlags().Bool("json", false, "machine-readable JSON output")
	root.PersistentFlags().Bool("no-color", false, "disable ANSI color output")
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		noColor, _ := cmd.Flags().GetBool("no-color")
		configureTerminalPresentation(noColor)
	}
	addConnectivityFlags(root)

	root.AddGroup(
		&cobra.Group{ID: commonCommandGroup, Title: "Common Commands:"},
		&cobra.Group{ID: advancedCommandGroup, Title: "Advanced Commands:"},
	)
	root.SetHelpCommandGroupID(advancedCommandGroup)
	root.SetCompletionCommandGroupID(advancedCommandGroup)

	commonCommands := []*cobra.Command{
		makeDeployCommand(),
		makeCheckCommand(),
		makeStatusCommand(),
		makeResetCommand(),
	}
	advancedCommands := []*cobra.Command{
		makeInitCommand(),
		makeSetupCommand(),
		makeResolveCommand(),
		makeSourceCommand(),
		makeScenariosCommand(),
		makeEnvCommand(),
		makeImportCommand(),
		makeValidateCommand(),
		makePresentCommand(),
		makeSmokeCommand(),
		makePublishCheckCommand(),
	}
	for _, cmd := range commonCommands {
		cmd.GroupID = commonCommandGroup
	}
	for _, cmd := range advancedCommands {
		cmd.GroupID = advancedCommandGroup
	}
	root.AddCommand(append(commonCommands, advancedCommands...)...)
	return root
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func terminalSupportsFullScreen() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb")
}

func configureTerminalPresentation(explicitNoColor bool) {
	if explicitNoColor || os.Getenv("NO_COLOR") != "" || !terminalSupportsFullScreen() {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

func asJSON(cmd *cobra.Command) bool {
	value, err := cmd.Flags().GetBool("json")
	return err == nil && value
}

func runWithEngine(cmd *cobra.Command, runFn func(*engine.Engine) (*model.CommandReport, int)) error {
	return runReportCommand(cmd, func() (*model.CommandReport, int) {
		return runFn(engine.NewEngine(profileFromCommand(cmd)))
	})
}

func profileFromCommand(cmd *cobra.Command) string {
	profile, err := cmd.Flags().GetString("profile")
	if err == nil {
		profile = strings.TrimSpace(profile)
	}
	if profile != "" {
		return profile
	}
	return strings.TrimSpace(os.Getenv("BOX_DISPATCH_PROFILE"))
}

func makeCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate tool, config, and connectivity prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			platform, _ := cmd.Flags().GetString("platform")
			offline, _ := cmd.Flags().GetBool("offline")
			return runConnectivityCheck(cmd, scenario, platform, offline, isTTY())
		},
	}

	addConnectivityFlags(cmd)

	return cmd
}

func addConnectivityFlags(cmd *cobra.Command) {
	cmd.Flags().String("scenario", "", "scenario filter (e.g. clm, lifesciences, all)")
	cmd.Flags().String("platform", "", "provider filter (box, salesforce, databricks, aws)")
	cmd.Flags().Bool("offline", false, "skip live connectivity checks")
}

// runLaunchShell starts the full-screen solution launch wizard.
func runLaunchShell() error {
	program := tea.NewProgram(newDispatchShell(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("launch shell failed: %w", err)
	}
	return nil
}

func runResetShell() error {
	program := tea.NewProgram(newResetShell(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("reset shell failed: %w", err)
	}
	return nil
}

func runConnectivityCheck(cmd *cobra.Command, scenario, platform string, offline, preferInteractive bool) error {
	cfg := checker.CheckConfig{
		Scenario:  scenario,
		Platform:  platform,
		Offline:   offline,
		Providers: checker.ProvidersForScenarioAndPlatform(scenario, platform),
	}

	var report checker.CheckReport
	var err error

	interactive := preferInteractive && terminalSupportsFullScreen() && !asJSON(cmd)
	if interactive {
		p := checker.NewInteractiveCheckModel(cfg)
		if _, err := tea.NewProgram(p).Run(); err != nil {
			// Fall back to non-interactive text mode if interactive execution is unavailable.
			report, err = checker.Check(cfg)
			if err != nil {
				return err
			}
			if asJSON(cmd) {
				return writeJSON(cmd, report)
			}
			return printTextReport(cmd, report)
		}
		if asJSON(cmd) {
			return writeJSON(cmd, p.Report())
		}
		return nil
	} else {
		report, err = checker.Check(cfg)
		if err != nil {
			return err
		}
	}

	if asJSON(cmd) {
		return writeJSON(cmd, report)
	}
	return printTextReport(cmd, report)
}

func makeInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize local Box Dispatch runtime from a scenario source repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceURL, _ := cmd.Flags().GetString("source-url")
			branch, _ := cmd.Flags().GetString("branch")
			scenarioPath, _ := cmd.Flags().GetString("scenario-path")
			sourceRef, _ := cmd.Flags().GetString("source-ref")
			force, _ := cmd.Flags().GetBool("force")

			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Init(sourceURL, branch, scenarioPath, sourceRef, force)
			})
		},
	}

	cmd.Flags().String("source-url", "", "git remote URL for scenario source repository")
	cmd.Flags().String("branch", "main", "source branch to checkout")
	cmd.Flags().String("scenario-path", ".", "path to scenario directory inside source repo")
	cmd.Flags().String("source-ref", "", "optional source ref (tag/sha/branch alias)")
	cmd.Flags().Bool("force", false, "overwrite existing source checkout if needed")
	return cmd
}

func makeSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Bootstrap runtime config from example template",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Setup(force)
			})
		},
	}
	cmd.Flags().Bool("force", false, "overwrite runtime config from example file")
	return cmd
}

func makeResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve provider configuration and materialize scenario artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			allowUnresolved, _ := cmd.Flags().GetBool("allow-unresolved")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Resolve(scenario, dryRun, allowUnresolved)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	cmd.Flags().Bool("dry-run", false, "render outputs only")
	cmd.Flags().Bool("allow-unresolved", false, "continue when tokens are unresolved")
	return cmd
}

func makeDeployCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy",
		Aliases: []string{"bootstrap"},
		Short:   "Deploy resolved artifacts and provider configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			yes, _ := cmd.Flags().GetBool("yes")
			allowUnresolved, _ := cmd.Flags().GetBool("allow-unresolved")
			skipValidate, _ := cmd.Flags().GetBool("skip-validate")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Bootstrap(scenario, dryRun, yes, allowUnresolved, skipValidate)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	cmd.Flags().Bool("dry-run", false, "run through bootstrap steps without writing state")
	cmd.Flags().Bool("yes", false, "apply without interactive confirmation")
	cmd.Flags().Bool("allow-unresolved", false, "continue when tokens are unresolved")
	cmd.Flags().Bool("skip-validate", false, "skip provider-level precheck validation")
	return cmd
}

func makeResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset a recorded deployment by its audited resource IDs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON(cmd) || !isTTY() {
				return fmt.Errorf("reset requires an interactive terminal because it previews recorded resources and requires typed confirmation")
			}
			return runResetShell()
		},
	}
}

func makeSourceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "source",
		Short: "Show metadata about the currently configured scenario source",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Source()
			})
		},
	}
}

func makeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status and unresolved tokens for the selected scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Status(scenario)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	return cmd
}

func makeScenariosCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scenarios",
		Short: "List configured scenarios from runtime config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Scenarios()
			})
		},
	}
}

func makeEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Render required environment token state for a scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Env(scenario)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	return cmd
}

func makeValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate bootstrap artifacts and deployment state",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			presenterReady, _ := cmd.Flags().GetBool("presenter-ready")
			skipArtifacts, _ := cmd.Flags().GetBool("skip-artifacts")
			legacyOffline, _ := cmd.Flags().GetBool("offline")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Validate(scenario, presenterReady, skipArtifacts || legacyOffline)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	cmd.Flags().Bool("presenter-ready", false, "validate generated presenter receipt presence")
	cmd.Flags().Bool("skip-artifacts", false, "skip generated artifact presence checks")
	cmd.Flags().Bool("offline", false, "deprecated compatibility alias for --skip-artifacts")
	_ = cmd.Flags().MarkDeprecated("offline", "use --skip-artifacts")
	_ = cmd.Flags().MarkHidden("offline")
	return cmd
}

func makePresentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "present",
		Short: "Generate a presenter handoff note for the active scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			output, _ := cmd.Flags().GetString("output")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Present(scenario, output)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	cmd.Flags().String("output", "", "output markdown path (defaults to presenter-notes.md)")
	return cmd
}

func makeSmokeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smoke",
		Short: "Run smoke checks for the selected scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			record, _ := cmd.Flags().GetString("record")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Smoke(scenario, record)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	cmd.Flags().String("record", "", "optional path for smoke record JSON output")
	return cmd
}

func makePublishCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish-check",
		Short: "Verify deploy blockers before publish handoff",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.PublishCheck(scenario)
			})
		},
	}
	cmd.Flags().String("scenario", "", "scenario key (defaults to active scenario)")
	return cmd
}

func runReportCommand(cmd *cobra.Command, runFn func() (*model.CommandReport, int)) error {
	report, exitCode := runFn()
	if asJSON(cmd) {
		return writeJSON(cmd, report)
	}
	if err := printEngineReport(cmd, report); err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("command failed: %s", cmd.CommandPath())
	}
	return nil
}

func printEngineReport(cmd *cobra.Command, report *model.CommandReport) error {
	out := cmd.OutOrStdout()
	if report == nil {
		return fmt.Errorf("empty report")
	}

	_, _ = fmt.Fprintf(out, "\nBox Dispatch %s report\n", report.Command)
	_, _ = fmt.Fprintf(out, "Started: %s\n", report.StartedAt)
	_, _ = fmt.Fprintf(out, "Status:  %s\n", strings.ToUpper(report.Status))

	if report.Scenario != "" {
		_, _ = fmt.Fprintf(out, "Scenario: %s\n", report.Scenario)
	}
	if report.DryRun {
		_, _ = fmt.Fprintln(out, "Mode: dry-run")
	}
	if report.ConfirmRequired {
		_, _ = fmt.Fprintln(out, "Note: confirmation required")
	}

	if len(report.Phases) > 0 {
		_, _ = fmt.Fprintln(out, "\nPhases:")
		for _, phase := range report.Phases {
			icon := reportStatusIcon(phase.Status)
			line := fmt.Sprintf("  %s %s (%dms)", icon, phase.Name, phase.DurationMs)
			if phase.Error != "" {
				line += " - " + phase.Error
			}
			_, _ = fmt.Fprintln(out, line)
		}
	}

	if len(report.Validation) > 0 {
		_, _ = fmt.Fprintln(out, "\nValidation:")
		keys := make([]string, 0, len(report.Validation))
		for key := range report.Validation {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			_, _ = fmt.Fprintf(out, "  - %s: %v\n", key, report.Validation[key])
		}
	}

	if len(report.Manual) > 0 {
		_, _ = fmt.Fprintln(out, "\nManual:")
		for _, item := range report.Manual {
			_, _ = fmt.Fprintf(out, "  - %s\n", item)
		}
	}

	if report.NextCommand != "" {
		_, _ = fmt.Fprintf(out, "\nNext: %s\n", report.NextCommand)
	}

	return nil
}

func printTextReport(cmd *cobra.Command, report checker.CheckReport) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "\nBox Dispatch Connectivity Check")
	_, _ = fmt.Fprintln(out, "=========================")

	allGood := true
	for _, p := range report.Providers {
		icon := "✓"
		if p.Blocked {
			icon = "✗"
			allGood = false
		} else if p.RequiresAttention {
			icon = "!"
			allGood = false
		}

		status := "pass"
		if p.RequiresAttention {
			status = "warn"
		}
		if p.Blocked {
			status = "blocked"
		}

		_, _ = fmt.Fprintf(out, "\n%s %s (%s)\n", icon, strings.ToUpper(p.Name), status)
		for _, c := range p.Checks {
			_, _ = fmt.Fprintf(out, "  - %s\n", c)
		}
		if len(p.Guidance) > 0 {
			_, _ = fmt.Fprintln(out, "  Guide:")
			for _, g := range p.Guidance {
				_, _ = fmt.Fprintf(out, "    %s\n", g)
			}
		}
	}

	if allGood {
		_, _ = fmt.Fprintln(out, "\nAll checks passed. You are ready to move forward.")
	} else {
		_, _ = fmt.Fprintln(out, "\nOne or more providers needs setup. Complete each guide section above.")
	}
	return nil
}

func writeJSON(cmd *cobra.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func reportStatusIcon(status string) string {
	switch status {
	case "passed":
		return "✓"
	case "warn":
		return "!"
	case "blocked", "failed":
		return "✗"
	case "skipped":
		return "•"
	default:
		return "?"
	}
}
