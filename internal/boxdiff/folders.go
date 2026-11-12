package boxdiff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	FolderComparisonMaxDepth   = 4
	FolderComparisonMaxFolders = 500
	FolderComparisonMaxEntries = 1000
	FolderPickerMaxFolders     = 500
	FolderSearchMaxResults     = 50
)

type FolderChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FolderChildren struct {
	ParentID string         `json:"parentId"`
	Folders  []FolderChoice `json:"folders"`
	Partial  bool           `json:"partial"`
}

type FolderSearchResults struct {
	Folders []FolderChoice `json:"folders"`
	Partial bool           `json:"partial"`
}

type FolderItemCounts struct {
	Folders  int `json:"folders"`
	Files    int `json:"files"`
	WebLinks int `json:"webLinks"`
	Other    int `json:"other"`
}

// Folder is a provider-ID-free view of a folder beneath the connected user's
// All Files root. Path is the stable comparison key across enterprises.
type Folder struct {
	ID    string           `json:"-"`
	Path  string           `json:"path"`
	Name  string           `json:"name"`
	Depth int              `json:"depth"`
	Items FolderItemCounts `json:"items"`
}

// FolderEntry is a provider-ID-free file or web-link node beneath the connected
// user's All Files root. File contents, hashes, versions, URLs, and Box IDs are
// deliberately excluded from structural comparison.
type FolderEntry struct {
	ID   string `json:"-"`
	Path string `json:"path"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// FolderPromotionSelection is the provider-neutral node identity the browser
// sends back when preparing a deployment. Provider IDs never cross the browser
// boundary; the server resolves them again from a fresh live comparison.
type FolderPromotionSelection struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// FolderPromotionItem is the complete server-side payload needed to assemble
// a portable folder package. Content and URLs are deliberately omitted from
// comparison responses.
type FolderPromotionItem struct {
	Path    string
	Type    string
	Content []byte
	URL     string
}

type FolderPromotion struct {
	Comparison FolderComparison
	Items      []FolderPromotionItem
}

type FolderSnapshot struct {
	Environment      Environment   `json:"environment"`
	RootLabel        string        `json:"rootLabel"`
	Folders          []Folder      `json:"folders"`
	Entries          []FolderEntry `json:"entries"`
	Truncated        bool          `json:"truncated"`
	EntriesTruncated bool          `json:"entriesTruncated"`
}

type FolderScope struct {
	RootLabel             string `json:"rootLabel"`
	LeftRootLabel         string `json:"leftRootLabel"`
	RightRootLabel        string `json:"rightRootLabel"`
	MaxDepth              int    `json:"maxDepth"`
	MaxFolders            int    `json:"maxFolders"`
	MaxEntries            int    `json:"maxEntries"`
	LeftTruncated         bool   `json:"leftTruncated"`
	RightTruncated        bool   `json:"rightTruncated"`
	LeftEntriesTruncated  bool   `json:"leftEntriesTruncated"`
	RightEntriesTruncated bool   `json:"rightEntriesTruncated"`
}

type FolderDiff struct {
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Changes []Change `json:"changes"`
	Left    *Folder  `json:"left,omitempty"`
	Right   *Folder  `json:"right,omitempty"`
}

type FolderEntryDiff struct {
	Path    string       `json:"path"`
	Name    string       `json:"name"`
	Type    string       `json:"type"`
	Status  string       `json:"status"`
	Changes []Change     `json:"changes"`
	Left    *FolderEntry `json:"left,omitempty"`
	Right   *FolderEntry `json:"right,omitempty"`
}

type FolderComparison struct {
	StartedAt   string            `json:"startedAt"`
	CompletedAt string            `json:"completedAt"`
	Left        Environment       `json:"left"`
	Right       Environment       `json:"right"`
	Scope       FolderScope       `json:"scope"`
	Summary     Summary           `json:"summary"`
	Folders     []FolderDiff      `json:"folders"`
	Entries     []FolderEntryDiff `json:"entries"`
}

func CompareFolderSnapshots(startedAt time.Time, left, right FolderSnapshot, completedAt time.Time) (FolderComparison, error) {
	if strings.TrimSpace(left.Environment.EnterpriseID) == "" || strings.TrimSpace(right.Environment.EnterpriseID) == "" {
		return FolderComparison{}, fmt.Errorf("both live Box connections must report an enterprise ID")
	}
	if left.Environment.EnterpriseID == right.Environment.EnterpriseID {
		return FolderComparison{}, fmt.Errorf("select Box connections from two different enterprises")
	}
	comparison := FolderComparison{
		StartedAt: startedAt.UTC().Format(time.RFC3339Nano), CompletedAt: completedAt.UTC().Format(time.RFC3339Nano),
		Left: left.Environment, Right: right.Environment,
		Folders: []FolderDiff{}, Entries: []FolderEntryDiff{},
		Scope: FolderScope{
			RootLabel: firstNonEmpty(left.RootLabel, "All Files"), LeftRootLabel: firstNonEmpty(left.RootLabel, "All Files"), RightRootLabel: firstNonEmpty(right.RootLabel, "All Files"),
			MaxDepth: FolderComparisonMaxDepth, MaxFolders: FolderComparisonMaxFolders, MaxEntries: FolderComparisonMaxEntries,
			LeftTruncated: left.Truncated, RightTruncated: right.Truncated,
			LeftEntriesTruncated: left.EntriesTruncated, RightEntriesTruncated: right.EntriesTruncated,
		},
	}
	leftByPath := foldersByPath(left.Folders)
	rightByPath := foldersByPath(right.Folders)
	paths := make([]string, 0, len(leftByPath)+len(rightByPath))
	seen := map[string]bool{}
	for path := range leftByPath {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range rightByPath {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		leftFolder, onLeft := leftByPath[path]
		rightFolder, onRight := rightByPath[path]
		diff := FolderDiff{Path: path, Changes: []Change{}}
		switch {
		case !onRight:
			diff.Name = leftFolder.Name
			diff.Status = "left_only"
			diff.Left = folderPointer(leftFolder)
			comparison.Summary.LeftOnly++
		case !onLeft:
			diff.Name = rightFolder.Name
			diff.Status = "right_only"
			diff.Right = folderPointer(rightFolder)
			comparison.Summary.RightOnly++
		default:
			diff.Name = firstNonEmpty(leftFolder.Name, rightFolder.Name)
			diff.Left = folderPointer(leftFolder)
			diff.Right = folderPointer(rightFolder)
			diff.Changes = compareFolder(leftFolder, rightFolder)
			if len(diff.Changes) == 0 {
				diff.Status = "equal"
				comparison.Summary.Equal++
			} else {
				diff.Status = "changed"
				comparison.Summary.Changed++
			}
		}
		comparison.Folders = append(comparison.Folders, diff)
	}
	leftEntriesByPath := folderEntriesByPath(left.Entries)
	rightEntriesByPath := folderEntriesByPath(right.Entries)
	entryPaths := make([]string, 0, len(leftEntriesByPath)+len(rightEntriesByPath))
	seen = map[string]bool{}
	for path := range leftEntriesByPath {
		entryPaths = append(entryPaths, path)
		seen[path] = true
	}
	for path := range rightEntriesByPath {
		if !seen[path] {
			entryPaths = append(entryPaths, path)
		}
	}
	sort.Strings(entryPaths)
	for _, path := range entryPaths {
		leftEntry, onLeft := leftEntriesByPath[path]
		rightEntry, onRight := rightEntriesByPath[path]
		diff := FolderEntryDiff{Path: path, Changes: []Change{}}
		switch {
		case !onRight:
			diff.Name, diff.Type, diff.Status, diff.Left = leftEntry.Name, leftEntry.Type, "left_only", folderEntryPointer(leftEntry)
			comparison.Summary.LeftOnly++
		case !onLeft:
			diff.Name, diff.Type, diff.Status, diff.Right = rightEntry.Name, rightEntry.Type, "right_only", folderEntryPointer(rightEntry)
			comparison.Summary.RightOnly++
		default:
			diff.Name, diff.Type = firstNonEmpty(leftEntry.Name, rightEntry.Name), firstNonEmpty(leftEntry.Type, rightEntry.Type)
			diff.Left, diff.Right = folderEntryPointer(leftEntry), folderEntryPointer(rightEntry)
			if leftEntry.Type == rightEntry.Type {
				diff.Status = "equal"
				comparison.Summary.Equal++
			} else {
				diff.Status = "changed"
				diff.Changes = append(diff.Changes, Change{Path: "type", Before: leftEntry.Type, After: rightEntry.Type})
				comparison.Summary.Changed++
			}
		}
		comparison.Entries = append(comparison.Entries, diff)
	}
	return comparison, nil
}

func compareFolder(left, right Folder) []Change {
	leftValues := map[string]any{
		"items.folders":  left.Items.Folders,
		"items.files":    left.Items.Files,
		"items.webLinks": left.Items.WebLinks,
		"items.other":    left.Items.Other,
	}
	rightValues := map[string]any{
		"items.folders":  right.Items.Folders,
		"items.files":    right.Items.Files,
		"items.webLinks": right.Items.WebLinks,
		"items.other":    right.Items.Other,
	}
	paths := []string{"items.files", "items.folders", "items.other", "items.webLinks"}
	changes := make([]Change, 0, len(paths))
	for _, path := range paths {
		if reflect.DeepEqual(leftValues[path], rightValues[path]) {
			continue
		}
		changes = append(changes, Change{Path: path, Before: leftValues[path], After: rightValues[path]})
	}
	return changes
}

func foldersByPath(folders []Folder) map[string]Folder {
	result := make(map[string]Folder, len(folders))
	for _, folder := range folders {
		if folder.Path != "" {
			result[folder.Path] = folder
		}
	}
	return result
}

func folderEntriesByPath(entries []FolderEntry) map[string]FolderEntry {
	result := make(map[string]FolderEntry, len(entries))
	for _, entry := range entries {
		if entry.Path != "" {
			result[entry.Path] = entry
		}
	}
	return result
}

func folderPointer(folder Folder) *Folder               { return &folder }
func folderEntryPointer(entry FolderEntry) *FolderEntry { return &entry }
