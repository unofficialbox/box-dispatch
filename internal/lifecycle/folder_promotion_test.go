package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
)

type folderPromotionBoxAPI struct {
	boxAPI
	folders   map[string]string
	files     map[string]string
	webLinks  map[string]struct{ id, target string }
	deletedID string
}

func (api *folderPromotionBoxAPI) findFolder(_ context.Context, parentID, name string) (string, bool, error) {
	id, found := api.folders[parentID+"/"+name]
	return id, found, nil
}

func (api *folderPromotionBoxAPI) ensureFolder(_ context.Context, parentID, name string) (string, error) {
	key := parentID + "/" + name
	if id := api.folders[key]; id != "" {
		return id, nil
	}
	id := "folder-" + name
	api.folders[key] = id
	return id, nil
}

func (api *folderPromotionBoxAPI) findFile(_ context.Context, parentID, name string) (string, bool, error) {
	id, found := api.files[parentID+"/"+name]
	return id, found, nil
}

func (api *folderPromotionBoxAPI) uploadFile(_ context.Context, parentID, source string) (string, error) {
	name := filepath.Base(source)
	id := "file-" + name
	api.files[parentID+"/"+name] = id
	return id, nil
}

func (api *folderPromotionBoxAPI) findWebLink(_ context.Context, parentID, name string) (string, string, bool, error) {
	link, found := api.webLinks[parentID+"/"+name]
	return link.id, link.target, found, nil
}

func (api *folderPromotionBoxAPI) createWebLink(_ context.Context, parentID, name, target string) (string, error) {
	id := "link-" + name
	api.webLinks[parentID+"/"+name] = struct{ id, target string }{id: id, target: target}
	return id, nil
}

func (api *folderPromotionBoxAPI) deleteWebLink(_ context.Context, id string) error {
	api.deletedID = id
	return nil
}

func writeFolderPromotionPackage(t *testing.T, items []any) string {
	t.Helper()
	root := t.TempDir()
	doc := bcl.NewDocument("folder-promotion", "generic", "test", "2026-11-12T00:00:00Z", nil)
	doc.Resources = []bcl.BCLResource{{
		Name: "config_box_folder-promotion", Type: "config_file", Provider: "generic",
		Config: map[string]any{
			"artifact_name": "folder-promotion", "artifact_type": "config", "provider": "generic",
			"provider_object_id": "box/folder-promotion", "enterprise_id": "source-eid",
			"destination_root_id": "destination-root", "items": items,
		},
	}}
	if err := bcl.WriteBCL(filepath.Join(root, "config", "box", "folder-promotion.bcl"), doc); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDeployFolderPromotionRecordsOnlyCreatedResourceIDs(t *testing.T) {
	root := writeFolderPromotionPackage(t, []any{
		map[string]any{"path": "/Legal", "type": "folder"},
		map[string]any{"path": "/Legal/reference.txt", "type": "file", "source": "content/Legal/reference.txt"},
		map[string]any{"path": "/Legal/handbook", "type": "web_link", "url": "https://example.test/handbook"},
	})
	content := filepath.Join(root, "content", "Legal", "reference.txt")
	if err := os.MkdirAll(filepath.Dir(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(content, []byte("portable content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &folderPromotionBoxAPI{folders: map[string]string{}, files: map[string]string{}, webLinks: map[string]struct{ id, target string }{}}
	components := []string{"Folder:/Legal", "File:/Legal/reference.txt", "Web Link:/Legal/handbook"}

	deployed, resources, err := deployFolderPromotion(context.Background(), root, api, components, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(deployed, components) {
		t.Fatalf("deployed = %v, want %v", deployed, components)
	}
	got := make([]string, 0, len(resources))
	for _, resource := range resources {
		got = append(got, resource.Kind+":"+resource.ID)
	}
	want := []string{"folder:folder-Legal", "file:file-reference.txt", "web_link:link-handbook"}
	if !slices.Equal(got, want) {
		t.Fatalf("recorded resources = %v, want stable created IDs %v", got, want)
	}
}

func TestFolderPromotionRejectsUnsafePackageData(t *testing.T) {
	tests := []struct {
		name  string
		items []any
	}{
		{name: "traversal path", items: []any{map[string]any{"path": "/Legal/../../Outside", "type": "folder"}}},
		{name: "traversal source", items: []any{map[string]any{"path": "/Legal/file.txt", "type": "file", "source": "../secret.txt"}}},
		{name: "non-http link", items: []any{map[string]any{"path": "/Legal/link", "type": "web_link", "url": "javascript:alert(1)"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readFolderPromotion(writeFolderPromotionPackage(t, test.items)); err == nil {
				t.Fatal("unsafe folder promotion package was accepted")
			}
		})
	}
}

func TestAppendFolderPromotionChangeUsesSafeScopeOnly(t *testing.T) {
	item := Item{}
	if err := appendFolderPromotionChange(&item, folderPromotionItem{Path: "/Legal/reference.txt", Type: "file", Source: "content/private", URL: "https://secret.example.test"}); err != nil {
		t.Fatal(err)
	}
	if len(item.Changes) != 1 || item.Changes[0].After != "{\n  \"path\": \"/Legal/reference.txt\",\n  \"type\": \"file\"\n}" {
		t.Fatalf("scope preview leaked or changed shape: %+v", item.Changes)
	}
}

func TestDeleteBoxWebLinkUsesRecordedID(t *testing.T) {
	api := &folderPromotionBoxAPI{}
	outcome := deleteBoxResource(boxContext{api: api, ctx: context.Background()}, ResourceReference{Kind: "web_link", ID: "link-123", Name: "handbook"})
	if !outcome.Deleted || api.deletedID != "link-123" {
		t.Fatalf("web link teardown must delete only the recorded ID: outcome=%+v deleted=%q", outcome, api.deletedID)
	}
}
