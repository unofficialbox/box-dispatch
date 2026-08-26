package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsMissingVariablesOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("DISPATCH_DOTENV_EXISTING=from-file\n# comment\nexport DISPATCH_DOTENV_MISSING=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISPATCH_DOTENV_EXISTING", "already-set")
	os.Unsetenv("DISPATCH_DOTENV_MISSING")
	t.Cleanup(func() { os.Unsetenv("DISPATCH_DOTENV_MISSING") })

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DISPATCH_DOTENV_EXISTING") != "already-set" {
		t.Fatalf("existing env was overwritten: %q", os.Getenv("DISPATCH_DOTENV_EXISTING"))
	}
	if os.Getenv("DISPATCH_DOTENV_MISSING") != "loaded" {
		t.Fatalf("missing env was not loaded: %q", os.Getenv("DISPATCH_DOTENV_MISSING"))
	}
}

func TestLoadDotEnvIgnoresMissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatal(err)
	}
}
