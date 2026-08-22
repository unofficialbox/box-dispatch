package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/unofficialbox/box-dispatch/internal/webapi"
)

func makeServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local API for the Dispatch web app",
		Long:  "Starts a loopback-only API for the Dispatch web app. It exposes sanitized local state, deployment history, and a saved BCL plan draft. The browser can explicitly validate and apply a saved plan through the same Dispatch lifecycle services.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			port, _ := cmd.Flags().GetInt("port")
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			server := &http.Server{
				Addr:              address,
				Handler:           webapi.NewHandler(profileFromCommand(cmd)),
				ReadHeaderTimeout: 5 * time.Second,
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Dispatch web API listening on http://%s\n", address)
			return server.ListenAndServe()
		},
	}
	cmd.Flags().Int("port", 8787, "loopback port for the local web API")
	return cmd
}
