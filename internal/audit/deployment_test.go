package audit

import (
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
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
