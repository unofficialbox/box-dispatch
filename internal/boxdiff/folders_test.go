package boxdiff

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

func TestCompareFolderSnapshotsMatchesRelativePathsAndCounts(t *testing.T) {
	started := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	left := FolderSnapshot{
		Environment: Environment{EnterpriseID: "left"},
		Folders: []Folder{
			{Path: "/Archive", Name: "Archive", Depth: 1},
			{Path: "/Contracts", Name: "Contracts", Depth: 1, Items: FolderItemCounts{Files: 2, Folders: 1}},
		},
	}
	right := FolderSnapshot{
		Environment: Environment{EnterpriseID: "right"},
		Folders: []Folder{
			{Path: "/Contracts", Name: "Contracts", Depth: 1, Items: FolderItemCounts{Files: 3, Folders: 1}},
			{Path: "/New", Name: "New", Depth: 1},
		},
		Truncated: true,
	}

	comparison, err := CompareFolderSnapshots(started, left, right, started.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Summary != (Summary{Changed: 1, LeftOnly: 1, RightOnly: 1}) {
		t.Fatalf("summary = %+v", comparison.Summary)
	}
	if comparison.Folders[1].Path != "/Contracts" || comparison.Folders[1].Status != "changed" || len(comparison.Folders[1].Changes) != 1 || comparison.Folders[1].Changes[0].Path != "items.files" {
		t.Fatalf("contracts diff = %+v", comparison.Folders[1])
	}
	if !comparison.Scope.RightTruncated || comparison.Scope.RootLabel != "All Files" {
		t.Fatalf("scope = %+v", comparison.Scope)
	}
}

func TestCompareFolderSnapshotsRejectsSameEnterprise(t *testing.T) {
	_, err := CompareFolderSnapshots(time.Now(), FolderSnapshot{Environment: Environment{EnterpriseID: "same"}}, FolderSnapshot{Environment: Environment{EnterpriseID: "same"}}, time.Now())
	if err == nil {
		t.Fatal("expected same-enterprise comparison to fail")
	}
}

func TestCrawlFolderTreeHonorsDepthAndFolderLimits(t *testing.T) {
	nameA, nameB, nameA1 := "A", "B", "A1"
	tree := map[string][]schemas.Item{
		"0": {{Folder: &schemas.FolderMini{Id: "a", Name: &nameA}}, {Folder: &schemas.FolderMini{Id: "b", Name: &nameB}}},
		"a": {{Folder: &schemas.FolderMini{Id: "a1", Name: &nameA1}}},
		"b": {},
	}
	list := func(_ context.Context, id string) iter.Seq2[*schemas.Item, error] {
		return func(yield func(*schemas.Item, error) bool) {
			for index := range tree[id] {
				if !yield(&tree[id][index], nil) {
					return
				}
			}
		}
	}

	folders, truncated, err := crawlFolderTree(context.Background(), list, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 || !truncated {
		t.Fatalf("depth-limited folders = %+v truncated = %v", folders, truncated)
	}
	folders, truncated, err = crawlFolderTree(context.Background(), list, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Path != "/A" || !truncated {
		t.Fatalf("folder-limited folders = %+v truncated = %v", folders, truncated)
	}
}

func TestCrawlFolderTreeReturnsListingError(t *testing.T) {
	list := func(_ context.Context, _ string) iter.Seq2[*schemas.Item, error] {
		return func(yield func(*schemas.Item, error) bool) { yield(nil, errors.New("unavailable")) }
	}
	if _, _, err := crawlFolderTree(context.Background(), list, 1, 1); err == nil {
		t.Fatal("expected listing error")
	}
}
