package lifecycle

import "testing"

func TestOrderResourcesForTeardownDeletesLeavesBeforeContainers(t *testing.T) {
	resources := []ResourceReference{
		{Kind: "folder", Name: "workspace"},
		{Kind: "file", Name: "sample.docx"},
		{Kind: "hub", Name: "hub"},
		{Kind: "form", Name: "intake form"},
		{Kind: "metadata_template", Name: "contract"},
		{Kind: "ai_agent", Name: "agent"},
	}
	ordered := orderResourcesForTeardown(resources)
	got := make([]string, 0, len(ordered))
	for _, resource := range ordered {
		got = append(got, resource.Kind)
	}
	// The recursive folder delete must come last so it cannot remove a child
	// that a later step still expects to delete.
	if got[len(got)-1] != "folder" {
		t.Fatalf("folder must be deleted last, got order %v", got)
	}
	// Private browser surfaces go first.
	if got[0] != "form" {
		t.Fatalf("private surfaces must be removed first, got order %v", got)
	}
	if len(got) != len(resources) {
		t.Fatalf("ordering dropped resources: %v", got)
	}
}

func TestDeleteBoxResourceRefusesRootFolder(t *testing.T) {
	// The guard returns before any API call, so a zero context is safe here.
	outcome := deleteBoxResource(boxContext{}, ResourceReference{Kind: "folder", ID: "0", Name: "All Files"})
	if outcome.Deleted {
		t.Fatal("the Box root folder must never be deleted")
	}
	if outcome.Error == "" {
		t.Fatal("refusing the root folder should report a reason")
	}
}

func TestDeleteBoxResourceRequiresRecordedID(t *testing.T) {
	outcome := deleteBoxResource(boxContext{}, ResourceReference{Kind: "file", ID: "", Name: "orphan"})
	if outcome.Deleted || outcome.Error == "" {
		t.Fatalf("a resource with no recorded id cannot be deleted: %+v", outcome)
	}
}

func TestDeleteBoxResourceReportsUnmanagedKinds(t *testing.T) {
	// Automate workflows have no delete API; they must be reported, not attempted.
	outcome := deleteBoxResource(boxContext{}, ResourceReference{Kind: "automate_workflow", ID: "123", Name: "renewal"})
	if outcome.Deleted {
		t.Fatal("an unmanaged kind must not report as deleted")
	}
	if !outcome.Unmanaged {
		t.Fatalf("automate_workflow should be reported unmanaged: %+v", outcome)
	}
}

func TestTeardownResultCounts(t *testing.T) {
	result := TeardownResult{Outcomes: []TeardownOutcome{
		{Deleted: true},
		{Error: "boom"},
		{Unmanaged: true},
	}}
	if result.Deleted() != 1 {
		t.Fatalf("Deleted() = %d, want 1", result.Deleted())
	}
	if len(result.Remaining()) != 2 {
		t.Fatalf("Remaining() = %d, want 2", len(result.Remaining()))
	}
}

func TestDestroyProviderIgnoresOtherProvidersResources(t *testing.T) {
	// Only the requested provider's resources are ever considered.
	result, err := DestroyProvider(t.TempDir(), "databricks", []ResourceReference{
		{Provider: "box", Kind: "folder", ID: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outcomes) != 0 {
		t.Fatalf("no databricks resources were recorded, got %+v", result.Outcomes)
	}
}

func TestDestroyProviderReportsUnsupportedProvider(t *testing.T) {
	result, err := DestroyProvider(t.TempDir(), "databricks", []ResourceReference{
		{Provider: "databricks", Kind: "cluster", ID: "abc", Name: "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outcomes) != 1 || !result.Outcomes[0].Unmanaged {
		t.Fatalf("unsupported provider resources must be reported unmanaged: %+v", result.Outcomes)
	}
	if result.Deleted() != 0 {
		t.Fatal("nothing may be deleted for an unsupported provider")
	}
}

func TestPrivateDestroyNeverCountsAbsentAsDeleted(t *testing.T) {
	// An unauthenticated Box session makes every private surface look "absent"
	// (the app tier answers 200 with an empty list instead of 401). Reporting
	// that as deleted would claim a reset that removed nothing.
	var absent TeardownOutcome
	applyPrivateDestroyOutcome(&absent, "absent", "Box Form")
	if absent.Deleted {
		t.Fatal("an absent surface must never be reported as deleted")
	}
	if absent.Error == "" {
		t.Fatal("an absent surface must explain that nothing was removed")
	}

	var deleted TeardownOutcome
	applyPrivateDestroyOutcome(&deleted, "deleted", "Box Form")
	if !deleted.Deleted || deleted.Error != "" {
		t.Fatalf("a confirmed delete should be recorded: %+v", deleted)
	}

	var failed TeardownOutcome
	applyPrivateDestroyOutcome(&failed, "Box App delete failed: boom", "Box App")
	if failed.Deleted || failed.Error == "" {
		t.Fatalf("a failure must not be recorded as deleted: %+v", failed)
	}

	var empty TeardownOutcome
	applyPrivateDestroyOutcome(&empty, "", "Box App")
	if empty.Deleted || empty.Error == "" {
		t.Fatalf("an empty outcome must not be recorded as deleted: %+v", empty)
	}
}

func TestBrowserIsOnlyNeededForPrivateSurfaces(t *testing.T) {
	// A Box deploy of public-API components must never warm a browser.
	public := []ResourceReference{
		{Kind: "folder", ID: "1"}, {Kind: "file", ID: "2"},
		{Kind: "metadata_template", ID: "k"}, {Kind: "hub", ID: "3"},
		{Kind: "ai_agent", ID: "4"}, {Kind: "docgen_template", ID: "5"},
	}
	if boxPrivateResourcesPresent(public) {
		t.Fatal("public-API resources must not require the browser")
	}
	for _, kind := range []string{"form", "app"} {
		if !boxPrivateResourcesPresent([]ResourceReference{{Kind: kind, ID: "1"}}) {
			t.Fatalf("%s must require the browser", kind)
		}
	}
	if boxPrivateResourcesPresent(nil) {
		t.Fatal("no resources must not require the browser")
	}
}
