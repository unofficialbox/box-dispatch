package engine

import (
	"path/filepath"
	"testing"
)

// TestImportExampleDirectory checks the README's directory-import claim: pointing
// import at examples/bcl/clm aggregates every *artifacts.bcl file into one CLM
// runtime config with both providers, regardless of each file's syntax.
func TestImportExampleDirectory(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "bcl", "clm")
	cfg, report, err := (&Engine{}).importFromBCLDirectory(dir, "")
	if err != nil {
		t.Fatalf("importFromBCLDirectory(%s): %v", dir, err)
	}
	if cfg.ActiveScenario != "clm" {
		t.Fatalf("active scenario = %q, want clm", cfg.ActiveScenario)
	}
	for _, provider := range []string{"box", "salesforce-agentforce"} {
		if _, ok := cfg.Providers[provider]; !ok {
			t.Fatalf("imported config missing provider %q: %+v", provider, report.providers)
		}
	}
	if got := cfg.Providers["box"].Env["BOX_FOLDER_ID"]; got != "123456789012" {
		t.Fatalf("BOX_FOLDER_ID = %q, want 123456789012", got)
	}
	if got := cfg.Providers["salesforce-agentforce"].Env["SF_ORG_ID"]; got != "00DXX0000004C2xMAE" {
		t.Fatalf("SF_ORG_ID = %q, want 00DXX0000004C2xMAE", got)
	}
}
