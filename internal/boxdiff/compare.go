// Package boxdiff compares live, normalized Box configuration snapshots.
package boxdiff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

type Environment struct {
	ConnectionID string `json:"connectionId"`
	Alias        string `json:"alias"`
	Identity     string `json:"identity"`
	EnterpriseID string `json:"enterpriseId"`
	Domain       string `json:"domain,omitempty"`
	CapturedAt   string `json:"capturedAt"`
}

type MetadataTemplate struct {
	Key                    string                  `json:"key"`
	DisplayName            string                  `json:"displayName"`
	Hidden                 bool                    `json:"hidden"`
	CopyInstanceOnItemCopy bool                    `json:"copyInstanceOnItemCopy"`
	Fields                 []MetadataTemplateField `json:"fields"`
}

type MetadataTemplateField struct {
	Key              string   `json:"key"`
	Type             string   `json:"type"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description,omitempty"`
	Hidden           bool     `json:"hidden"`
	Options          []string `json:"options,omitempty"`
	TaxonomyKey      string   `json:"taxonomyKey,omitempty"`
	MultiSelect      bool     `json:"multiSelect,omitempty"`
	SelectableLevels []int64  `json:"selectableLevels,omitempty"`
}

type Snapshot struct {
	Environment Environment        `json:"environment"`
	Templates   []MetadataTemplate `json:"templates"`
}

type Change struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

type MetadataTemplateDiff struct {
	Key         string            `json:"key"`
	DisplayName string            `json:"displayName"`
	Status      string            `json:"status"`
	Changes     []Change          `json:"changes"`
	Left        *MetadataTemplate `json:"left,omitempty"`
	Right       *MetadataTemplate `json:"right,omitempty"`
}

type Summary struct {
	Equal     int `json:"equal"`
	Changed   int `json:"changed"`
	LeftOnly  int `json:"leftOnly"`
	RightOnly int `json:"rightOnly"`
}

type Comparison struct {
	StartedAt   string                 `json:"startedAt"`
	CompletedAt string                 `json:"completedAt"`
	Left        Environment            `json:"left"`
	Right       Environment            `json:"right"`
	Summary     Summary                `json:"summary"`
	Templates   []MetadataTemplateDiff `json:"templates"`
}

func NormalizeTemplate(template *schemas.MetadataTemplate) MetadataTemplate {
	normalized := MetadataTemplate{
		Key:                    stringValue(template.TemplateKey),
		DisplayName:            stringValue(template.DisplayName),
		Hidden:                 boolValue(template.Hidden),
		CopyInstanceOnItemCopy: boolValue(template.CopyInstanceOnItemCopy),
		Fields:                 make([]MetadataTemplateField, 0, len(template.Fields)),
	}
	for _, field := range template.Fields {
		options := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, option.Key)
		}
		normalizedField := MetadataTemplateField{
			Key: field.Key, Type: string(field.Type), DisplayName: field.DisplayName,
			Description: stringValue(field.Description), Hidden: boolValue(field.Hidden),
			Options: options, TaxonomyKey: stringValue(field.TaxonomyKey),
		}
		if field.OptionsRules != nil {
			normalizedField.MultiSelect = boolValue(field.OptionsRules.MultiSelect)
			normalizedField.SelectableLevels = append([]int64(nil), field.OptionsRules.SelectableLevels...)
		}
		normalized.Fields = append(normalized.Fields, normalizedField)
	}
	return normalized
}

func CompareSnapshots(startedAt time.Time, left, right Snapshot, completedAt time.Time) (Comparison, error) {
	if strings.TrimSpace(left.Environment.EnterpriseID) == "" || strings.TrimSpace(right.Environment.EnterpriseID) == "" {
		return Comparison{}, fmt.Errorf("both live Box connections must report an enterprise ID")
	}
	if left.Environment.EnterpriseID == right.Environment.EnterpriseID {
		return Comparison{}, fmt.Errorf("select Box connections from two different enterprises")
	}
	comparison := Comparison{
		StartedAt: startedAt.UTC().Format(time.RFC3339Nano), CompletedAt: completedAt.UTC().Format(time.RFC3339Nano),
		Left: left.Environment, Right: right.Environment,
	}
	leftByKey := templatesByKey(left.Templates)
	rightByKey := templatesByKey(right.Templates)
	keys := make([]string, 0, len(leftByKey)+len(rightByKey))
	seen := map[string]bool{}
	for key := range leftByKey {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range rightByKey {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		leftTemplate, onLeft := leftByKey[key]
		rightTemplate, onRight := rightByKey[key]
		diff := MetadataTemplateDiff{Key: key, Changes: []Change{}}
		switch {
		case !onRight:
			diff.Status = "left_only"
			diff.DisplayName = leftTemplate.DisplayName
			diff.Left = templatePointer(leftTemplate)
			comparison.Summary.LeftOnly++
		case !onLeft:
			diff.Status = "right_only"
			diff.DisplayName = rightTemplate.DisplayName
			diff.Right = templatePointer(rightTemplate)
			comparison.Summary.RightOnly++
		default:
			diff.DisplayName = firstNonEmpty(leftTemplate.DisplayName, rightTemplate.DisplayName, key)
			diff.Left = templatePointer(leftTemplate)
			diff.Right = templatePointer(rightTemplate)
			diff.Changes = compareTemplate(leftTemplate, rightTemplate)
			if len(diff.Changes) == 0 {
				diff.Status = "equal"
				comparison.Summary.Equal++
			} else {
				diff.Status = "changed"
				comparison.Summary.Changed++
			}
		}
		comparison.Templates = append(comparison.Templates, diff)
	}
	return comparison, nil
}

func compareTemplate(left, right MetadataTemplate) []Change {
	leftValues := flattenTemplate(left)
	rightValues := flattenTemplate(right)
	paths := make([]string, 0, len(leftValues)+len(rightValues))
	seen := map[string]bool{}
	for path := range leftValues {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range rightValues {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	changes := make([]Change, 0)
	for _, path := range paths {
		before, beforeOK := leftValues[path]
		after, afterOK := rightValues[path]
		if beforeOK == afterOK && reflect.DeepEqual(before, after) {
			continue
		}
		changes = append(changes, Change{Path: path, Before: before, After: after})
	}
	return changes
}

func flattenTemplate(template MetadataTemplate) map[string]any {
	values := map[string]any{
		"displayName": template.DisplayName, "hidden": template.Hidden,
		"copyInstanceOnItemCopy": template.CopyInstanceOnItemCopy,
	}
	order := make([]string, 0, len(template.Fields))
	for _, field := range template.Fields {
		order = append(order, field.Key)
		prefix := "fields." + field.Key + "."
		values[prefix+"type"] = field.Type
		values[prefix+"displayName"] = field.DisplayName
		values[prefix+"description"] = field.Description
		values[prefix+"hidden"] = field.Hidden
		values[prefix+"options"] = field.Options
		values[prefix+"taxonomyKey"] = field.TaxonomyKey
		values[prefix+"multiSelect"] = field.MultiSelect
		values[prefix+"selectableLevels"] = field.SelectableLevels
	}
	values["fields.order"] = order
	return values
}

func templatesByKey(templates []MetadataTemplate) map[string]MetadataTemplate {
	result := make(map[string]MetadataTemplate, len(templates))
	for _, template := range templates {
		if template.Key != "" {
			result[template.Key] = template
		}
	}
	return result
}

func templatePointer(template MetadataTemplate) *MetadataTemplate { return &template }

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolValue(value *bool) bool { return value != nil && *value }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
