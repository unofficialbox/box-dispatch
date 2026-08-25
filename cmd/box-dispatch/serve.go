package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unofficialbox/box-dispatch/internal/webapi"
	"github.com/unofficialbox/box-dispatch/internal/webui"
)

func makeServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Dispatch web application",
		Long:  "Starts the complete Dispatch web application on a loopback-only address. The executable serves both the browser interface and its local API, which owns credentials, deployment history, BCL plan state, validation, and deployment.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			port, _ := cmd.Flags().GetInt("port")
			noOpen, _ := cmd.Flags().GetBool("no-open")
			return runWebApplication(cmd, port, !noOpen)
		},
	}
	addWebApplicationFlags(cmd)
	return cmd
}

func makeTerminalCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "terminal",
		Short: "Open the legacy full-screen terminal interface",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLaunchShell()
		},
	}
}

func makeMockCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Serve the browser workspace with an in-memory mock API",
		Long:  "Starts the browser workspace against a deterministic in-memory API. It does not read provider credentials, clone packages, or call Box or Salesforce.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			port, _ := cmd.Flags().GetInt("port")
			noOpen, _ := cmd.Flags().GetBool("no-open")
			return runMockWebApplication(cmd, port, !noOpen)
		},
	}
	cmd.Flags().Int("port", 8788, "loopback port for the mocked Dispatch web application")
	cmd.Flags().Bool("no-open", false, "serve without opening a browser")
	return cmd
}

func addWebApplicationFlags(cmd *cobra.Command) {
	cmd.Flags().Int("port", 8787, "loopback port for the Dispatch web application")
	cmd.Flags().Bool("no-open", false, "serve without opening a browser")
	cmd.Flags().String("record-api", "", "write credential-redacted browser API requests and responses as JSON Lines")
}

func dispatchWebHandler(profile string) http.Handler {
	return combineDispatchHandlers(webapi.NewHandler(profile), webui.Handler())
}

func combineDispatchHandlers(api, ui http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		ui.ServeHTTP(w, r)
	})
}

func runWebApplication(cmd *cobra.Command, port int, openBrowser bool) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start Dispatch web application: %w", err)
	}
	defer listener.Close()

	var api http.Handler = webapi.NewHandler(profileFromCommand(cmd))
	recordPath, _ := cmd.Flags().GetString("record-api")
	if strings.TrimSpace(recordPath) != "" {
		recorder, recordErr := webapi.NewHTTPRecorder(recordPath)
		if recordErr != nil {
			return recordErr
		}
		defer recorder.Close()
		api = recorder.Wrap(api)
		fmt.Fprintf(cmd.OutOrStdout(), "Recording credential-redacted browser API traffic to %s\n", recordPath)
	}
	server := &http.Server{
		Addr:              address,
		Handler:           combineDispatchHandlers(api, webui.Handler()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	url := "http://" + address
	fmt.Fprintf(cmd.OutOrStdout(), "Dispatch is running at %s\n", url)
	if openBrowser {
		if open := browserOpenCommand(runtime.GOOS, url); open == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your browser.\n", url)
		} else if err := open.Start(); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your browser. Automatic launch failed: %v\n", url, err)
		}
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runMockWebApplication(cmd *cobra.Command, port int, openBrowser bool) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start mocked Dispatch web application: %w", err)
	}
	defer listener.Close()
	server := &http.Server{
		Addr: address, Handler: combineDispatchHandlers(webapi.NewMockHandler(), webui.Handler()), ReadHeaderTimeout: 5 * time.Second,
	}
	url := "http://" + address
	fmt.Fprintf(cmd.OutOrStdout(), "Mock Dispatch is running at %s (no provider calls)\n", url)
	if openBrowser {
		if open := browserOpenCommand(runtime.GOOS, url); open == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your browser.\n", url)
		} else if err := open.Start(); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your browser. Automatic launch failed: %v\n", url, err)
		}
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
