package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

func TestWriteArtifactBundle(t *testing.T) {
	tmp := t.TempDir()
	artifacts := []config.DeployedArtifact{
		{
			Provider:         "box",
			Scenario:         "acme",
			ArtifactType:     "box.folder",
			ProviderObjectID: "folder-1",
			ArtifactName:     "Folder",
			Source:           "environment",
			CreatedAt:        "2026-07-23T12:00:00Z",
		},
	}

	paths, err := writeArtifactBundle(tmp, "box", "acme", "resolve", "2026-07-23T12:00:00Z", artifacts)
	if err != nil {
		t.Fatalf("writeArtifactBundle error: %v", err)
	}

	for _, path := range []string{paths.BCL} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output file %q to exist: %v", path, err)
		}
	}

	expectedBCL := filepath.Join(tmp, "box-artifacts.bcl")
	if paths.BCL != expectedBCL {
		t.Fatalf("unexpected output paths: %+v", paths)
	}
}
