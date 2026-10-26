package boxdiff

import (
	"reflect"
	"testing"
	"time"

	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

func TestNormalizeTemplateOmitsEnterpriseSpecificIdentifiers(t *testing.T) {
	key, name, scope := "contracts", "Contracts", "enterprise_eid-one"
	description, fieldID, taxonomyID, namespace, optionID := "Contract stage", "field-1", "taxonomy-1", "enterprise_eid-one", "option-1"
	hidden, copyInstances, multiSelect := false, true, true
	template := &schemas.MetadataTemplate{
		Id: "template-1", Scope: &scope, TemplateKey: &key, DisplayName: &name, Hidden: &hidden, CopyInstanceOnItemCopy: &copyInstances,
		Fields: []schemas.MetadataTemplateFields{{
			Id: &fieldID, Key: "stage", Type: schemas.MetadataTemplateFieldsTypeEnum, DisplayName: "Stage",
			Description: &description, Options: []schemas.MetadataTemplateFieldsOptions{{Key: "draft", Id: &optionID}},
			TaxonomyKey: &key, TaxonomyId: &taxonomyID, Namespace: &namespace,
			OptionsRules: &schemas.FieldsOptionsRules{MultiSelect: &multiSelect, SelectableLevels: []int64{1, 2}},
		}},
	}

	got := NormalizeTemplate(template)
	want := MetadataTemplate{
		Key: "contracts", DisplayName: "Contracts", CopyInstanceOnItemCopy: true,
		Fields: []MetadataTemplateField{{Key: "stage", Type: "enum", DisplayName: "Stage", Description: "Contract stage", Options: []string{"draft"}, TaxonomyKey: "contracts", MultiSelect: true, SelectableLevels: []int64{1, 2}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized template = %#v, want %#v", got, want)
	}
}

func TestCompareSnapshotsReportsSemanticChangesAndPresence(t *testing.T) {
	left := Snapshot{Environment: Environment{EnterpriseID: "eid-left"}, Templates: []MetadataTemplate{
		{Key: "contracts", DisplayName: "Contracts", Fields: []MetadataTemplateField{{Key: "stage", Type: "enum", DisplayName: "Stage", Options: []string{"draft", "signed"}}}},
		{Key: "left", DisplayName: "Left only"},
	}}
	right := Snapshot{Environment: Environment{EnterpriseID: "eid-right"}, Templates: []MetadataTemplate{
		{Key: "contracts", DisplayName: "Contract records", Fields: []MetadataTemplateField{{Key: "stage", Type: "enum", DisplayName: "Stage", Options: []string{"draft", "approved", "signed"}}}},
		{Key: "right", DisplayName: "Right only"},
	}}

	comparison, err := CompareSnapshots(time.Unix(1, 0), left, right, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Summary != (Summary{Changed: 1, LeftOnly: 1, RightOnly: 1}) {
		t.Fatalf("summary = %+v", comparison.Summary)
	}
	changed := comparison.Templates[0]
	if changed.Key != "contracts" || changed.Status != "changed" {
		t.Fatalf("changed diff = %+v", changed)
	}
	paths := make([]string, 0, len(changed.Changes))
	for _, change := range changed.Changes {
		paths = append(paths, change.Path)
	}
	if !reflect.DeepEqual(paths, []string{"displayName", "fields.stage.options"}) {
		t.Fatalf("change paths = %v", paths)
	}
	if comparison.Templates[1].Status != "left_only" || comparison.Templates[2].Status != "right_only" {
		t.Fatalf("presence diffs = %+v", comparison.Templates)
	}
}

func TestCompareSnapshotsTreatsNormalizedEquivalentTemplatesAsEqual(t *testing.T) {
	template := MetadataTemplate{Key: "contracts", DisplayName: "Contracts", Fields: []MetadataTemplateField{{Key: "owner", Type: "string", DisplayName: "Owner"}}}
	comparison, err := CompareSnapshots(time.Time{}, Snapshot{Environment: Environment{EnterpriseID: "one"}, Templates: []MetadataTemplate{template}}, Snapshot{Environment: Environment{EnterpriseID: "two"}, Templates: []MetadataTemplate{template}}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Summary.Equal != 1 || comparison.Templates[0].Status != "equal" || len(comparison.Templates[0].Changes) != 0 {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestCompareSnapshotsRejectsSameEnterprise(t *testing.T) {
	_, err := CompareSnapshots(time.Time{}, Snapshot{Environment: Environment{EnterpriseID: "same"}}, Snapshot{Environment: Environment{EnterpriseID: "same"}}, time.Time{})
	if err == nil {
		t.Fatal("expected same-enterprise comparison to fail")
	}
}
