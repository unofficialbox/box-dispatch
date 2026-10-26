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
)

type FolderItemCounts struct {
	Folders  int `json:"folders"`
	Files    int `json:"files"`
	WebLinks int `json:"webLinks"`
	Other    int `json:"other"`
}

// Folder is a provider-ID-free view of a folder beneath the connected user's
// All Files root. Path is the stable comparison key across enterprises.
type Folder struct {
	Path  string           `json:"path"`
	Name  string           `json:"name"`
	Depth int              `json:"depth"`
	Items FolderItemCounts `json:"items"`
}

type FolderSnapshot struct {
	Environment Environment `json:"environment"`
	Folders     []Folder    `json:"folders"`
	Truncated   bool        `json:"truncated"`
}

type FolderScope struct {
	RootLabel      string `json:"rootLabel"`
	MaxDepth       int    `json:"maxDepth"`
	MaxFolders     int    `json:"maxFolders"`
	LeftTruncated  bool   `json:"leftTruncated"`
	RightTruncated bool   `json:"rightTruncated"`
}

type FolderDiff struct {
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Changes []Change `json:"changes"`
	Left    *Folder  `json:"left,omitempty"`
	Right   *Folder  `json:"right,omitempty"`
}

type FolderComparison struct {
	StartedAt   string       `json:"startedAt"`
	CompletedAt string       `json:"completedAt"`
	Left        Environment  `json:"left"`
	Right       Environment  `json:"right"`
	Scope       FolderScope  `json:"scope"`
	Summary     Summary      `json:"summary"`
	Folders     []FolderDiff `json:"folders"`
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
		Scope: FolderScope{RootLabel: "All Files", MaxDepth: FolderComparisonMaxDepth, MaxFolders: FolderComparisonMaxFolders, LeftTruncated: left.Truncated, RightTruncated: right.Truncated},
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

func folderPointer(folder Folder) *Folder { return &folder }
