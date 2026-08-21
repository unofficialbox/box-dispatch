package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain isolates the runtime config directory so ambient machine config does
// not leak into shell construction. Tests that need a specific runtime config
// seed their own via t.Setenv("XDG_CONFIG_HOME", ...), which overrides this for
// the duration of that test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "box-dispatch-test-config")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestRootHelpGroupsCommonAndAdvancedCommands(t *testing.T) {
	root := newRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	commonStart := strings.Index(help, "Common Commands:")
	advancedStart := strings.Index(help, "Advanced Commands:")
	if commonStart < 0 || advancedStart <= commonStart {
		t.Fatalf("root help omitted ordered command groups:\n%s", help)
	}
	common := help[commonStart:advancedStart]
	for _, command := range []string{"check", "deploy", "reset", "status"} {
		commandRow := "\n  " + command + " "
		if !strings.Contains(common, commandRow) {
			t.Errorf("common help omitted %q:\n%s", command, common)
		}
	}
	for _, command := range []string{"init", "resolve", "validate", "present", "serve"} {
		commandRow := "\n  " + command + " "
		if strings.Contains(common, commandRow) {
			t.Errorf("advanced command %q leaked into common help:\n%s", command, common)
		}
		if !strings.Contains(help[advancedStart:], commandRow) {
			t.Errorf("advanced help omitted %q:\n%s", command, help[advancedStart:])
		}
	}
}

func TestRootHelpExposesNoColorFlag(t *testing.T) {
	root := newRootCommand()
	if flag := root.PersistentFlags().Lookup("no-color"); flag == nil || !strings.Contains(flag.Usage, "ANSI color") {
		t.Fatalf("--no-color flag = %#v", flag)
	}
}

func TestTerminalPresentationHonorsNoColorAndDumbTerminal(t *testing.T) {
	original := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("NO_COLOR", "1")
	configureTerminalPresentation(false)
	if got := lipgloss.ColorProfile(); got != termenv.Ascii {
		t.Fatalf("NO_COLOR profile = %v, want Ascii", got)
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	configureTerminalPresentation(true)
	if got := lipgloss.ColorProfile(); got != termenv.Ascii {
		t.Fatalf("explicit --no-color profile = %v, want Ascii", got)
	}

	t.Setenv("TERM", "dumb")
	if terminalSupportsFullScreen() {
		t.Fatal("TERM=dumb should disable the full-screen shell")
	}
}

func TestDeployIsCanonicalAndBootstrapRemainsAlias(t *testing.T) {
	root := newRootCommand()
	deploy, _, err := root.Find([]string{"deploy"})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, _, err := root.Find([]string{"bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	if deploy != bootstrap || deploy.Name() != "deploy" {
		t.Fatalf("deploy=%q bootstrap=%q, want one canonical deploy command", deploy.Name(), bootstrap.Name())
	}
	if !strings.Contains(strings.Join(deploy.Aliases, " "), "bootstrap") {
		t.Fatalf("deploy aliases = %#v, want bootstrap compatibility", deploy.Aliases)
	}
}

func TestConnectivityOfflineFlagIsConsistent(t *testing.T) {
	root := newRootCommand()
	check, _, err := root.Find([]string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	rootOffline := root.Flags().Lookup("offline")
	checkOffline := check.Flags().Lookup("offline")
	if rootOffline == nil || checkOffline == nil || rootOffline.Usage != checkOffline.Usage {
		t.Fatalf("offline flag usage differs: root=%#v check=%#v", rootOffline, checkOffline)
	}
}

func TestValidateUsesSkipArtifactsAndHidesLegacyOfflineAlias(t *testing.T) {
	root := newRootCommand()
	validate, _, err := root.Find([]string{"validate"})
	if err != nil {
		t.Fatal(err)
	}
	if validate.Flags().Lookup("skip-artifacts") == nil {
		t.Fatal("validate omitted --skip-artifacts")
	}
	legacy := validate.Flags().Lookup("offline")
	if legacy == nil || !legacy.Hidden || legacy.Deprecated == "" {
		t.Fatalf("legacy --offline compatibility flag = %#v, want hidden and deprecated", legacy)
	}
	if err := validate.Flags().Parse([]string{"--offline"}); err != nil {
		t.Fatalf("legacy --offline no longer parses: %v", err)
	}
	value, err := validate.Flags().GetBool("offline")
	if err != nil || !value {
		t.Fatalf("legacy --offline value = %t, %v; want true", value, err)
	}
}
