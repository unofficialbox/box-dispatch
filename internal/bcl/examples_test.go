package bcl

import (
	"path/filepath"
	"testing"
)

// TestParseBCLLocalsTolerance pins the parser leniency the HCL examples rely on:
// a bare (unquoted) bcl key, newline-separated object entries with no commas,
// and a trailing comma before a closing bracket.
func TestParseBCLLocalsTolerance(t *testing.T) {
	src := `locals {
  bcl = {
    "version" = "1.0"
    "provider" = "box"
    "resources" = [
      {
        "type" = "box.folder"
        "provider" = "box"
        "config" = { "provider_object_id" = "42" }
      },
    ]
  }
}`
	inv, err := parseBCLLocals([]byte(src))
	if err != nil {
		t.Fatalf("parseBCLLocals: %v", err)
	}
	if inv["provider"] != "box" {
		t.Fatalf("provider = %v, want box", inv["provider"])
	}
	res, ok := inv["resources"].([]any)
	if !ok || len(res) != 1 {
		t.Fatalf("resources = %v, want one entry", inv["resources"])
	}
}

// TestExampleFilesImport guards the worked examples under examples/bcl/ so they
// keep parsing and extracting the artifacts their README advertises — one file
// per on-disk syntax (HCL locals and JSON).
func TestExampleFilesImport(t *testing.T) {
	cases := []struct {
		file             string
		wantResources    int
		wantArtifactType string
		wantObjectID     string
	}{
		{"box-artifacts.bcl", 2, "box.folder", "123456789012"},                  // HCL locals syntax
		{"salesforce-artifacts.bcl", 1, "salesforce.org", "00DXX0000004C2xMAE"}, // JSON syntax
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", "bcl", "clm", tc.file)
			doc, err := LoadBCL(path)
			if err != nil {
				t.Fatalf("LoadBCL(%s): %v", tc.file, err)
			}
			if len(doc.Resources) != tc.wantResources {
				t.Fatalf("%s: resources = %d, want %d", tc.file, len(doc.Resources), tc.wantResources)
			}
			arts := ExtractArtifactsFromBCL(doc)
			if len(arts) != tc.wantResources {
				t.Fatalf("%s: extracted %d artifacts, want %d", tc.file, len(arts), tc.wantResources)
			}
			var found bool
			for _, a := range arts {
				if a.ArtifactType == tc.wantArtifactType && a.ProviderObjectID == tc.wantObjectID {
					found = true
				}
				if a.ProviderObjectID == "" {
					t.Errorf("%s: artifact %q has no provider object ID", tc.file, a.ArtifactType)
				}
			}
			if !found {
				t.Fatalf("%s: no %s artifact with object ID %q in %+v", tc.file, tc.wantArtifactType, tc.wantObjectID, arts)
			}
		})
	}
}
