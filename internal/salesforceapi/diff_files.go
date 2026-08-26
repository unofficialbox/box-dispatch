package salesforceapi

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const metadataDiffPreviewLimit = 512 * 1024

func metadataFileDiffs(defaultRoot, component string, descriptor sourceDescriptor, retrieved map[string][]byte, componentMissing bool) ([]MetadataFileDiff, error) {
	if descriptor.ObjectName != "" {
		local, err := os.ReadFile(descriptor.Path)
		if err != nil {
			return nil, err
		}
		remote := retrieved["objects/"+descriptor.ObjectName+".object"]
		if descriptor.ChildTag != "" && len(remote) > 0 {
			localNode, localErr := parseMetadataXML(local)
			remoteNode, remoteErr := parseMetadataXML(remote)
			if localErr == nil && remoteErr == nil {
				member := strings.TrimPrefix(descriptor.Member, descriptor.ObjectName+".")
				if remoteChild := findMetadataChild(remoteNode, descriptor.ChildTag, member); remoteChild != nil {
					adapted := cloneMetadataNode(remoteChild)
					adapted.Name = localNode.Name
					remote = renderMetadataNode(projectMetadataNode(localNode, adapted))
				} else {
					remote = nil
				}
			}
		}
		return []MetadataFileDiff{newMetadataFileDiff(component, descriptor.Relative, remote, local, componentMissing)}, nil
	}

	if root := metadataBundleRoot(descriptor.Relative); root != "" {
		return metadataBundleFileDiffs(defaultRoot, component, filepath.ToSlash(root), retrieved, componentMissing)
	}

	files := []MetadataFileDiff{}
	localPrimary := strings.TrimSuffix(descriptor.Path, "-meta.xml")
	if _, err := os.Stat(localPrimary); err == nil {
		primaryRelative := strings.TrimSuffix(descriptor.Relative, "-meta.xml")
		local, readErr := os.ReadFile(localPrimary)
		if readErr != nil {
			return nil, readErr
		}
		if componentMissing || !canonicalMetadataData(primaryRelative, local, retrieved[primaryRelative]) {
			files = append(files, newMetadataFileDiff(component, primaryRelative, retrieved[primaryRelative], local, componentMissing))
		}
		localMeta, readErr := os.ReadFile(descriptor.Path)
		if readErr != nil {
			return nil, readErr
		}
		if componentMissing || !canonicalMetadataData(descriptor.Relative, localMeta, retrieved[descriptor.Relative]) {
			files = append(files, newMetadataFileDiff(component, descriptor.Relative, retrieved[descriptor.Relative], localMeta, componentMissing))
		}
		return files, nil
	}

	target := strings.TrimSuffix(descriptor.Relative, "-meta.xml")
	local, err := os.ReadFile(descriptor.Path)
	if err != nil {
		return nil, err
	}
	return []MetadataFileDiff{newMetadataFileDiff(component, descriptor.Relative, retrieved[target], local, componentMissing)}, nil
}

func metadataBundleFileDiffs(defaultRoot, component, root string, retrieved map[string][]byte, componentMissing bool) ([]MetadataFileDiff, error) {
	localRoot := filepath.Join(defaultRoot, filepath.FromSlash(root))
	files := []MetadataFileDiff{}
	err := filepath.WalkDir(localRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(defaultRoot, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		local, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		remote := retrieved[key]
		if componentMissing || !canonicalMetadataData(key, local, remote) {
			files = append(files, newMetadataFileDiff(component, key, remote, local, componentMissing))
		}
		return nil
	})
	return files, err
}

func newMetadataFileDiff(component, path string, before, after []byte, missing bool) MetadataFileDiff {
	previewable := len(before) <= metadataDiffPreviewLimit && len(after) <= metadataDiffPreviewLimit && utf8.Valid(before) && utf8.Valid(after)
	result := MetadataFileDiff{Component: component, Path: path, Kind: "update", Previewable: previewable}
	if missing || len(before) == 0 {
		result.Kind = "add"
	}
	if previewable {
		result.Before, result.After = displayMetadataPair(before, after)
	}
	return result
}

func displayMetadataPair(before, after []byte) (string, string) {
	trimmedAfter := bytes.TrimSpace(after)
	trimmedBefore := bytes.TrimSpace(before)
	if bytes.HasPrefix(trimmedAfter, []byte("<")) && bytes.HasPrefix(trimmedBefore, []byte("<")) {
		local, localErr := parseMetadataXML(after)
		remote, remoteErr := parseMetadataXML(before)
		if localErr == nil && remoteErr == nil {
			return string(renderMetadataNode(projectMetadataNode(local, remote))), string(renderMetadataNode(local))
		}
	}
	if json.Valid(after) && (len(before) == 0 || json.Valid(before)) {
		return indentJSON(before), indentJSON(after)
	}
	return normalizeMetadataText(before), normalizeMetadataText(after)
}

func indentJSON(data []byte) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return ""
	}
	var output bytes.Buffer
	if json.Indent(&output, data, "", "  ") == nil {
		return output.String()
	}
	return normalizeMetadataText(data)
}

func projectMetadataNode(local, remote *metadataXMLNode) *metadataXMLNode {
	if local == nil || remote == nil {
		return &metadataXMLNode{}
	}
	projected := &metadataXMLNode{Name: local.Name, Attrs: map[string]string{}, Text: remote.Text}
	for key := range local.Attrs {
		if value, ok := remote.Attrs[key]; ok {
			projected.Attrs[key] = value
		}
	}
	used := make([]bool, len(remote.Children))
	nextUnkeyedIndex := map[string]int{}
	localCounts := map[string]int{}
	for _, child := range local.Children {
		localCounts[child.Name]++
	}
	for _, child := range local.Children {
		identityName, identityValue := metadataNodeIdentity(child)
		start := 0
		if identityName == "" && localCounts[child.Name] > 1 {
			start = nextUnkeyedIndex[child.Name]
		}
		match := -1
		for index := start; index < len(remote.Children); index++ {
			candidate := remote.Children[index]
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
		if match < 0 {
			continue
		}
		used[match] = true
		if identityName == "" && localCounts[child.Name] > 1 {
			nextUnkeyedIndex[child.Name] = match + 1
		}
		projected.Children = append(projected.Children, projectMetadataNode(child, remote.Children[match]))
	}
	return projected
}

func cloneMetadataNode(node *metadataXMLNode) *metadataXMLNode {
	if node == nil {
		return nil
	}
	clone := &metadataXMLNode{Name: node.Name, Text: node.Text, Attrs: map[string]string{}}
	for key, value := range node.Attrs {
		clone.Attrs[key] = value
	}
	for _, child := range node.Children {
		clone.Children = append(clone.Children, cloneMetadataNode(child))
	}
	return clone
}

func renderMetadataNode(node *metadataXMLNode) []byte {
	var output bytes.Buffer
	output.WriteString(xml.Header)
	encoder := xml.NewEncoder(&output)
	encoder.Indent("", "  ")
	writeMetadataNode(encoder, node)
	_ = encoder.Flush()
	return output.Bytes()
}

func writeMetadataNode(encoder *xml.Encoder, node *metadataXMLNode) {
	start := xml.StartElement{Name: xml.Name{Local: node.Name}}
	keys := make([]string, 0, len(node.Attrs))
	for key := range node.Attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: key}, Value: node.Attrs[key]})
	}
	_ = encoder.EncodeToken(start)
	if node.Text != "" {
		_ = encoder.EncodeToken(xml.CharData([]byte(node.Text)))
	}
	for _, child := range node.Children {
		writeMetadataNode(encoder, child)
	}
	_ = encoder.EncodeToken(start.End())
}
