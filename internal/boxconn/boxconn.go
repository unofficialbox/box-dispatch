// Package boxconn models the Box connections box-dispatch can authenticate with:
// the box CLI's environments (each OAuth2, CCG or JWT) plus the box-dispatch CCG
// app. It is shared by the connectivity check (type badge), the shell (switcher)
// and the lifecycle (which connection to use).
package boxconn

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

const (
	SourceCLI      = "cli"      // a box CLI environment
	SourceDispatch = "dispatch" // credentials captured by box-dispatch itself

	// DispatchCCGName is the stable name of the box-dispatch CCG connection, used
	// as the default pointer value and in the switcher.
	DispatchCCGName = "box-dispatch-ccg"
)

// Connection is one selectable Box authentication.
type Connection struct {
	Name     string // CLI environment name, or DispatchCCGName
	AuthType string // "OAuth2" | "CCG" | "JWT" | raw method
	Source   string // SourceCLI or SourceDispatch
	Current  bool   // the CLI's current default environment (CLI connections only)
	Default  bool   // box-dispatch's pinned default
	Detail   string // human context (e.g. subject for CCG)
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// authLabel normalises the box CLI's auth-method names.
func authLabel(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "oauth20", "oauth2", "oauth":
		return "OAuth2"
	case "ccg", "client_credentials":
		return "CCG"
	case "jwt":
		return "JWT"
	default:
		return strings.TrimSpace(method)
	}
}

// ParseCLIEnvironments extracts name+auth pairs from the text output of
// `box configure:environments:get`. Each environment block carries an indented
// "Name:" then "Auth Method:" line.
func ParseCLIEnvironments(output string) []Connection {
	connections := []Connection{}
	name, auth := "", ""
	flush := func() {
		if name != "" {
			connections = append(connections, Connection{Name: name, AuthType: authLabel(auth), Source: SourceCLI})
		}
		name, auth = "", ""
	}
	for _, line := range strings.Split(ansi.ReplaceAllString(output, ""), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Name:"):
			flush()
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "Name:"))
		case strings.HasPrefix(trimmed, "Auth Method:"):
			auth = strings.TrimSpace(strings.TrimPrefix(trimmed, "Auth Method:"))
		}
	}
	flush()
	return connections
}

// firstName returns the first "Name:" value in CLI output.
func firstName(output string) string {
	for _, line := range strings.Split(ansi.ReplaceAllString(output, ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Name:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "Name:"))
		}
	}
	return ""
}

func run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "box", args...).Output()
	return string(out), err
}

// SetCLICurrent points the box CLI's current environment at name. The CLI has no
// per-command environment flag, so selecting a CLI connection for box-dispatch
// means mutating this global state (see docs/ROADMAP.md item 2).
func SetCLICurrent(name string) error {
	_, err := run("configure:environments:set-current", name)
	return err
}

// List returns every known Box connection: the CLI environments plus the
// box-dispatch CCG when captured, with Current and Default marked.
func List() []Connection {
	connections := []Connection{}
	if out, err := run("configure:environments:get"); err == nil {
		connections = ParseCLIEnvironments(out)
	}

	current := ""
	if out, err := run("configure:environments:get", "-c"); err == nil {
		current = firstName(out)
	}

	settings, _ := shellstate.LoadConnectionSettings()
	if settings.HasBoxCCG() {
		detail := "box-dispatch app, subject " + settings.BoxCCGSubjectType + " " + settings.BoxCCGSubjectID
		connections = append(connections, Connection{Name: DispatchCCGName, AuthType: "CCG", Source: SourceDispatch, Detail: detail})
	}

	return markState(connections, current, settings.BoxDefaultConnection)
}

// markState flags Current (CLI) and Default across the connection set.
func markState(connections []Connection, current, def string) []Connection {
	for i := range connections {
		connections[i].Current = connections[i].Source == SourceCLI && connections[i].Name == current
		connections[i].Default = connections[i].Name == def
	}
	return connections
}

// Active reports the connection box-dispatch will actually use for Box calls:
// the pinned default when it still exists, otherwise the box-dispatch CCG when
// configured, otherwise the CLI's current environment.
func Active() (Connection, bool) {
	connections := List()
	settings, _ := shellstate.LoadConnectionSettings()

	if settings.BoxDefaultConnection != "" {
		for _, connection := range connections {
			if connection.Name == settings.BoxDefaultConnection {
				return connection, true
			}
		}
	}
	if settings.HasBoxCCG() {
		for _, connection := range connections {
			if connection.Source == SourceDispatch {
				return connection, true
			}
		}
	}
	for _, connection := range connections {
		if connection.Current {
			return connection, true
		}
	}
	return Connection{}, false
}
