package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPruneUnselectedProviderTrees(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"config/box/spec.json",
		"config/salesforce/spec.json",
		"config/agentforce/spec.json",
		"config/agentcore/spec.json",
		"databricks/bundle.yml",
		"demo-salesforce-project/sfdx-project.json",
	}
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := pruneUnselected(root, []string{"box"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "box", "spec.json")); err != nil {
		t.Fatal("mandatory Box assets were removed")
	}
	for _, path := range []string{"config/salesforce", "config/agentforce", "config/agentcore", "databricks", "demo-salesforce-project"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("unselected provider path remains: %s", path)
		}
	}
}

func TestBuildClonesDetachesAndWritesManifest(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "config", "box"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config", "box", "spec.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "dispatch@example.com"}, {"config", "user.name", "Box Dispatch Test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = source
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	destination := filepath.Join(t.TempDir(), "package")
	// The template must have a bundled manifest: the fixture ships no
	// dispatch.json, so Build relies on WriteBundled to supply one.
	manifest, err := Build(PackageRequest{Repository: source, Destination: destination, TemplateID: "clm", Components: []string{"box"}})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TemplateID != "clm" {
		t.Fatalf("template = %q, want clm", manifest.TemplateID)
	}
	if manifest.Destination != destination {
		t.Fatalf("destination = %q, want %q", manifest.Destination, destination)
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); !os.IsNotExist(err) {
		t.Fatal("packaged workspace retained upstream .git directory")
	}
	if _, err := os.Stat(filepath.Join(destination, ".dispatch", "package.json")); err != nil {
		t.Fatal("package manifest was not written")
	}
}
