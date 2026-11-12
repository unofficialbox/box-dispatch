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
		Entries: []FolderEntry{
			{Path: "/Contracts/agreement.pdf", Name: "agreement.pdf", Type: "file"},
			{Path: "/Contracts/reference", Name: "reference", Type: "file"},
		},
	}
	right := FolderSnapshot{
		Environment: Environment{EnterpriseID: "right"},
		Folders: []Folder{
			{Path: "/Contracts", Name: "Contracts", Depth: 1, Items: FolderItemCounts{Files: 3, Folders: 1}},
			{Path: "/New", Name: "New", Depth: 1},
		},
		Entries: []FolderEntry{
			{Path: "/Contracts/agreement.pdf", Name: "agreement.pdf", Type: "file"},
			{Path: "/Contracts/reference", Name: "reference", Type: "web_link"},
			{Path: "/New/readme.txt", Name: "readme.txt", Type: "file"},
		},
		Truncated: true, EntriesTruncated: true,
	}

	comparison, err := CompareFolderSnapshots(started, left, right, started.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Summary != (Summary{Equal: 1, Changed: 2, LeftOnly: 1, RightOnly: 2}) {
		t.Fatalf("summary = %+v", comparison.Summary)
	}
	if comparison.Folders[1].Path != "/Contracts" || comparison.Folders[1].Status != "changed" || len(comparison.Folders[1].Changes) != 1 || comparison.Folders[1].Changes[0].Path != "items.files" {
		t.Fatalf("contracts diff = %+v", comparison.Folders[1])
	}
	if !comparison.Scope.RightTruncated || comparison.Scope.RootLabel != "All Files" {
		t.Fatalf("scope = %+v", comparison.Scope)
	}
	if !comparison.Scope.RightEntriesTruncated || comparison.Scope.MaxEntries != FolderComparisonMaxEntries {
		t.Fatalf("entry scope = %+v", comparison.Scope)
	}
	if comparison.Entries[1].Path != "/Contracts/reference" || comparison.Entries[1].Status != "changed" || comparison.Entries[1].Changes[0].Path != "type" {
		t.Fatalf("reference diff = %+v", comparison.Entries[1])
	}
	if comparison.Entries[2].Path != "/New/readme.txt" || comparison.Entries[2].Status != "right_only" {
		t.Fatalf("readme diff = %+v", comparison.Entries[2])
	}
}

func TestCompareFolderSnapshotsRejectsSameEnterprise(t *testing.T) {
	_, err := CompareFolderSnapshots(time.Now(), FolderSnapshot{Environment: Environment{EnterpriseID: "same"}}, FolderSnapshot{Environment: Environment{EnterpriseID: "same"}}, time.Now())
	if err == nil {
		t.Fatal("expected same-enterprise comparison to fail")
	}
}

func TestCompareFolderSnapshotsReturnsEmptyCollections(t *testing.T) {
	started := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	comparison, err := CompareFolderSnapshots(
		started,
		FolderSnapshot{Environment: Environment{EnterpriseID: "left"}},
		FolderSnapshot{Environment: Environment{EnterpriseID: "right"}},
		started.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Folders == nil || comparison.Entries == nil {
		t.Fatalf("collections must be empty arrays: folders=%#v entries=%#v", comparison.Folders, comparison.Entries)
	}
}

func TestCrawlFolderTreeHonorsDepthAndFolderLimits(t *testing.T) {
	nameA, nameB, nameA1, fileName, linkName := "A", "B", "A1", "readme.txt", "Reference"
	tree := map[string][]schemas.Item{
		"0": {{Folder: &schemas.FolderMini{Id: "a", Name: &nameA}}, {Folder: &schemas.FolderMini{Id: "b", Name: &nameB}}, {File: &schemas.FileFull{Name: &fileName}}},
		"a": {{Folder: &schemas.FolderMini{Id: "a1", Name: &nameA1}}, {WebLink: &schemas.WebLink{Name: &linkName}}},
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

	folders, entries, truncated, entriesTruncated, err := crawlFolderTree(context.Background(), list, 1, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 || !truncated {
		t.Fatalf("depth-limited folders = %+v truncated = %v", folders, truncated)
	}
	if len(entries) != 2 || entriesTruncated || entries[0].Path != "/A/Reference" || entries[1].Path != "/readme.txt" {
		t.Fatalf("entries = %+v truncated = %v", entries, entriesTruncated)
	}
	folders, entries, truncated, entriesTruncated, err = crawlFolderTree(context.Background(), list, 4, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Path != "/A" || !truncated {
		t.Fatalf("folder-limited folders = %+v truncated = %v", folders, truncated)
	}
	if len(entries) != 1 || !entriesTruncated {
		t.Fatalf("entry-limited entries = %+v truncated = %v", entries, entriesTruncated)
	}
}

func TestCrawlFolderTreeReturnsListingError(t *testing.T) {
	list := func(_ context.Context, _ string) iter.Seq2[*schemas.Item, error] {
		return func(yield func(*schemas.Item, error) bool) { yield(nil, errors.New("unavailable")) }
	}
	if _, _, _, _, err := crawlFolderTree(context.Background(), list, 1, 1, 1); err == nil {
		t.Fatal("expected listing error")
	}
}
