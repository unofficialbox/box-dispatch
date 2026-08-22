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

type sourceDescriptor struct {
	Type       string
	Member     string
	Path       string
	Relative   string
	ObjectName string
	ChildTag   string
}

func InventorySource(project string) (map[string]bool, string, error) {
	descriptors, version, err := readSourceDescriptors(project)
	if err != nil {
		return nil, "", err
	}
	result := make(map[string]bool, len(descriptors))
	for component := range descriptors {
		result[component] = true
	}
	return result, version, nil
}

func BuildMetadataPackage(project string, selected []string) ([]byte, string, error) {
	descriptors, version, err := readSourceDescriptors(project)
	if err != nil {
		return nil, "", err
	}
	selectedSet := map[string]bool{}
	for _, component := range selected {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}
		if _, ok := descriptors[component]; !ok {
			return nil, "", fmt.Errorf("Salesforce source component %s was not found in the package", component)
		}
		selectedSet[component] = true
	}
	if len(selectedSet) == 0 {
		return nil, "", fmt.Errorf("no Salesforce metadata components were selected")
	}

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	manifestMembers := map[string][]string{}
	objectChildren := map[string][]sourceDescriptor{}
	objectBases := map[string]sourceDescriptor{}
	writtenBundles := map[string]bool{}

	components := make([]string, 0, len(selectedSet))
	for component := range selectedSet {
		components = append(components, component)
	}
	sort.Strings(components)
	for _, component := range components {
		descriptor := descriptors[component]
		manifestMembers[descriptor.Type] = append(manifestMembers[descriptor.Type], descriptor.Member)
		if descriptor.ObjectName != "" {
			if descriptor.ChildTag == "" {
				objectBases[descriptor.ObjectName] = descriptor
			} else {
				objectChildren[descriptor.ObjectName] = append(objectChildren[descriptor.ObjectName], descriptor)
			}
			continue
		}
		if bundleRoot := metadataBundleRoot(descriptor.Relative); bundleRoot != "" {
			if !writtenBundles[bundleRoot] {
				if err := addDirectoryToZip(archive, filepath.Join(sourceDefaultRoot(project), filepath.FromSlash(bundleRoot)), bundleRoot); err != nil {
					_ = archive.Close()
					return nil, "", err
				}
				writtenBundles[bundleRoot] = true
			}
			continue
		}
		if err := addDescriptorToZip(archive, descriptor); err != nil {
			_ = archive.Close()
			return nil, "", err
		}
	}

	for objectName, children := range objectChildren {
		base, ok := objectBases[objectName]
		if !ok {
			base = descriptors["CustomObject:"+objectName]
		}
		if base.Path == "" {
			_ = archive.Close()
			return nil, "", fmt.Errorf("Salesforce custom object source %s is missing its object descriptor", objectName)
		}
		if err := addComposedObject(archive, base, children); err != nil {
			_ = archive.Close()
			return nil, "", err
		}
		delete(objectBases, objectName)
	}
	for _, base := range objectBases {
		if err := addComposedObject(archive, base, nil); err != nil {
			_ = archive.Close()
			return nil, "", err
		}
	}

	manifest, err := metadataManifest(version, manifestMembers)
	if err != nil {
		_ = archive.Close()
		return nil, "", err
	}
	if err := addZipFile(archive, "package.xml", manifest); err != nil {
		_ = archive.Close()
		return nil, "", err
	}
	if err := archive.Close(); err != nil {
		return nil, "", err
	}
	return output.Bytes(), version, nil
}

func readSourceDescriptors(project string) (map[string]sourceDescriptor, string, error) {
	root := sourceDefaultRoot(project)
	version := ""
	data, err := os.ReadFile(filepath.Join(project, "sfdx-project.json"))
	if err == nil {
		var settings struct {
			SourceAPIVersion string `json:"sourceApiVersion"`
		}
		if json.Unmarshal(data, &settings) == nil {
			version = strings.TrimSpace(settings.SourceAPIVersion)
		}
	}
	result := map[string]sourceDescriptor{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "-meta.xml") {
			return nil
		}
		metadataType, err := metadataRootType(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		descriptor := sourceDescriptor{Type: metadataType, Path: path, Relative: relative}
		parts := strings.Split(relative, "/")
		base := strings.TrimSuffix(entry.Name(), "-meta.xml")
		base = strings.TrimSuffix(base, filepath.Ext(base))
		switch metadataType {
		case "CustomObject":
			if len(parts) < 2 {
				return fmt.Errorf("invalid Salesforce custom object path %s", relative)
			}
			descriptor.Member, descriptor.ObjectName = parts[1], parts[1]
		case "CustomField":
			if len(parts) < 4 {
				return fmt.Errorf("invalid Salesforce custom field path %s", relative)
			}
			descriptor.ObjectName, descriptor.ChildTag = parts[1], "fields"
			descriptor.Member = parts[1] + "." + base
		case "ListView":
			if len(parts) < 4 {
				return fmt.Errorf("invalid Salesforce list view path %s", relative)
			}
			descriptor.ObjectName, descriptor.ChildTag = parts[1], "listViews"
			descriptor.Member = parts[1] + "." + base
		default:
			if metadataBundleRoot(relative) != "" {
				descriptor.Member = parts[1]
			} else {
				descriptor.Member = base
			}
		}
		result[descriptor.Type+":"+descriptor.Member] = descriptor
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("inventory Salesforce source: %w", err)
	}
	return result, version, nil
}

func sourceDefaultRoot(project string) string {
	return filepath.Join(project, "force-app", "main", "default")
}

func metadataRootType(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("parse Salesforce metadata descriptor %s: %w", filepath.Base(path), err)
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}

func metadataBundleRoot(relative string) string {
	parts := strings.Split(relative, "/")
	if len(parts) < 3 {
		return ""
	}
	switch parts[0] {
	case "aura", "lwc", "uiBundles", "experienceBundles":
		return strings.Join(parts[:2], "/")
	default:
		return ""
	}
}

func addDescriptorToZip(archive *zip.Writer, descriptor sourceDescriptor) error {
	primary := strings.TrimSuffix(descriptor.Path, "-meta.xml")
	if _, err := os.Stat(primary); err == nil {
		if err := copyPathToZip(archive, primary, strings.TrimSuffix(descriptor.Relative, "-meta.xml")); err != nil {
			return err
		}
		return copyPathToZip(archive, descriptor.Path, descriptor.Relative)
	}
	return copyPathToZip(archive, descriptor.Path, strings.TrimSuffix(descriptor.Relative, "-meta.xml"))
}

func addComposedObject(archive *zip.Writer, base sourceDescriptor, children []sourceDescriptor) error {
	data, err := os.ReadFile(base.Path)
	if err != nil {
		return err
	}
	closing := []byte("</CustomObject>")
	index := bytes.LastIndex(data, closing)
	if index < 0 {
		return fmt.Errorf("Salesforce custom object %s has no closing element", base.Member)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Member < children[j].Member })
	var additions bytes.Buffer
	for _, child := range children {
		childData, err := os.ReadFile(child.Path)
		if err != nil {
			return err
		}
		inner, err := metadataInnerXML(childData)
		if err != nil {
			return fmt.Errorf("compose Salesforce metadata %s: %w", child.Member, err)
		}
		additions.WriteString("    <" + child.ChildTag + ">")
		additions.Write(inner)
		additions.WriteString("</" + child.ChildTag + ">\n")
	}
	composed := append([]byte(nil), data[:index]...)
	composed = append(composed, additions.Bytes()...)
	composed = append(composed, data[index:]...)
	return addZipFile(archive, "objects/"+base.Member+".object", composed)
}

func metadataInnerXML(data []byte) ([]byte, error) {
	searchStart := 0
	if declaration := bytes.Index(data, []byte("?>")); declaration >= 0 {
		searchStart = declaration + 2
	}
	rootStart := bytes.IndexByte(data[searchStart:], '<')
	if rootStart >= 0 {
		rootStart += searchStart
	}
	start := -1
	if rootStart >= 0 {
		if offset := bytes.IndexByte(data[rootStart:], '>'); offset >= 0 {
			start = rootStart + offset
		}
	}
	end := bytes.LastIndex(data, []byte("</"))
	if start < 0 || end <= start {
		return nil, fmt.Errorf("metadata descriptor is malformed")
	}
	return bytes.TrimSpace(data[start+1 : end]), nil
}

func addDirectoryToZip(archive *zip.Writer, source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		return copyPathToZip(archive, path, filepath.ToSlash(filepath.Join(destination, relative)))
	})
}

func copyPathToZip(archive *zip.Writer, source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	writer, err := archive.Create(filepath.ToSlash(destination))
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}

func addZipFile(archive *zip.Writer, path string, data []byte) error {
	writer, err := archive.Create(path)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func metadataManifest(version string, members map[string][]string) ([]byte, error) {
	if version == "" {
		version = "67.0"
	}
	var output strings.Builder
	output.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	output.WriteString(`<Package xmlns="http://soap.sforce.com/2006/04/metadata">` + "\n")
	types := make([]string, 0, len(members))
	for metadataType := range members {
		types = append(types, metadataType)
	}
	sort.Strings(types)
	for _, metadataType := range types {
		output.WriteString("    <types>\n")
		values := append([]string(nil), members[metadataType]...)
		sort.Strings(values)
		for _, member := range values {
			output.WriteString("        <members>" + xmlEscape(member) + "</members>\n")
		}
		output.WriteString("        <name>" + xmlEscape(metadataType) + "</name>\n")
		output.WriteString("    </types>\n")
	}
	output.WriteString("    <version>" + xmlEscape(version) + "</version>\n</Package>\n")
	return []byte(output.String()), nil
}
