package audit

import (
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

func TestDeployedResourcesFlattensEveryProvider(t *testing.T) {
	record := DeploymentRecord{Providers: []ProviderRecord{
		{Provider: "box", Resources: []lifecycle.ResourceReference{
			{Provider: "box", Kind: "folder", ID: "1"},
			{Provider: "box", Kind: "hub", ID: "2"},
		}},
		{Provider: "salesforce", Resources: []lifecycle.ResourceReference{
			{Provider: "salesforce", Kind: "metadata_deployment", ID: "3"},
		}},
		{Provider: "databricks"},
	}}
	resources := record.DeployedResources()
	if len(resources) != 3 {
		t.Fatalf("got %d resources, want 3: %+v", len(resources), resources)
	}
	kinds := map[string]bool{}
	for _, resource := range resources {
		kinds[resource.Kind] = true
	}
	for _, want := range []string{"folder", "hub", "metadata_deployment"} {
		if !kinds[want] {
			t.Fatalf("missing %q in flattened resources: %+v", want, resources)
		}
	}
}

func TestDeployedResourcesIsEmptyWithoutRecordedResources(t *testing.T) {
	record := DeploymentRecord{Providers: []ProviderRecord{{Provider: "box"}}}
	if resources := record.DeployedResources(); len(resources) != 0 {
		t.Fatalf("expected no resources, got %+v", resources)
	}
}

func TestProviderRecordsCaptureDeploymentEnvironment(t *testing.T) {
	records := providerRecords(nil, []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusPresent}}, map[string]string{"box": " 5105484 "})
	if len(records) != 1 || records[0].EnvironmentID != "5105484" {
		t.Fatalf("provider records = %#v", records)
	}
}

func TestProviderRecordsCaptureValidatedFileChanges(t *testing.T) {
	validated := salesforceapi.MetadataFileDiff{Component: "Settings:Communities", Path: "settings/Communities.settings-meta.xml", Kind: "update", Before: "false", After: "true", Previewable: true}
	before := []lifecycle.Item{{Provider: "salesforce", Changes: []salesforceapi.MetadataFileDiff{validated}}}
	after := []lifecycle.Item{{Provider: "salesforce", Status: lifecycle.StatusPresent}}

	records := providerRecords(before, after, nil)
	if len(records) != 1 || len(records[0].Changes) != 1 || records[0].Changes[0] != validated {
		t.Fatalf("provider records = %#v", records)
	}
	record := DeploymentRecord{Providers: records}
	if changes := record.FileChanges(); len(changes) != 1 || changes[0] != validated {
		t.Fatalf("file changes = %#v", changes)
	}
}
