package salesforceapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type MetadataDiff struct {
	Matching []string
	Missing  []string
	Changed  []string
	Files    []MetadataFileDiff
}

type MetadataFileDiff struct {
	Component   string `json:"component"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
	Previewable bool   `json:"previewable"`
}

func CompareMetadataSource(project string, components []string, retrievedZip []byte) (MetadataDiff, error) {
	descriptors, _, err := readSourceDescriptors(project)
	if err != nil {
		return MetadataDiff{}, err
	}
	retrieved, err := readMetadataZip(retrievedZip)
	if err != nil {
		return MetadataDiff{}, err
	}
	childrenByObject := map[string]map[string]bool{}
	for _, descriptor := range descriptors {
		if descriptor.ObjectName == "" || descriptor.ChildTag == "" {
			continue
		}
		if childrenByObject[descriptor.ObjectName] == nil {
			childrenByObject[descriptor.ObjectName] = map[string]bool{}
		}
		childrenByObject[descriptor.ObjectName][descriptor.ChildTag] = true
	}

	result := MetadataDiff{}
	defaultRoot := sourceDefaultRoot(project)
	selected := append([]string(nil), components...)
	sort.Strings(selected)
	for _, component := range selected {
		descriptor, ok := descriptors[component]
		if !ok {
			return MetadataDiff{}, fmt.Errorf("Salesforce source component %s was not found in the package", component)
		}
		matching, missing, compareErr := compareMetadataComponent(defaultRoot, descriptor, childrenByObject[descriptor.ObjectName], retrieved)
		if compareErr != nil {
			return MetadataDiff{}, fmt.Errorf("compare Salesforce metadata %s: %w", component, compareErr)
		}
		switch {
		case missing:
			result.Missing = append(result.Missing, component)
			files, fileErr := metadataFileDiffs(defaultRoot, component, descriptor, retrieved, true)
			if fileErr != nil {
				return MetadataDiff{}, fmt.Errorf("prepare Salesforce metadata preview %s: %w", component, fileErr)
			}
			result.Files = append(result.Files, files...)
		case matching:
			result.Matching = append(result.Matching, component)
		default:
			result.Changed = append(result.Changed, component)
			files, fileErr := metadataFileDiffs(defaultRoot, component, descriptor, retrieved, false)
			if fileErr != nil {
				return MetadataDiff{}, fmt.Errorf("prepare Salesforce metadata preview %s: %w", component, fileErr)
			}
			result.Files = append(result.Files, files...)
		}
	}
	return result, nil
}

func compareMetadataComponent(defaultRoot string, descriptor sourceDescriptor, childTags map[string]bool, retrieved map[string][]byte) (matching, missing bool, err error) {
	if descriptor.ObjectName != "" {
		objectPath := "objects/" + descriptor.ObjectName + ".object"
		remote, ok := retrieved[objectPath]
		if !ok {
			return false, true, nil
		}
		local, readErr := os.ReadFile(descriptor.Path)
		if readErr != nil {
			return false, false, readErr
		}
		if descriptor.ChildTag != "" {
			localNode, parseErr := parseMetadataXML(local)
			if parseErr != nil {
				return false, false, parseErr
			}
			remoteNode, parseErr := parseMetadataXML(remote)
			if parseErr != nil {
				return false, false, parseErr
			}
			member := strings.TrimPrefix(descriptor.Member, descriptor.ObjectName+".")
			remoteChild := findMetadataChild(remoteNode, descriptor.ChildTag, member)
			if remoteChild == nil {
				return false, true, nil
			}
			return metadataChildrenMatch(localNode.Children, remoteChild.Children), false, nil
		}
		localNode, parseErr := parseMetadataXML(local)
		if parseErr != nil {
			return false, false, parseErr
		}
		remoteNode, parseErr := parseMetadataXML(remote)
		if parseErr != nil {
			return false, false, parseErr
		}
		localNode.Children = removeMetadataChildren(localNode.Children, childTags)
		remoteNode.Children = removeMetadataChildren(remoteNode.Children, childTags)
		return metadataNodeMatches(localNode, remoteNode), false, nil
	}

	if root := metadataBundleRoot(descriptor.Relative); root != "" {
		return compareMetadataBundle(defaultRoot, filepath.ToSlash(root), retrieved)
	}

	localPrimary := strings.TrimSuffix(descriptor.Path, "-meta.xml")
	if _, statErr := os.Stat(localPrimary); statErr == nil {
		primaryRelative := strings.TrimSuffix(descriptor.Relative, "-meta.xml")
		primary, ok := retrieved[primaryRelative]
		if !ok {
			return false, true, nil
		}
		local, readErr := os.ReadFile(localPrimary)
		if readErr != nil {
			return false, false, readErr
		}
		if !canonicalMetadataData(primaryRelative, local, primary) {
			return false, false, nil
		}
		remoteMeta, ok := retrieved[descriptor.Relative]
		if !ok {
			return false, true, nil
		}
		localMeta, readErr := os.ReadFile(descriptor.Path)
		if readErr != nil {
			return false, false, readErr
		}
		return canonicalMetadataData(descriptor.Relative, localMeta, remoteMeta), false, nil
	}

	target := strings.TrimSuffix(descriptor.Relative, "-meta.xml")
	remote, ok := retrieved[target]
	if !ok {
		return false, true, nil
	}
	local, readErr := os.ReadFile(descriptor.Path)
	if readErr != nil {
		return false, false, readErr
	}
	return canonicalMetadataData(target, local, remote), false, nil
}

func compareMetadataBundle(defaultRoot, root string, retrieved map[string][]byte) (matching, missing bool, err error) {
	localRoot := filepath.Join(defaultRoot, filepath.FromSlash(root))
	matched := true
	err = filepath.WalkDir(localRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(defaultRoot, path)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(relative)
		remote, ok := retrieved[key]
		if !ok {
			missing = true
			return nil
		}
		local, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !canonicalMetadataData(key, local, remote) {
			matched = false
		}
		return nil
	})
	if err != nil || missing {
		return false, missing, err
	}
	return matched, false, nil
}

func readMetadataZip(data []byte) (map[string][]byte, error) {
	if len(data) == 0 {
		return map[string][]byte{}, nil
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open Salesforce metadata retrieval package: %w", err)
	}
	result := map[string][]byte{}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		opened, openErr := file.Open()
		if openErr != nil {
			return nil, openErr
		}
		contents, readErr := io.ReadAll(opened)
		closeErr := opened.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		name := strings.TrimPrefix(filepath.ToSlash(file.Name), "unpackaged/")
		if name == "package.xml" {
			continue
		}
		result[name] = contents
	}
	return result, nil
}

type metadataXMLNode struct {
	Name     string
	Attrs    map[string]string
	Text     string
	Children []*metadataXMLNode
}

func parseMetadataXML(data []byte) (*metadataXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *metadataXMLNode
	stack := []*metadataXMLNode{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &metadataXMLNode{Name: value.Name.Local, Attrs: map[string]string{}}
			for _, attr := range value.Attr {
				if attr.Name.Local != "xmlns" {
					node.Attrs[attr.Name.Local] = strings.TrimSpace(attr.Value)
				}
			}
			if len(stack) == 0 {
				root = node
			} else {
				stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 && strings.TrimSpace(string(value)) != "" {
				stack[len(stack)-1].Text += strings.TrimSpace(string(value))
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("metadata XML has no root element")
	}
	return root, nil
}

func findMetadataChild(root *metadataXMLNode, tag, fullName string) *metadataXMLNode {
	for _, child := range root.Children {
		if child.Name != tag {
			continue
		}
		for _, value := range child.Children {
			if value.Name == "fullName" && value.Text == fullName {
				return child
			}
		}
	}
	return nil
}

func removeMetadataChildren(children []*metadataXMLNode, names map[string]bool) []*metadataXMLNode {
	result := make([]*metadataXMLNode, 0, len(children))
	for _, child := range children {
		if !names[child.Name] {
			result = append(result, child)
		}
	}
	return result
}

func canonicalMetadataData(path string, local, remote []byte) bool {
	if normalizeMetadataText(local) == normalizeMetadataText(remote) {
		return true
	}
	if bytes.HasPrefix(bytes.TrimSpace(local), []byte("<")) && bytes.HasPrefix(bytes.TrimSpace(remote), []byte("<")) {
		left, leftErr := parseMetadataXML(local)
		right, rightErr := parseMetadataXML(remote)
		if leftErr != nil || rightErr != nil {
			return false
		}
		return metadataNodeMatches(left, right)
	}
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".json":
		var left, right any
		if json.Unmarshal(local, &left) != nil || json.Unmarshal(remote, &right) != nil {
			return false
		}
		leftData, _ := json.Marshal(left)
		rightData, _ := json.Marshal(right)
		return bytes.Equal(leftData, rightData)
	default:
		return normalizeMetadataText(local) == normalizeMetadataText(remote)
	}
}

// metadataNodeMatches compares the package's declared metadata intent with the
// representation returned by Salesforce. Metadata retrieval is not a byte-for-byte
// round trip: Salesforce adds default values, inferred package dependencies, and
// other server-owned elements. Those additions do not require another deployment.
// Every value declared by the package must still be present and equal.
func metadataNodeMatches(local, remote *metadataXMLNode) bool {
	rootName := ""
	if local != nil {
		rootName = local.Name
	}
	return metadataNodeMatchesWithin(rootName, local, remote)
}

func metadataNodeMatchesWithin(rootName string, local, remote *metadataXMLNode) bool {
	if local == nil || remote == nil || local.Name != remote.Name || !metadataTextMatches(rootName, local.Name, local.Text, remote.Text) {
		return false
	}
	for key, value := range local.Attrs {
		if remote.Attrs[key] != value {
			return false
		}
	}
	return metadataChildrenMatchWithin(rootName, local.Children, remote.Children)
}

func metadataChildrenMatch(local, remote []*metadataXMLNode) bool {
	return metadataChildrenMatchWithin("", local, remote)
}

func metadataChildrenMatchWithin(rootName string, local, remote []*metadataXMLNode) bool {
	used := make([]bool, len(remote))
	localCounts := map[string]int{}
	nextUnkeyedIndex := map[string]int{}
	for _, child := range local {
		localCounts[child.Name]++
	}
	for _, child := range local {
		identityName, identityValue := metadataNodeIdentity(child)
		match := -1
		start := 0
		if identityName == "" && localCounts[child.Name] > 1 {
			start = nextUnkeyedIndex[child.Name]
		}
		for index := start; index < len(remote); index++ {
			candidate := remote[index]
			if used[index] || candidate.Name != child.Name {
				continue
			}
			if identityName != "" {
				candidateName, candidateValue := metadataNodeIdentity(candidate)
				if candidateName != identityName || candidateValue != identityValue {
					continue
				}
			}
			match = index
			break
		}
		if match < 0 || !metadataNodeMatchesWithin(rootName, child, remote[match]) {
			return false
		}
		used[match] = true
		if identityName == "" && localCounts[child.Name] > 1 {
			nextUnkeyedIndex[child.Name] = match + 1
		}
	}
	return true
}

func metadataTextMatches(rootName, nodeName, local, remote string) bool {
	local = strings.TrimSpace(local)
	remote = strings.TrimSpace(remote)
	if local == remote {
		return true
	}
	// Salesforce never returns an Auth Provider's actual secret through the
	// Metadata API. A non-empty package value can only be verified as present.
	if rootName == "AuthProvider" && nodeName == "consumerSecret" {
		return local != "" && remote == "Placeholder_Value"
	}
	// Scratch orgs make unverified Experience Cloud sender addresses inert by
	// appending .invalid. This is the same declared address, not configuration drift.
	if rootName == "Network" && (nodeName == "emailSenderAddress" || nodeName == "newSenderAddress") {
		return local != "" && remote == local+".invalid"
	}
	return false
}

func metadataNodeIdentity(node *metadataXMLNode) (string, string) {
	identityChild := map[string]string{
		"applicationVisibilities": "application",
		"classAccesses":           "apexClass",
		"fieldPermissions":        "field",
		"objectPermissions":       "object",
		"tabSettings":             "tab",
		"userPermissions":         "name",
	}[node.Name]
	if identityChild == "" {
		return "", ""
	}
	for _, child := range node.Children {
		if child.Name == identityChild {
			return identityChild, strings.TrimSpace(child.Text)
		}
	}
	return "", ""
}

func canonicalMetadataNode(node *metadataXMLNode) string {
	var output strings.Builder
	output.WriteString("<" + node.Name)
	keys := make([]string, 0, len(node.Attrs))
	for key := range node.Attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		output.WriteString(" " + key + "=" + node.Attrs[key])
	}
	output.WriteString(">")
	output.WriteString(strings.TrimSpace(node.Text))
	output.WriteString(canonicalMetadataChildren(node.Children))
	output.WriteString("</" + node.Name + ">")
	return output.String()
}

func canonicalMetadataChildren(children []*metadataXMLNode) string {
	var output strings.Builder
	for _, child := range children {
		output.WriteString(canonicalMetadataNode(child))
	}
	return output.String()
}

func normalizeMetadataText(data []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
