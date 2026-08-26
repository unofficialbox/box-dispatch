package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/unofficialbox/box-dispatch/internal/webapi"
	"github.com/unofficialbox/box-dispatch/internal/webui"
)

func browserOpenCommand(goos, target string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", target)
	case "linux":
		return exec.Command("xdg-open", target)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		return nil
	}
}

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

func makeMockCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Serve the browser workspace with an in-memory mock API",
		Long:  "Starts the browser workspace against a deterministic in-memory API. It does not read provider credentials, clone packages, or call Box or Salesforce.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			port, _ := cmd.Flags().GetInt("port")
			noOpen, _ := cmd.Flags().GetBool("no-open")
			validationFailureProvider, _ := cmd.Flags().GetString("fail-validation-provider")
			connectionFailureProvider, _ := cmd.Flags().GetString("fail-connection-provider")
			return runMockWebApplication(cmd, port, !noOpen, validationFailureProvider, connectionFailureProvider)
		},
	}
	cmd.Flags().Int("port", 8788, "loopback port for the mocked Dispatch web application")
	cmd.Flags().Bool("no-open", false, "serve without opening a browser")
	cmd.Flags().String("fail-validation-provider", "", "simulate stale authentication for a provider during validation")
	cmd.Flags().String("fail-connection-provider", "", "simulate stale authentication during a provider connection check")
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

func salesforceLoginRedirectHandler(api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/OauthRedirect" {
			http.NotFound(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}

func boxLoginRedirectHandler(api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/callback" {
			http.NotFound(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}

func runWebApplication(cmd *cobra.Command, port int, openBrowser bool) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start Dispatch web application: %w", err)
	}
	defer listener.Close()

	var salesforceCallbackReady, boxCallbackReady atomic.Bool
	var api http.Handler = webapi.NewHandlerWithOptions(webapi.ServerOptions{
		Profile:                 profileFromCommand(cmd),
		SalesforceCallbackReady: salesforceCallbackReady.Load,
		BoxCallbackReady:        boxCallbackReady.Load,
	})
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
	if oauthListener, oauthErr := net.Listen("tcp", "127.0.0.1:1717"); oauthErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Salesforce login needs port 1717. Stop a Salesforce CLI login (or anything else using 1717) and restart Dispatch.\n")
	} else {
		defer oauthListener.Close()
		salesforceCallbackReady.Store(true)
		oauthServer := &http.Server{Handler: salesforceLoginRedirectHandler(api), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = oauthServer.Serve(oauthListener) }()
	}
	if boxListener, boxErr := net.Listen("tcp", "127.0.0.1:4400"); boxErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Box login needs port 4400. Stop anything using 4400 and restart Dispatch.\n")
	} else {
		defer boxListener.Close()
		boxCallbackReady.Store(true)
		boxServer := &http.Server{Handler: boxLoginRedirectHandler(api), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = boxServer.Serve(boxListener) }()
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

func runMockWebApplication(cmd *cobra.Command, port int, openBrowser bool, validationFailureProvider, connectionFailureProvider string) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start mocked Dispatch web application: %w", err)
	}
	defer listener.Close()
	server := &http.Server{
		Addr: address, Handler: combineDispatchHandlers(webapi.NewMockHandlerWithOptions(webapi.MockOptions{
			ValidationFailureProvider: strings.ToLower(strings.TrimSpace(validationFailureProvider)),
			ConnectionFailureProvider: strings.ToLower(strings.TrimSpace(connectionFailureProvider)),
		}), webui.Handler()), ReadHeaderTimeout: 5 * time.Second,
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
