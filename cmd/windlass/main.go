package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/massnerder/box-windlass/internal/checker"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	root := &cobra.Command{
		Use:   "windlass",
		Short: "Windlass - solution shipping accelerators for Box and partners",
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Validate tool, config, and connectivity prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario, _ := cmd.Flags().GetString("scenario")
			platform, _ := cmd.Flags().GetString("platform")
			offline, _ := cmd.Flags().GetBool("offline")
			asJSON, _ := cmd.Flags().GetBool("json")
			interactive := isTTY() && !asJSON

			providers := checker.ProvidersForScenarioAndPlatform(scenario, platform)
			cfg := checker.CheckConfig{
				Scenario: scenario,
				Platform: platform,
				Offline:  offline,
				Providers: providers,
			}

			var report checker.CheckReport
			var err error

			if interactive {
				p := checker.NewInteractiveCheckModel(cfg)
				if _, err := tea.NewProgram(p).Run(); err != nil {
					return fmt.Errorf("interactive check failed: %w", err)
				}
				report = p.Report()
			} else {
				report, err = checker.Check(cfg)
				if err != nil {
					return err
				}
			}

			if asJSON {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			return printTextReport(cmd, report)
		},
	}

	checkCmd.Flags().String("scenario", "", "scenario filter (e.g. clm, lifesciences, all)")
	checkCmd.Flags().String("platform", "", "provider filter (box, salesforce, databricks, aws)")
	checkCmd.Flags().Bool("offline", false, "skip live connectivity checks")
	checkCmd.Flags().Bool("json", false, "machine readable JSON output")

	root.AddCommand(checkCmd)

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func printTextReport(cmd *cobra.Command, report checker.CheckReport) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "\nWindlass Connectivity Check")
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
