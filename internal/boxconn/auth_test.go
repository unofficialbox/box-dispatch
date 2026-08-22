package boxconn

import (
	"os"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

func TestResolveAuthUsesSelectedCCG(t *testing.T) {
	isolateConnectionSettings(t, config.ConnectionSettings{
		BoxCCGClientID: "id", BoxCCGClientSecret: "secret",
		BoxCCGSubjectType: "user", BoxCCGSubjectID: "123",
		BoxDefaultConnection: DispatchCCGName,
	})
	t.Setenv("BOX_CLIENT_ID", "")
	t.Setenv("BOX_CLIENT_SECRET", "")
	t.Setenv("BOX_REFRESH_TOKEN", "")

	got, err := ResolveAuth()
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != AuthCCG || got.Selection() != DispatchCCGName || got.SubjectID != "123" {
		t.Fatalf("resolved CCG = %+v", got)
	}
}

func TestResolveAuthUsesOAuth2Environment(t *testing.T) {
	isolateConnectionSettings(t, config.ConnectionSettings{})
	t.Setenv("BOX_CLIENT_ID", "client-id")
	t.Setenv("BOX_CLIENT_SECRET", "client-secret")
	t.Setenv("BOX_REFRESH_TOKEN", "refresh-token")

	got, err := ResolveAuth()
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != AuthOAuth2 || got.Selection() != "oauth2:client-id" {
		t.Fatalf("resolved OAuth2 = %+v", got)
	}
}

func TestResolveAuthRejectsLegacyCLISelection(t *testing.T) {
	isolateConnectionSettings(t, config.ConnectionSettings{BoxDefaultConnection: "old-box-cli"})
	t.Setenv("BOX_CLIENT_ID", "")
	t.Setenv("BOX_CLIENT_SECRET", "")
	t.Setenv("BOX_REFRESH_TOKEN", "")

	if _, err := ResolveAuth(); err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("ResolveAuth error = %v, want legacy-CLI migration guidance", err)
	}
}

func isolateConnectionSettings(t *testing.T, settings config.ConnectionSettings) {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := shellstate.SaveConnectionSettings(settings); err != nil {
		t.Fatal(err)
	}
}
