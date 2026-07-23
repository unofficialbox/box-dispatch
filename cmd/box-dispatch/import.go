package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/engine"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"github.com/spf13/cobra"
)

func makeImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [source]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Import deployment manifest into the active profile",
		Long:  "Import a Box Dispatch deployment manifest (`*.bcl`) and generate runtime config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, _ := cmd.Flags().GetString("source")
			if len(args) == 1 {
				if source != "" {
					return fmt.Errorf("provide import source either as positional argument or --source, not both")
				}
				source = args[0]
			}
			scenario, _ := cmd.Flags().GetString("scenario")
			force, _ := cmd.Flags().GetBool("force")
			format, _ := cmd.Flags().GetString("format")

			if source == "" {
				return fmt.Errorf("missing source path")
			}
			if _, err := os.Stat(source); err != nil {
				return err
			}
			format = strings.TrimSpace(format)
			if format == "" {
				format = "bcl"
			}
			if format != "bcl" {
				return fmt.Errorf("import format %q is not supported", format)
			}

			return runWithEngine(cmd, func(e *engine.Engine) (*model.CommandReport, int) {
				return e.Import(source, scenario, format, force)
			})
		},
	}

	cmd.Flags().String("source", "", "path to deployment manifest")
	cmd.Flags().String("scenario", "", "target scenario name (for artifact imports)")
	cmd.Flags().String("format", "bcl", "import format: bcl")
	cmd.Flags().Bool("force", false, "overwrite existing runtime config")
	cmd.Flags().Lookup("format").NoOptDefVal = "bcl"
	return cmd
}
