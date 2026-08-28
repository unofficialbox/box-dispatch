package salesforceapi

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	writeSourceFixture(t, project, "force-app/main/default/settings/Communities.settings-meta.xml", metadataXML("CommunitiesSettings", "<enableNetworksEnabled>true</enableNetworksEnabled>"))
	writeSourceFixture(t, project, "force-app/main/default/digitalExperiences/site/ContractPortal1/ContractPortal1.digitalExperience-meta.xml", metadataXML("DigitalExperienceBundle", "<label>Contracts</label>"))
	writeSourceFixture(t, project, "force-app/main/default/digitalExperiences/site/ContractPortal1/sfdc_cms__site/ContractPortal1/_meta.json", `{"apiName":"ContractPortal1","type":"sfdc_cms__site"}`)
	writeSourceFixture(t, project, "force-app/main/default/digitalExperiences/site/ContractPortal1/sfdc_cms__site/ContractPortal1/content.json", `{"contentBody":{"appSpace":"c__contractBox"}}`)

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
		"DigitalExperience:site/ContractPortal1.sfdc_cms__site/ContractPortal1",
		"DigitalExperienceBundle:site/ContractPortal1",
		"Settings:Communities",
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
		"settings/Communities.settings",
		"digitalExperiences/site/ContractPortal1/ContractPortal1.digitalExperience-meta.xml",
		"digitalExperiences/site/ContractPortal1/sfdc_cms__site/ContractPortal1/_meta.json",
		"digitalExperiences/site/ContractPortal1/sfdc_cms__site/ContractPortal1/content.json",
	} {
		if _, ok := files[path]; !ok {
			t.Errorf("archive missing %s: %#v", path, files)
		}
	}
	manifest := string(files["package.xml"])
	for _, member := range []string{"<members>Contract__c.Status__c</members>", "<members>ContractService</members>", "<members>site/ContractPortal1.sfdc_cms__site/ContractPortal1</members>", "<name>DigitalExperience</name>", "<members>site/ContractPortal1</members>", "<name>DigitalExperienceBundle</name>", "<name>Settings</name>", "<name>UIBundle</name>", "<version>67.0</version>"} {
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

func TestCompareMetadataSourceDetectsChangedExistingConfiguration(t *testing.T) {
	project := t.TempDir()
	writeSourceFixture(t, project, "sfdx-project.json", `{"sourceApiVersion":"67.0"}`)
	writeSourceFixture(t, project, "force-app/main/default/objects/Contract__c/Contract__c.object-meta.xml", metadataXML("CustomObject", "<label>Contract</label><pluralLabel>Contracts</pluralLabel>"))
	writeSourceFixture(t, project, "force-app/main/default/objects/Contract__c/fields/Amount__c.field-meta.xml", metadataXML("CustomField", "<fullName>Amount__c</fullName><label>Amount</label><required>true</required><type>Currency</type>"))
	writeSourceFixture(t, project, "force-app/main/default/settings/Communities.settings-meta.xml", metadataXML("CommunitiesSettings", "<enableNetworksEnabled>true</enableNetworksEnabled>"))
	writeSourceFixture(t, project, "force-app/main/default/classes/ContractService.cls", "public class ContractService {}\n")
	writeSourceFixture(t, project, "force-app/main/default/classes/ContractService.cls-meta.xml", metadataXML("ApexClass", "<apiVersion>67.0</apiVersion><status>Active</status>"))
	writeSourceFixture(t, project, "force-app/main/default/uiBundles/contractBox/ui-bundle.json", `{"name":"contractBox","external":true}`)
	writeSourceFixture(t, project, "force-app/main/default/uiBundles/contractBox/ui-bundle.json-meta.xml", metadataXML("UIBundle", "<description>Contract workspace</description>"))
	components := []string{
		"ApexClass:ContractService",
		"CustomField:Contract__c.Amount__c",
		"CustomObject:Contract__c",
		"Settings:Communities",
		"UIBundle:contractBox",
	}

	matchingZip, _, err := BuildMetadataPackage(project, components)
	if err != nil {
		t.Fatal(err)
	}
	matchingFiles := readZipFiles(t, matchingZip)
	matchingFiles["classes/ContractService.cls-meta.xml"] = []byte(metadataXML("ApexClass", "<apiVersion>67.0</apiVersion><packageVersions><majorNumber>5</majorNumber><minorNumber>43</minorNumber><namespace>box</namespace></packageVersions><status>Active</status>"))
	matchingFiles["objects/Contract__c.object"] = []byte(metadataXML("CustomObject", "<actionOverrides><actionName>View</actionName><type>Default</type></actionOverrides><label>Contract</label><pluralLabel>Contracts</pluralLabel><fields><fullName>Amount__c</fullName><label>Amount</label><required>true</required><trackTrending>false</trackTrending><type>Currency</type></fields>"))
	matchingFiles["settings/Communities.settings"] = []byte(metadataXML("CommunitiesSettings", "<canModerateAllFeedPosts>false</canModerateAllFeedPosts><enableNetworksEnabled>true</enableNetworksEnabled>"))
	matchingFiles["uiBundles/contractBox/ui-bundle.json-meta.xml"] = []byte(metadataXML("UIBundle", "<description>Contract workspace</description><isDataApp>false</isDataApp>"))
	matchingZip = writeZipFiles(t, matchingFiles)
	matching, err := CompareMetadataSource(project, components, matchingZip)
	if err != nil {
		t.Fatal(err)
	}
	wantMatching := append([]string(nil), components...)
	sort.Strings(wantMatching)
	if !slices.Equal(matching.Matching, wantMatching) || len(matching.Missing) != 0 || len(matching.Changed) != 0 {
		t.Fatalf("matching diff = %#v", matching)
	}

	files := readZipFiles(t, matchingZip)
	files["settings/Communities.settings"] = []byte(metadataXML("CommunitiesSettings", "<enableNetworksEnabled>false</enableNetworksEnabled>"))
	files["objects/Contract__c.object"] = []byte(metadataXML("CustomObject", "<label>Contract</label><pluralLabel>Contracts</pluralLabel><fields><fullName>Amount__c</fullName><label>Contract Amount</label><required>false</required><type>Currency</type></fields>"))
	delete(files, "uiBundles/contractBox/ui-bundle.json")
	changedZip := writeZipFiles(t, files)
	diff, err := CompareMetadataSource(project, components, changedZip)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(diff.Changed, []string{"CustomField:Contract__c.Amount__c", "Settings:Communities"}) {
		t.Fatalf("changed = %#v", diff.Changed)
	}
	if !slices.Equal(diff.Missing, []string{"UIBundle:contractBox"}) {
		t.Fatalf("missing = %#v", diff.Missing)
	}
	if !slices.Equal(diff.Matching, []string{"ApexClass:ContractService", "CustomObject:Contract__c"}) {
		t.Fatalf("matching = %#v", diff.Matching)
	}
	if !slices.ContainsFunc(diff.Files, func(file MetadataFileDiff) bool {
		return file.Component == "Settings:Communities" && file.Kind == "update" && file.Previewable && strings.Contains(file.Before, "false") && strings.Contains(file.After, "true")
	}) {
		t.Fatalf("settings preview missing from %#v", diff.Files)
	}
	if !slices.ContainsFunc(diff.Files, func(file MetadataFileDiff) bool {
		return file.Component == "UIBundle:contractBox" && file.Path == "uiBundles/contractBox/ui-bundle.json" && file.Kind == "add" && file.Previewable
	}) {
		t.Fatalf("missing bundle preview missing from %#v", diff.Files)
	}
}

func TestMetadataNodeMatchesReorderedPermissionEntriesButDetectsChangedPermission(t *testing.T) {
	local, err := parseMetadataXML([]byte(metadataXML("PermissionSet", "<fieldPermissions><editable>true</editable><field>Contract__c.Amount__c</field><readable>true</readable></fieldPermissions><fieldPermissions><editable>false</editable><field>Contract__c.Status__c</field><readable>true</readable></fieldPermissions>")))
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := parseMetadataXML([]byte(metadataXML("PermissionSet", "<fieldPermissions><editable>false</editable><field>Contract__c.Status__c</field><readable>true</readable></fieldPermissions><fieldPermissions><editable>true</editable><field>Contract__c.Amount__c</field><readable>true</readable></fieldPermissions>")))
	if err != nil {
		t.Fatal(err)
	}
	if !metadataNodeMatches(local, reordered) {
		t.Fatal("reordered permission entries should be semantically equal")
	}
	changed, err := parseMetadataXML([]byte(metadataXML("PermissionSet", "<fieldPermissions><editable>false</editable><field>Contract__c.Status__c</field><readable>true</readable></fieldPermissions><fieldPermissions><editable>false</editable><field>Contract__c.Amount__c</field><readable>true</readable></fieldPermissions>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(local, changed) {
		t.Fatal("a changed field permission must require deployment")
	}
}

func TestMetadataNodeMatchesMaskedAuthProviderSecret(t *testing.T) {
	local, err := parseMetadataXML([]byte(metadataXML("AuthProvider", "<consumerKey>box-client</consumerKey><consumerSecret>set-in-setup</consumerSecret>")))
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := parseMetadataXML([]byte(metadataXML("AuthProvider", "<consumerKey>box-client</consumerKey><consumerSecret>Placeholder_Value</consumerSecret>")))
	if err != nil {
		t.Fatal(err)
	}
	if !metadataNodeMatches(local, retrieved) {
		t.Fatal("Salesforce's masked Auth Provider secret should preserve declared intent")
	}

	changedKey, err := parseMetadataXML([]byte(metadataXML("AuthProvider", "<consumerKey>different-client</consumerKey><consumerSecret>Placeholder_Value</consumerSecret>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(local, changedKey) {
		t.Fatal("an unrelated Auth Provider change must still require deployment")
	}
	emptySecret, err := parseMetadataXML([]byte(metadataXML("AuthProvider", "<consumerKey>box-client</consumerKey><consumerSecret></consumerSecret>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(emptySecret, retrieved) {
		t.Fatal("a placeholder must not prove that an empty package secret was configured")
	}
}

func TestMetadataNodeMatchesScratchNetworkSenderAddress(t *testing.T) {
	local, err := parseMetadataXML([]byte(metadataXML("Network", "<newSenderAddress>noreply@example.com</newSenderAddress><status>Live</status>")))
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := parseMetadataXML([]byte(metadataXML("Network", "<newSenderAddress>noreply@example.com.invalid</newSenderAddress><status>Live</status>")))
	if err != nil {
		t.Fatal(err)
	}
	if !metadataNodeMatches(local, retrieved) {
		t.Fatal("a scratch org's inert sender suffix should preserve the declared Network address")
	}

	changedAddress, err := parseMetadataXML([]byte(metadataXML("Network", "<newSenderAddress>different@example.com.invalid</newSenderAddress><status>Live</status>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(local, changedAddress) {
		t.Fatal("a different Network sender address must still require deployment")
	}
	changedStatus, err := parseMetadataXML([]byte(metadataXML("Network", "<newSenderAddress>noreply@example.com.invalid</newSenderAddress><status>DownForMaintenance</status>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(local, changedStatus) {
		t.Fatal("an unrelated Network change must still require deployment")
	}
}

func TestMetadataNodeDoesNotApplyServerOwnedValuesToOtherTypes(t *testing.T) {
	local, err := parseMetadataXML([]byte(metadataXML("NamedCredential", "<consumerSecret>set-in-setup</consumerSecret><newSenderAddress>noreply@example.com</newSenderAddress>")))
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := parseMetadataXML([]byte(metadataXML("NamedCredential", "<consumerSecret>Placeholder_Value</consumerSecret><newSenderAddress>noreply@example.com.invalid</newSenderAddress>")))
	if err != nil {
		t.Fatal(err)
	}
	if metadataNodeMatches(local, retrieved) {
		t.Fatal("server-owned comparison rules must remain scoped to their metadata types")
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

func writeZipFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		writer, err := archive.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(files[path]); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
