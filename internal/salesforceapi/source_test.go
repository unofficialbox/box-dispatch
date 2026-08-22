package salesforceapi

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryAndBuildMetadataPackage(t *testing.T) {
	project := t.TempDir()
	writeSourceFixture(t, project, "sfdx-project.json", `{"sourceApiVersion":"67.0"}`)
	writeSourceFixture(t, project, "force-app/main/default/objects/Contract__c/Contract__c.object-meta.xml", metadataXML("CustomObject", "<label>Contract</label>"))
	writeSourceFixture(t, project, "force-app/main/default/objects/Contract__c/fields/Status__c.field-meta.xml", metadataXML("CustomField", "<fullName>Status__c</fullName><label>Status</label><type>Text</type>"))
	writeSourceFixture(t, project, "force-app/main/default/objects/Contract__c/listViews/All.listView-meta.xml", metadataXML("ListView", "<fullName>All</fullName><label>All</label>"))
	writeSourceFixture(t, project, "force-app/main/default/classes/ContractService.cls", "public class ContractService {}")
	writeSourceFixture(t, project, "force-app/main/default/classes/ContractService.cls-meta.xml", metadataXML("ApexClass", "<apiVersion>67.0</apiVersion><status>Active</status>"))
	writeSourceFixture(t, project, "force-app/main/default/applications/Contract.app-meta.xml", metadataXML("CustomApplication", "<label>Contracts</label>"))
	writeSourceFixture(t, project, "force-app/main/default/uiBundles/contractBox/ui-bundle.json", "{}")
	writeSourceFixture(t, project, "force-app/main/default/uiBundles/contractBox/ui-bundle.json-meta.xml", metadataXML("UIBundle", "<description>Box panel</description>"))

	inventory, version, err := InventorySource(project)
	if err != nil {
		t.Fatal(err)
	}
	wantComponents := []string{
		"ApexClass:ContractService",
		"CustomApplication:Contract",
		"CustomField:Contract__c.Status__c",
		"CustomObject:Contract__c",
		"ListView:Contract__c.All",
		"UIBundle:contractBox",
	}
	for _, component := range wantComponents {
		if !inventory[component] {
			t.Errorf("inventory missing %s: %#v", component, inventory)
		}
	}
	if version != "67.0" {
		t.Fatalf("version = %q", version)
	}

	data, deployedVersion, err := BuildMetadataPackage(project, wantComponents)
	if err != nil {
		t.Fatal(err)
	}
	if deployedVersion != version {
		t.Fatalf("deployed version = %q", deployedVersion)
	}
	files := readZipFiles(t, data)
	object := string(files["objects/Contract__c.object"])
	if !strings.Contains(object, "<fields><fullName>Status__c</fullName>") || !strings.Contains(object, "<listViews><fullName>All</fullName>") {
		t.Fatalf("composed object = %s", object)
	}
	for _, path := range []string{
		"applications/Contract.app",
		"classes/ContractService.cls",
		"classes/ContractService.cls-meta.xml",
		"uiBundles/contractBox/ui-bundle.json",
		"uiBundles/contractBox/ui-bundle.json-meta.xml",
	} {
		if _, ok := files[path]; !ok {
			t.Errorf("archive missing %s: %#v", path, files)
		}
	}
	manifest := string(files["package.xml"])
	for _, member := range []string{"<members>Contract__c.Status__c</members>", "<members>ContractService</members>", "<name>UIBundle</name>", "<version>67.0</version>"} {
		if !strings.Contains(manifest, member) {
			t.Errorf("manifest missing %s: %s", member, manifest)
		}
	}
}

func TestBuildMetadataPackageRejectsUnknownComponent(t *testing.T) {
	project := t.TempDir()
	writeSourceFixture(t, project, "force-app/main/default/classes/Known.cls-meta.xml", metadataXML("ApexClass", "<apiVersion>67.0</apiVersion>"))
	_, _, err := BuildMetadataPackage(project, []string{"ApexClass:Missing"})
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("err = %v", err)
	}
}

func metadataXML(root, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n<" + root + ` xmlns="http://soap.sforce.com/2006/04/metadata">` + inner + "</" + root + ">"
}

func writeSourceFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readZipFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range reader.File {
		contents, err := func() ([]byte, error) {
			opened, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer opened.Close()
			var output bytes.Buffer
			_, err = output.ReadFrom(opened)
			return output.Bytes(), err
		}()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = contents
	}
	return files
}
