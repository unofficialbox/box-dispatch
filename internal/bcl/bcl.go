package bcl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"unicode"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

const (
	bclVersion = "1.0"
	// ext versions are namespace-owned and track extension-specific schemas.
	bclBoxExtVersion = "1.0"
)

var invalidResourceName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type BCLDocument struct {
	Version     string         `json:"version"`
	Scenario    string         `json:"scenario"`
	Provider    string         `json:"provider"`
	Context     string         `json:"context"`
	GeneratedAt string         `json:"generatedAt"`
	Ext         map[string]any `json:"ext,omitempty"`
	Resources   []BCLResource  `json:"resources"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type BCLResource struct {
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	TerraformType string         `json:"terraformType,omitempty"`
	Provider      string         `json:"provider"`
	Config        map[string]any `json:"config,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type TFJSONDocument struct {
	Resource map[string]map[string]map[string]any `json:"resource"`
}

func NewDocument(scenario, provider, context string, generatedAt string, metadata map[string]any) BCLDocument {
	return BCLDocument{
		Version:     bclVersion,
		Scenario:    scenario,
		Provider:    provider,
		Context:     context,
		GeneratedAt: generatedAt,
		Metadata:    metadata,
		Resources:   []BCLResource{},
	}
}

func FromDeployedArtifacts(scenario, provider, context string, generatedAt string, artifacts []config.DeployedArtifact) BCLDocument {
	doc := NewDocument(scenario, provider, context, generatedAt, map[string]any{
		"source": "environment",
	})
	for _, artifact := range artifacts {
		resource := BCLResource{
			Name:          sanitizeResourceName(artifact.ArtifactType, artifact.ProviderObjectID),
			Type:          artifact.ArtifactType,
			Provider:      artifact.Provider,
			TerraformType: terraformTypeForArtifact(artifact.ArtifactType),
			Metadata: map[string]any{
				"provider":         artifact.Provider,
				"artifactType":     artifact.ArtifactType,
				"providerObjectId": artifact.ProviderObjectID,
				"enterpriseId":     artifact.EnterpriseID,
				"artifactName":     artifact.ArtifactName,
				"scenario":         artifact.Scenario,
				"createdAt":        artifact.CreatedAt,
			},
			Config: map[string]any{
				"provider":           artifact.Provider,
				"artifact_type":      artifact.ArtifactType,
				"provider_object_id": artifact.ProviderObjectID,
				"enterprise_id":      artifact.EnterpriseID,
				"artifact_name":      artifact.ArtifactName,
				"created_at":         artifact.CreatedAt,
			},
		}
		doc.Resources = append(doc.Resources, resource)
	}
	doc.Ext = bclExtensions(doc.Resources)
	return doc
}

func sanitizeResourceName(artifactType, objectID string) string {
	base := strings.ToLower(fmt.Sprintf("%s_%s", artifactType, objectID))
	safe := invalidResourceName.ReplaceAllString(base, "_")
	safe = strings.Trim(safe, "_-")
	if safe == "" {
		return "resource"
	}
	return safe
}

func terraformTypeForArtifact(artifactType string) string {
	switch artifactType {
	case "box.folder":
		return "null_resource"
	case "box.enterprise":
		return "null_resource"
	case "salesforce.org":
		return "null_resource"
	default:
		return "null_resource"
	}
}

func WriteBCL(path string, doc BCLDocument) error {
	payload := toBCLHCL(doc)
	if err := ensureParentDir(path); err != nil {
		return err
	}
	data := []byte(payload + "\n")
	return os.WriteFile(path, data, 0o644)
}

func WriteTF(path string, doc BCLDocument) error {
	payload := toTFHCL(doc)
	if err := ensureParentDir(path); err != nil {
		return err
	}
	data := []byte(payload + "\n")
	return os.WriteFile(path, data, 0o644)
}

func WriteTFJSON(path string, doc BCLDocument) error {
	payload := toTFJSON(doc)
	if err := ensureParentDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteDeploymentBundle(path string, tfPath string, tfJSONPath string, doc BCLDocument) error {
	if err := WriteBCL(path, doc); err != nil {
		return err
	}
	if err := WriteTF(tfPath, doc); err != nil {
		return err
	}
	if err := WriteTFJSON(tfJSONPath, doc); err != nil {
		return err
	}
	return nil
}

func LoadBCL(path string) (BCLDocument, error) {
	var doc BCLDocument
	raw, err := os.ReadFile(path)
	if err != nil {
		return BCLDocument{}, err
	}
	if err := json.Unmarshal(raw, &doc); err == nil && doc.Provider != "" {
		normalizeBCLDocumentDefaults(&doc)
		return doc, nil
	}

	inventory, err := parseBCLLocals(raw)
	if err != nil {
		return BCLDocument{}, fmt.Errorf("unsupported BCL format at %q: %w", path, err)
	}

	payload, err := json.Marshal(inventory)
	if err != nil {
		return BCLDocument{}, err
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return BCLDocument{}, fmt.Errorf("unsupported BCL format at %q: unable to decode parsed inventory", path)
	}
	normalizeBCLDocumentDefaults(&doc)
	return doc, nil
}

func toBCLHCL(doc BCLDocument) string {
	var out strings.Builder
	out.WriteString("# Managed by box-dispatch\n")
	out.WriteString("locals {\n")
	out.WriteString("  bcl = ")
	if doc.Ext == nil {
		doc.Ext = map[string]any{}
	}
	inventory := map[string]any{
		"version":     doc.Version,
		"scenario":    doc.Scenario,
		"provider":    doc.Provider,
		"context":     doc.Context,
		"generatedAt": doc.GeneratedAt,
		"metadata":    doc.Metadata,
		"resources":   resourceSummaries(doc.Resources),
		"ext":         doc.Ext,
	}
	out.WriteString(renderHCLValue(inventory, 2))
	out.WriteString("\n}\n\n")

	for _, resource := range doc.Resources {
		tfType := terraformTypeForArtifact(resource.Type)
		if resource.TerraformType != "" {
			tfType = resource.TerraformType
		}
		out.WriteString(fmt.Sprintf("resource \"%s\" \"%s\" {\n", tfType, resource.Name))
		values := map[string]any{}
		for k, v := range resource.Config {
			values[k] = v
		}
		for k, v := range resource.Metadata {
			values[k] = v
		}
		out.WriteString("  triggers = ")
		out.WriteString(renderHCLValue(values, 1))
		out.WriteString("\n")
		out.WriteString("}\n\n")
	}
	return out.String()
}

func resourceSummaries(resources []BCLResource) []any {
	out := make([]any, 0, len(resources))
	for _, resource := range resources {
		out = append(out, map[string]any{
			"name":             resource.Name,
			"type":             resource.Type,
			"provider":         resource.Provider,
			"terraformType":    resource.TerraformType,
			"artifact_type":    maybeString(resource.Metadata, "artifactType"),
			"providerObjectId": maybeString(resource.Metadata, "providerObjectId"),
			"enterpriseId":     maybeString(resource.Metadata, "enterpriseId"),
			"artifactName":     maybeString(resource.Metadata, "artifactName"),
			"scenario":         maybeString(resource.Metadata, "scenario"),
			"createdAt":        maybeString(resource.Metadata, "createdAt"),
		})
	}
	return out
}

func bclExtensions(resources []BCLResource) map[string]any {
	box := map[string]any{}
	boxArtifacts := []any{}
	for _, resource := range resources {
		if resource.Provider != "box" {
			continue
		}
		boxArtifacts = append(boxArtifacts, map[string]any{
			"name":             resource.Name,
			"artifactType":     maybeString(resource.Metadata, "artifactType"),
			"providerObjectId": maybeString(resource.Metadata, "providerObjectId"),
			"enterpriseId":     maybeString(resource.Metadata, "enterpriseId"),
		})
	}
	if len(boxArtifacts) > 0 {
		box["artifacts"] = boxArtifacts
		box["version"] = bclBoxExtVersion
	}
	if len(box) == 0 {
		return map[string]any{}
	}
	return map[string]any{"box": box}
}

func maybeString(values map[string]any, key string) string {
	raw := values[key]
	if raw == nil {
		return ""
	}
	str, ok := raw.(string)
	if !ok {
		return fmt.Sprintf("%v", raw)
	}
	return str
}

func toTFHCL(doc BCLDocument) string {
	return toBCLHCL(doc)
}

func toTFJSON(doc BCLDocument) TFJSONDocument {
	out := TFJSONDocument{
		Resource: map[string]map[string]map[string]any{},
	}
	for _, resource := range doc.Resources {
		tfType := resource.TerraformType
		if tfType == "" {
			tfType = "null_resource"
		}
		if _, ok := out.Resource[tfType]; !ok {
			out.Resource[tfType] = map[string]map[string]any{}
		}
		name := resource.Name
		triggers := map[string]any{}
		for k, v := range resource.Config {
			triggers[k] = v
		}
		for k, v := range resource.Metadata {
			triggers[k] = v
		}
		if tfType == "null_resource" {
			out.Resource[tfType][name] = map[string]any{
				"triggers": triggers,
			}
			continue
		}
		out.Resource[tfType][name] = triggers
	}
	return out
}

func ExtractArtifactsFromBCL(doc BCLDocument) []config.DeployedArtifact {
	artifacts := make([]config.DeployedArtifact, 0, len(doc.Resources))
	seen := map[string]struct{}{}

	for _, resource := range doc.Resources {
		provider := strings.TrimSpace(resource.Provider)
		artifactType := strings.TrimSpace(resource.Type)
		if provider == "" || artifactType == "" {
			continue
		}
		providerObjectID := firstNonEmpty(
			maybeString(resource.Config, "provider_object_id"),
			maybeString(resource.Metadata, "providerObjectId"),
		)
		if providerObjectID == "" {
			continue
		}
		key := provider + "|" + artifactType + "|" + providerObjectID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		artifacts = append(artifacts, config.DeployedArtifact{
			Provider:         provider,
			Scenario:         firstNonEmpty(maybeString(resource.Metadata, "scenario"), doc.Scenario),
			ArtifactType:     artifactType,
			ProviderObjectID: providerObjectID,
			EnterpriseID:     firstNonEmpty(maybeString(resource.Metadata, "enterpriseId"), maybeString(resource.Config, "enterprise_id")),
			ArtifactName:     firstNonEmpty(maybeString(resource.Metadata, "artifactName"), maybeString(resource.Config, "artifact_name"), resource.Name),
			Source:           firstNonEmpty(maybeString(resource.Metadata, "source"), "bcl"),
			CreatedAt:        maybeString(resource.Metadata, "createdAt"),
		})
	}
	return artifacts
}

func parseBCLLocals(raw []byte) (map[string]any, error) {
	text := string(raw)
	key := strings.Index(text, "\"bcl\"")
	if key < 0 {
		return nil, fmt.Errorf("missing bcl inventory block")
	}
	p := &hclLikeParser{src: text, pos: key}
	if err := p.expectToken(`"bcl"`); err != nil {
		return nil, err
	}
	p.skipSpaceAndComments()
	if err := p.expectChar('='); err != nil {
		return nil, err
	}
	p.skipSpaceAndComments()
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected bcl inventory object, got %T", value)
	}
	return result, nil
}

type hclLikeParser struct {
	src string
	pos int
}

func (p *hclLikeParser) skipSpaceAndComments() {
	for p.pos < len(p.src) {
		switch {
		case unicode.IsSpace(rune(p.src[p.pos])):
			p.pos++
		case p.src[p.pos] == '#':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		case p.src[p.pos] == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			p.pos += 2
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *hclLikeParser) expectChar(expected byte) error {
	p.skipSpaceAndComments()
	if p.pos >= len(p.src) || p.src[p.pos] != expected {
		return fmt.Errorf("expected %q", string(expected))
	}
	p.pos++
	return nil
}

func (p *hclLikeParser) expectToken(token string) error {
	p.skipSpaceAndComments()
	if strings.HasPrefix(p.src[p.pos:], token) {
		p.pos += len(token)
		return nil
	}
	return fmt.Errorf("expected %q", token)
}

func (p *hclLikeParser) parseValue() (any, error) {
	p.skipSpaceAndComments()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of input")
	}
	switch p.src[p.pos] {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseString()
	default:
		if starts := p.src[p.pos:]; strings.HasPrefix(starts, "true") {
			p.pos += 4
			return true, nil
		}
		if strings.HasPrefix(starts, "false") {
			p.pos += 5
			return false, nil
		}
		if strings.HasPrefix(starts, "null") {
			p.pos += 4
			return nil, nil
		}
		if strings.ContainsRune("0123456789-.", rune(p.src[p.pos])) {
			return p.parseNumber()
		}
	}
	return nil, fmt.Errorf("unsupported value at %d", p.pos)
}

func (p *hclLikeParser) parseString() (string, error) {
	if p.pos >= len(p.src) || p.src[p.pos] != '"' {
		return "", fmt.Errorf("expected string")
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' {
			if p.pos+1 >= len(p.src) {
				return "", fmt.Errorf("unterminated string")
			}
			p.pos += 2
			continue
		}
		if ch == '"' {
			raw := p.src[start : p.pos+1]
			p.pos++
			out, err := strconv.Unquote(raw)
			if err != nil {
				return "", err
			}
			return out, nil
		}
		p.pos++
	}
	return "", fmt.Errorf("unterminated string")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (p *hclLikeParser) parseNumber() (any, error) {
	start := p.pos
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if (ch >= '0' && ch <= '9') || ch == '-' || ch == '+' || ch == '.' || ch == 'e' || ch == 'E' {
			p.pos++
			continue
		}
		break
	}
	token := strings.TrimSpace(p.src[start:p.pos])
	if token == "" {
		return nil, fmt.Errorf("expected number")
	}
	if strings.ContainsAny(token, ".eE") {
		value, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
	value, err := strconv.ParseInt(token, 10, 64)
	if err == nil {
		return value, nil
	}
	valueFloat, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return nil, err
	}
	return valueFloat, nil
}

func (p *hclLikeParser) parseArray() ([]any, error) {
	if err := p.expectChar('['); err != nil {
		return nil, err
	}
	out := []any{}
	p.skipSpaceAndComments()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.pos++
		return out, nil
	}
	for {
		item, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, item)
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated array")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return out, nil
		default:
			return nil, fmt.Errorf("expected , or ] in array at %d", p.pos)
		}
	}
}

func (p *hclLikeParser) parseObject() (map[string]any, error) {
	if err := p.expectChar('{'); err != nil {
		return nil, err
	}
	out := map[string]any{}
	p.skipSpaceAndComments()
	if p.pos < len(p.src) && p.src[p.pos] == '}' {
		p.pos++
		return out, nil
	}
	for {
		p.skipSpaceAndComments()
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		if err := p.expectChar('='); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[key] = value
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated object")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return out, nil
		default:
			return nil, fmt.Errorf("expected , or } in object at %d", p.pos)
		}
	}
}

func normalizeBCLDocumentDefaults(doc *BCLDocument) {
	if doc.Resources == nil {
		doc.Resources = []BCLResource{}
	}
	if doc.Metadata == nil {
		doc.Metadata = map[string]any{}
	}
	if doc.Ext == nil {
		doc.Ext = map[string]any{}
	}
}

func renderHCLValue(value any, indent int) string {
	switch typed := value.(type) {
	case map[string]any:
		return renderHCLMap(typed, indent)
	case []any:
		return renderHCLList(typed, indent)
	case string:
		return strconv.Quote(typed)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return "null"
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return strconv.Quote(fmt.Sprintf("%v", typed))
	}
}

func renderHCLMap(raw map[string]any, indent int) string {
	out := strings.Builder{}
	out.WriteString("{\n")
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	indentStr := strings.Repeat("  ", indent)
	innerIndent := strings.Repeat("  ", indent+1)
	for _, key := range keys {
		if raw[key] == nil {
			continue
		}
		out.WriteString(innerIndent + strconv.Quote(key) + " = " + renderHCLValue(raw[key], indent+1) + "\n")
	}
	out.WriteString(indentStr + "}")
	return out.String()
}

func renderHCLList(items []any, indent int) string {
	if len(items) == 0 {
		return "[]"
	}
	out := strings.Builder{}
	out.WriteString("[\n")
	indentStr := strings.Repeat("  ", indent)
	innerIndent := strings.Repeat("  ", indent+1)
	for _, item := range items {
		out.WriteString(innerIndent + renderHCLValue(item, indent+1))
		out.WriteString(",\n")
	}
	out.WriteString(indentStr + "]")
	return out.String()
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
