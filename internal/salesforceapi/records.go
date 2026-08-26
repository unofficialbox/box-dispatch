package salesforceapi

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var salesforceIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type RecordReference struct {
	ID   string
	Key  string
	Name string
}

// FindRecordsByStringField resolves immutable Salesforce record IDs for a
// bounded list of package-owned external keys.
func (c *Client) FindRecordsByStringField(ctx context.Context, credential Credential, object, field string, keys []string) ([]RecordReference, error) {
	if !credential.Valid() {
		return nil, fmt.Errorf("Salesforce connection is incomplete")
	}
	if !salesforceIdentifier.MatchString(object) || !salesforceIdentifier.MatchString(field) {
		return nil, fmt.Errorf("Salesforce record lookup contains an invalid object or field name")
	}
	unique := map[string]bool{}
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			unique[key] = true
		}
	}
	if len(unique) == 0 {
		return nil, nil
	}
	values := make([]string, 0, len(unique))
	for key := range unique {
		values = append(values, "'"+strings.ReplaceAll(key, "'", "\\'")+"'")
	}
	sort.Strings(values)
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return nil, err
	}
	var result struct {
		Records []map[string]any `json:"records"`
	}
	soql := fmt.Sprintf("SELECT Id, Name, %s FROM %s WHERE %s IN (%s)", field, object, field, strings.Join(values, ","))
	if err := c.query(ctx, credential, version, soql, &result); err != nil {
		return nil, err
	}
	references := make([]RecordReference, 0, len(result.Records))
	for _, record := range result.Records {
		id, _ := record["Id"].(string)
		key, _ := record[field].(string)
		name, _ := record["Name"].(string)
		if strings.TrimSpace(id) != "" && strings.TrimSpace(key) != "" {
			references = append(references, RecordReference{ID: id, Key: key, Name: name})
		}
	}
	sort.Slice(references, func(i, j int) bool { return references[i].Key < references[j].Key })
	return references, nil
}
