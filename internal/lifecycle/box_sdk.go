package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	boxclient "github.com/unofficialbox/box-open-go-sdk/client"
	"github.com/unofficialbox/box-open-go-sdk/gantryruntime"
	"github.com/unofficialbox/box-open-go-sdk/managers"
	"github.com/unofficialbox/box-open-go-sdk/schemas"
	"github.com/unofficialbox/box-open-go-sdk/serialization"
)

type boxSDK struct {
	client *boxclient.Client
}

type boxAPI interface {
	findFolder(context.Context, string, string) (string, bool, error)
	ensureFolder(context.Context, string, string) (string, error)
	findFile(context.Context, string, string) (string, bool, error)
	fileExists(context.Context, string, string) (bool, error)
	uploadFile(context.Context, string, string) error
	metadataTemplateKeys(context.Context, []string) (map[string]bool, error)
	createMetadataTemplate(context.Context, boxMetadataTemplate) error
	docgenTemplateFileIDs(context.Context) (map[string]bool, error)
	createDocgenTemplate(context.Context, string) error
	aiAgentNames(context.Context) (map[string]bool, error)
	// createAIAgent and createHub return the created resource ID so the deploy
	// can record it; teardown deletes strictly by recorded ID.
	createAIAgent(context.Context, string, string, string, string) (string, error)
	hubTitles(context.Context) (map[string]bool, error)
	createHub(context.Context, string, string) (string, error)
	automateWorkflowNames(context.Context, string) (map[string]bool, error)

	// Teardown operations. Each deletes a single resource by ID; a missing
	// resource is reported by the caller rather than treated as fatal.
	deleteFolder(context.Context, string) error
	deleteFile(context.Context, string) error
	deleteMetadataTemplate(context.Context, string) error
	deleteDocgenTemplate(context.Context, string) error
	deleteAIAgent(context.Context, string) error
	deleteHub(context.Context, string) error
}

func newBoxAPI() (boxAPI, error) {
	if sdk, err := newBoxSDK(); err == nil {
		return sdk, nil
	}
	return boxCLI{}, nil
}

func newBoxSDK() (*boxSDK, error) {
	// A captured Client Credentials Grant app, used as a user, is preferred: it
	// carries the enterprise scopes (e.g. Doc Gen) the CLI's OAuth token lacks,
	// while the resources it creates stay owned by that user.
	if settings, err := shellstate.LoadConnectionSettings(); err == nil && settings.HasBoxCCG() {
		token, tokenErr := boxCCGToken(context.Background(), settings.BoxCCGClientID, settings.BoxCCGClientSecret, settings.BoxCCGSubjectType, settings.BoxCCGSubjectID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		return &boxSDK{client: boxclient.NewClient(gantryruntime.DeveloperToken(token))}, nil
	}

	identityOutput, err := exec.Command("box", "users:get", "me", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("resolve the authenticated Box user")
	}
	userID := boxUserID(identityOutput)
	if userID == "" {
		return nil, fmt.Errorf("Box CLI returned an unreadable user ID")
	}
	output, err := exec.Command("box", "tokens:get", "--user-id="+userID).Output()
	if err != nil {
		return nil, fmt.Errorf("get a user access token from the authenticated Box CLI session")
	}
	token := boxAccessToken(output)
	if token == "" {
		return nil, fmt.Errorf("Box CLI returned an unreadable access token")
	}
	return &boxSDK{client: boxclient.NewClient(gantryruntime.DeveloperToken(token))}, nil
}

var boxTokenURL = "https://api.box.com/oauth2/token"

// boxCCGToken mints a Client Credentials Grant access token for the given
// subject. subjectType "user" makes the token act as that user, so objects it
// creates are owned by the user; "enterprise" acts as the service account. Both
// carry the CCG app's enterprise scopes.
func boxCCGToken(ctx context.Context, clientID, clientSecret, subjectType, subjectID string) (string, error) {
	form := url.Values{
		"grant_type":       {"client_credentials"},
		"client_id":        {clientID},
		"client_secret":    {clientSecret},
		"box_subject_type": {subjectType},
		"box_subject_id":   {subjectID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, boxTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request Box CCG token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Box CCG token request returned %s: %s", resp.Status, boxTokenError(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse Box CCG token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("Box CCG token response contained no access token")
	}
	return payload.AccessToken, nil
}

// boxTokenError extracts the human-readable message from a Box OAuth error body.
func boxTokenError(body []byte) string {
	var payload struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != "" {
		if payload.Description != "" {
			return payload.Error + ": " + payload.Description
		}
		return payload.Error
	}
	return strings.TrimSpace(string(body))
}

func boxUserID(output []byte) string {
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(output, &identity) == nil {
		return strings.TrimSpace(identity.ID)
	}
	return ""
}

func boxAccessToken(output []byte) string {
	var value any
	if json.Unmarshal(output, &value) == nil {
		if token := findTokenValue(value); token != "" {
			return token
		}
	}
	plain := strings.TrimSpace(string(output))
	if len(plain) >= 20 && !strings.ContainsAny(plain, " \t\r\n:") {
		return plain
	}
	return ""
}

func findTokenValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"accessToken", "access_token", "token"} {
			if token, ok := typed[key].(string); ok && strings.TrimSpace(token) != "" {
				return strings.TrimSpace(token)
			}
		}
		for _, child := range typed {
			if token := findTokenValue(child); token != "" {
				return token
			}
		}
	case []any:
		for _, child := range typed {
			if token := findTokenValue(child); token != "" {
				return token
			}
		}
	}
	return ""
}

func (sdk *boxSDK) findFolder(ctx context.Context, parentID, name string) (string, bool, error) {
	for item, err := range sdk.client.Folders.ListItems(ctx, parentID, nil) {
		if err != nil {
			return "", false, err
		}
		if item.Folder != nil && item.Folder.Name != nil && *item.Folder.Name == name {
			return item.Folder.Id, true, nil
		}
	}
	return "", false, nil
}

func (sdk *boxSDK) ensureFolder(ctx context.Context, parentID, name string) (string, error) {
	if id, found, err := sdk.findFolder(ctx, parentID, name); err != nil {
		return "", err
	} else if found {
		return id, nil
	}
	folder, err := sdk.client.Folders.Create(ctx, &schemas.FolderCreateRequest{
		Name:   name,
		Parent: schemas.AttributesParent{Id: parentID},
	}, nil)
	if err != nil {
		return "", err
	}
	return folder.Id, nil
}

func (sdk *boxSDK) findFile(ctx context.Context, folderID, name string) (string, bool, error) {
	for item, err := range sdk.client.Folders.ListItems(ctx, folderID, nil) {
		if err != nil {
			return "", false, err
		}
		if item.File != nil && item.File.Name != nil && *item.File.Name == name {
			return item.File.Id, true, nil
		}
	}
	return "", false, nil
}

func (sdk *boxSDK) fileExists(ctx context.Context, folderID, name string) (bool, error) {
	_, found, err := sdk.findFile(ctx, folderID, name)
	return found, err
}

func (sdk *boxSDK) uploadFile(_ context.Context, folderID, source string) error {
	return uploadFileWithCLI(folderID, source)
}

func (sdk *boxSDK) metadataTemplateKeys(ctx context.Context, _ []string) (map[string]bool, error) {
	keys := map[string]bool{}
	for template, err := range sdk.client.MetadataTemplates.ListEnterprise(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if template.TemplateKey != nil {
			keys[*template.TemplateKey] = true
		}
	}
	return keys, nil
}

type boxCLI struct{}

func advancedBoxAPIUnavailable(operation string) error {
	return fmt.Errorf("%s requires the Box SDK token bridge; run box login and retry", operation)
}

func boxRequest(ctx context.Context, method, resource string, body any, headers ...string) ([]byte, error) {
	args := []string{"request", resource, "--method", method, "--json", "--no-color"}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		args = append(args, "--body", string(encoded))
	}
	for _, header := range headers {
		args = append(args, "--header", header)
	}
	output, err := exec.CommandContext(ctx, "box", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Box API %s %s: %s", method, resource, summarizeCommandOutput(output, err))
	}
	return boxRequestBody(output, method, resource)
}

// isBoxPermissionError reports whether a Box API call was refused for access
// reasons, which callers degrade on rather than treating as a hard failure.
func isBoxPermissionError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "returned 403") || strings.Contains(message, "returned 401")
}

// boxRequestBody unwraps the Box CLI's response envelope. `box request` emits
// {"statusCode":…,"headers":…,"body":…} and every caller wants the body, so
// returning the envelope made each parser read the wrong level: IDs came back
// empty and existing objects were reported as absent, which re-created them.
// A non-2xx status is surfaced as an error rather than parsed as a result.
func boxRequestBody(output []byte, method, resource string) ([]byte, error) {
	var envelope struct {
		StatusCode int             `json:"statusCode"`
		Body       json.RawMessage `json:"body"`
	}
	if json.Unmarshal(output, &envelope) != nil || envelope.StatusCode == 0 {
		// Not an envelope; hand back whatever the CLI produced.
		return output, nil
	}
	if envelope.StatusCode >= 300 {
		return nil, fmt.Errorf("Box API %s %s returned %d: %s", method, resource, envelope.StatusCode, strings.TrimSpace(string(envelope.Body)))
	}
	if len(envelope.Body) == 0 {
		return output, nil
	}
	return envelope.Body, nil
}

func (boxCLI) findFolder(_ context.Context, parentID, name string) (string, bool, error) {
	output, err := exec.Command("box", "folders:items", parentID, "--fields", "id,type,name", "--max-items", "1000", "--json").CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("list folder %s: %s", parentID, summarizeCommandOutput(output, err))
	}
	type entry struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	var entries []entry
	if json.Unmarshal(output, &entries) != nil {
		var collection struct {
			Entries []entry `json:"entries"`
		}
		if err := json.Unmarshal(output, &collection); err != nil {
			return "", false, fmt.Errorf("parse Box folder listing: %w", err)
		}
		entries = collection.Entries
	}
	for _, entry := range entries {
		if entry.Type == "folder" && entry.Name == name {
			return entry.ID, true, nil
		}
	}
	return "", false, nil
}

func (cli boxCLI) ensureFolder(ctx context.Context, parentID, name string) (string, error) {
	if id, found, err := cli.findFolder(ctx, parentID, name); err != nil {
		return "", err
	} else if found {
		return id, nil
	}
	output, err := exec.Command("box", "folders:create", parentID, name, "--json", "--yes").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("create Box folder %q: %s", name, summarizeCommandOutput(output, err))
	}
	var folder struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output, &folder); err != nil || folder.ID == "" {
		return "", fmt.Errorf("parse created Box folder %q", name)
	}
	return folder.ID, nil
}

func (boxCLI) findFile(_ context.Context, folderID, name string) (string, bool, error) {
	output, err := exec.Command("box", "folders:items", folderID, "--fields", "id,type,name", "--max-items", "1000", "--json").CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("list folder %s: %s", folderID, summarizeCommandOutput(output, err))
	}
	type entry struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	var entries []entry
	if json.Unmarshal(output, &entries) != nil {
		var collection struct {
			Entries []entry `json:"entries"`
		}
		if err := json.Unmarshal(output, &collection); err != nil {
			return "", false, fmt.Errorf("parse Box folder listing: %w", err)
		}
		entries = collection.Entries
	}
	for _, entry := range entries {
		if entry.Type == "file" && entry.Name == name {
			return entry.ID, true, nil
		}
	}
	return "", false, nil
}

func (cli boxCLI) fileExists(ctx context.Context, folderID, name string) (bool, error) {
	_, found, err := cli.findFile(ctx, folderID, name)
	return found, err
}

func (boxCLI) uploadFile(_ context.Context, folderID, source string) error {
	return uploadFileWithCLI(folderID, source)
}

func uploadFileWithCLI(folderID, source string) error {
	output, err := exec.Command("box", "files:upload", source, "--parent-id", folderID, "--json", "--yes").CombinedOutput()
	if err != nil {
		return fmt.Errorf("upload %s: %s", filepath.Base(source), summarizeCommandOutput(output, err))
	}
	return nil
}

func (boxCLI) metadataTemplateKeys(_ context.Context, requested []string) (map[string]bool, error) {
	keys := map[string]bool{}
	for _, key := range requested {
		cmd := exec.Command("box", "metadata-templates:get", key, "--scope", "enterprise", "--json")
		if err := cmd.Run(); err == nil {
			keys[key] = true
		}
	}
	return keys, nil
}

func (boxCLI) createMetadataTemplate(_ context.Context, template boxMetadataTemplate) error {
	command := []string{"box", "metadata-templates:create", "--display-name", template.DisplayName, "--template-key", template.TemplateKey, "--json", "--yes"}
	for _, field := range template.Fields {
		flag := map[string]string{"string": "--string", "enum": "--enum", "date": "--date", "float": "--number", "number": "--number"}[strings.ToLower(field.Type)]
		if flag == "" {
			continue
		}
		command = append(command, flag, field.DisplayName, "--field-key", field.Key)
		for _, option := range field.Options {
			command = append(command, "--option", option)
		}
	}
	output, err := exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", summarizeCommandOutput(output, err))
	}
	return nil
}

func (sdk *boxSDK) createMetadataTemplate(ctx context.Context, template boxMetadataTemplate) error {
	fields := make([]schemas.PostSchemaFields, 0, len(template.Fields))
	for _, field := range template.Fields {
		fieldType := schemas.FieldsType3(field.Type)
		switch fieldType {
		case schemas.FieldsType3String, schemas.FieldsType3Float, schemas.FieldsType3Date, schemas.FieldsType3Enum, schemas.FieldsType3MultiSelect:
		default:
			continue
		}
		options := make([]schemas.FieldsOptions, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, schemas.FieldsOptions{Key: option})
		}
		fields = append(fields, schemas.PostSchemaFields{Type: fieldType, Key: field.Key, DisplayName: field.DisplayName, Options: options})
	}
	_, err := sdk.client.MetadataTemplates.CreateSchema(ctx, &schemas.SchemaCreateRequest{
		Scope: "enterprise", TemplateKey: &template.TemplateKey, DisplayName: template.DisplayName, Fields: fields,
	})
	return err
}

func (sdk *boxSDK) docgenTemplateFileIDs(ctx context.Context) (map[string]bool, error) {
	ids := map[string]bool{}
	for template, err := range sdk.client.DocgenTemplate.ListDocgenTemplates(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if template.File != nil {
			ids[template.File.Id] = true
		}
	}
	return ids, nil
}

func (sdk *boxSDK) createDocgenTemplate(ctx context.Context, fileID string) error {
	_, err := sdk.client.DocgenTemplate.CreateDocgenTemplates(ctx, &schemas.DocGenTemplateCreateRequest{File: schemas.FileReference{Type: schemas.FileReferenceTypeFile, Id: fileID}})
	return err
}

func (sdk *boxSDK) aiAgentNames(ctx context.Context) (map[string]bool, error) {
	names := map[string]bool{}
	for agent, err := range sdk.client.AiStudio.ListAiAgents(ctx, nil) {
		if err != nil {
			return nil, err
		}
		names[agent.Name] = true
	}
	return names, nil
}

func (sdk *boxSDK) createAIAgent(ctx context.Context, mode, name, description, instructions string) (string, error) {
	body := &schemas.CreateAiAgent{Type: schemas.AiSingleAgentResponseTypeAiAgent, Name: name, AccessState: "enabled"}
	if mode == "extract" {
		body.Extract = &schemas.AiStudioAgentExtract{Type: schemas.AiAgentExtractTypeAiAgentExtract, AccessState: "enabled", Description: description, CustomInstructions: serialization.Value(instructions)}
	} else {
		body.Ask = &schemas.AiStudioAgentAsk{Type: schemas.AiAgentAskTypeAiAgentAsk, AccessState: "enabled", Description: description, CustomInstructions: serialization.Value(instructions)}
	}
	agent, err := sdk.client.AiStudio.CreateAiAgents(ctx, body)
	if err != nil || agent == nil {
		return "", err
	}
	return agent.Id, nil
}

func (sdk *boxSDK) hubTitles(ctx context.Context) (map[string]bool, error) {
	titles := map[string]bool{}
	for hub, err := range sdk.client.Hubs.List(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if hub.Title != nil {
			titles[*hub.Title] = true
		}
	}
	return titles, nil
}

func (sdk *boxSDK) createHub(ctx context.Context, title, description string) (string, error) {
	hub, err := sdk.client.Hubs.Create(ctx, &schemas.HubCreateRequest{Title: title, Description: &description})
	if err != nil || hub == nil {
		return "", err
	}
	return hub.Id, nil
}

func (sdk *boxSDK) deleteFolder(ctx context.Context, folderID string) error {
	// Recursive so a workspace folder deletes with its contents.
	recursive := true
	return sdk.client.Folders.Delete(ctx, folderID, &managers.FoldersDeleteOptions{Recursive: &recursive})
}

func (sdk *boxSDK) deleteFile(ctx context.Context, fileID string) error {
	return sdk.client.Files.Delete(ctx, fileID, nil)
}

func (sdk *boxSDK) deleteMetadataTemplate(ctx context.Context, templateKey string) error {
	return sdk.client.MetadataTemplates.DeleteSchema(ctx, schemas.GetFileIdMetadataIdIdScopeEnterprise, templateKey)
}

// deleteDocgenTemplate takes the template's file ID, which is what
// docgenTemplateFileIDs collects and what the deploy records.
func (sdk *boxSDK) deleteDocgenTemplate(ctx context.Context, templateID string) error {
	return sdk.client.DocgenTemplate.DeleteDocgenTemplate(ctx, templateID)
}

func (sdk *boxSDK) deleteAIAgent(ctx context.Context, agentID string) error {
	return sdk.client.AiStudio.DeleteAiAgent(ctx, agentID)
}

func (sdk *boxSDK) deleteHub(ctx context.Context, hubID string) error {
	return sdk.client.Hubs.Delete(ctx, hubID)
}

func (sdk *boxSDK) automateWorkflowNames(ctx context.Context, folderID string) (map[string]bool, error) {
	names := map[string]bool{}
	for action, err := range sdk.client.AutomateWorkflows.List(ctx, folderID, nil) {
		if err != nil {
			return nil, err
		}
		if action.Workflow.Name != nil {
			names[*action.Workflow.Name] = true
		}
	}
	return names, nil
}

func (boxCLI) docgenTemplateFileIDs(ctx context.Context) (map[string]bool, error) {
	output, err := boxRequest(ctx, "GET", "/docgen_templates?limit=1000", nil, "box-version: 2025.0")
	if err != nil {
		return nil, err
	}
	var response struct {
		Entries []struct {
			File *struct {
				ID string `json:"id"`
			} `json:"file"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse Box Doc Gen templates: %w", err)
	}
	ids := map[string]bool{}
	for _, entry := range response.Entries {
		if entry.File != nil {
			ids[entry.File.ID] = true
		}
	}
	return ids, nil
}

func (boxCLI) createDocgenTemplate(ctx context.Context, fileID string) error {
	_, err := boxRequest(ctx, "POST", "/docgen_templates", map[string]any{"file": map[string]string{"type": "file", "id": fileID}}, "box-version: 2025.0")
	return err
}

func (boxCLI) aiAgentNames(ctx context.Context) (map[string]bool, error) {
	output, err := boxRequest(ctx, "GET", "/ai_agents?limit=1000", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse Box AI Studio agents: %w", err)
	}
	names := map[string]bool{}
	for _, entry := range response.Entries {
		names[entry.Name] = true
	}
	return names, nil
}

func (boxCLI) createAIAgent(ctx context.Context, mode, name, description, instructions string) (string, error) {
	capability := map[string]any{"type": "ai_agent_ask", "access_state": "enabled", "description": description, "custom_instructions": instructions}
	body := map[string]any{"type": "ai_agent", "name": name, "access_state": "enabled", "ask": capability}
	if mode == "extract" {
		capability["type"] = "ai_agent_extract"
		delete(body, "ask")
		body["extract"] = capability
	}
	output, err := boxRequest(ctx, "POST", "/ai_agents", body)
	if err != nil {
		return "", err
	}
	return boxResourceID(output), nil
}

func (boxCLI) hubTitles(ctx context.Context) (map[string]bool, error) {
	output, err := boxRequest(ctx, "GET", "/hubs?limit=1000", nil, "box-version: 2025.0")
	if err != nil {
		return nil, err
	}
	var response struct {
		Entries []struct {
			Title string `json:"title"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse Box Hubs: %w", err)
	}
	titles := map[string]bool{}
	for _, entry := range response.Entries {
		titles[entry.Title] = true
	}
	return titles, nil
}

func (boxCLI) createHub(ctx context.Context, title, description string) (string, error) {
	output, err := boxRequest(ctx, "POST", "/hubs", map[string]string{"title": title, "description": description}, "box-version: 2025.0")
	if err != nil {
		return "", err
	}
	return boxResourceID(output), nil
}

func (boxCLI) deleteFolder(ctx context.Context, folderID string) error {
	_, err := boxRequest(ctx, "DELETE", "/folders/"+folderID+"?recursive=true", nil)
	return err
}

func (boxCLI) deleteFile(ctx context.Context, fileID string) error {
	_, err := boxRequest(ctx, "DELETE", "/files/"+fileID, nil)
	return err
}

func (boxCLI) deleteMetadataTemplate(ctx context.Context, templateKey string) error {
	_, err := boxRequest(ctx, "DELETE", "/metadata_templates/enterprise/"+templateKey+"/schema", nil)
	return err
}

func (boxCLI) deleteDocgenTemplate(ctx context.Context, templateID string) error {
	_, err := boxRequest(ctx, "DELETE", "/docgen_templates/"+templateID, nil, "box-version: 2025.0")
	return err
}

func (boxCLI) deleteAIAgent(ctx context.Context, agentID string) error {
	_, err := boxRequest(ctx, "DELETE", "/ai_agents/"+agentID, nil)
	return err
}

func (boxCLI) deleteHub(ctx context.Context, hubID string) error {
	_, err := boxRequest(ctx, "DELETE", "/hubs/"+hubID, nil, "box-version: 2025.0")
	return err
}

// boxResourceID pulls the id field out of a Box API create response.
func boxResourceID(output []byte) string {
	var response struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(output, &response) != nil {
		return ""
	}
	return strings.TrimSpace(response.ID)
}

func (boxCLI) automateWorkflowNames(ctx context.Context, folderID string) (map[string]bool, error) {
	output, err := boxRequest(ctx, "GET", "/automate_workflows?folder_id="+folderID+"&limit=1000", nil, "box-version: 2026.0")
	if err != nil {
		return nil, err
	}
	var response struct {
		Entries []struct {
			Workflow struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse Box Automate workflows: %w", err)
	}
	names := map[string]bool{}
	for _, entry := range response.Entries {
		names[entry.Workflow.Name] = true
	}
	return names, nil
}
