package lifecycle

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-open-go-sdk/auth"
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
	// fileDigest returns a file's id and its Box-side SHA-1 content hash, so the
	// caller can tell an unchanged file from one that needs a new version.
	fileDigest(context.Context, string, string) (id, sha1 string, found bool, err error)
	uploadFile(context.Context, string, string) (string, error)
	// uploadFileVersion replaces an existing same-named file with a new version.
	uploadFileVersion(context.Context, string, string) (string, error)
	metadataTemplateDisplayNames(context.Context) (map[string]string, error)
	applyMetadataTemplate(context.Context, boxMetadataTemplate) error
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
	sdk, err := newBoxSDK()
	if err != nil {
		return nil, err
	}
	return sdk, nil
}

func newBoxSDK() (*boxSDK, error) {
	connection, err := boxconn.ResolveAuth()
	if err != nil {
		return nil, err
	}
	accessToken, err := connection.AccessToken(context.Background())
	if err != nil {
		if boxOAuthSessionExpired(err) {
			boxconn.InvalidateSelectedOAuthConnection()
			return nil, fmt.Errorf("Box OAuth session has expired. Return to Connect and reconnect the selected Box account: %w", err)
		}
		return nil, err
	}
	tokenSource, err := boxSDKTokenSource(connection, accessToken)
	if err != nil {
		return nil, err
	}
	return &boxSDK{client: boxclient.NewClient(tokenSource)}, nil
}

func boxSDKTokenSource(connection boxconn.AuthConfig, accessToken string) (gantryruntime.TokenSource, error) {
	switch connection.Method {
	case boxconn.AuthCCG:
		subjectID := connection.SubjectID
		ccgConfig := auth.CCGConfig{
			ClientID:     connection.ClientID,
			ClientSecret: connection.ClientSecret,
			UserID:       subjectID,
		}
		// If subject type is enterprise, set EnterpriseID instead of UserID
		if connection.SubjectType == "enterprise" {
			ccgConfig.EnterpriseID = subjectID
			ccgConfig.UserID = ""
		}
		return auth.ClientCredentials(ccgConfig), nil
	case boxconn.AuthOAuth2:
		// AccessToken already exchanged the single-use refresh token and persisted
		// Box's rotated replacement. Reusing OAuth here would consume that new
		// refresh token immediately and leave the persisted connection stale.
		return auth.DeveloperToken(accessToken), nil
	default:
		return nil, fmt.Errorf("unsupported Box authentication method %q", connection.Method)
	}
}

func boxOAuthSessionExpired(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_grant") || strings.Contains(message, "refresh token has expired")
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
	folder, err := sdk.client.Folders.Create(ctx, &schemas.CreateFolderRequest{
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

func (sdk *boxSDK) fileDigest(ctx context.Context, folderID, name string) (string, string, bool, error) {
	for item, err := range sdk.client.Folders.ListItems(ctx, folderID, nil) {
		if err != nil {
			return "", "", false, err
		}
		if item.File != nil && item.File.Name != nil && *item.File.Name == name {
			sha1 := ""
			if item.File.Sha1 != nil {
				sha1 = strings.ToLower(strings.TrimSpace(*item.File.Sha1))
			}
			return item.File.Id, sha1, true, nil
		}
	}
	return "", "", false, nil
}

func (sdk *boxSDK) uploadFile(ctx context.Context, folderID, source string) (string, error) {
	name := filepath.Base(source)
	file, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	files, err := sdk.client.Uploads.UploadFile(ctx, &schemas.CreateFileContentRequest{
		Attributes: schemas.PostFileContentAttributes{
			Name:   name,
			Parent: schemas.AttributesParent{Id: folderID},
		},
		File: file,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	return resolveUploadedFileID(ctx, files, name, 10, 250*time.Millisecond, func(ctx context.Context) (string, bool, error) {
		return sdk.findFile(ctx, folderID, name)
	})
}

func (sdk *boxSDK) uploadFileVersion(ctx context.Context, folderID, source string) (string, error) {
	// First find the existing file ID
	fileID, found, err := sdk.findFile(ctx, folderID, filepath.Base(source))
	if err != nil {
		return "", fmt.Errorf("find existing file: %w", err)
	}
	if !found {
		return "", fmt.Errorf("file %s not found in folder %s", filepath.Base(source), folderID)
	}

	name := filepath.Base(source)
	file, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	_, err = sdk.client.Uploads.UploadFileVersion(ctx, fileID, &schemas.CreateFileIdContentRequest{
		Attributes: schemas.PostFileIdContentAttributes{Name: name},
		File:       file,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("upload version of %s: %w", name, err)
	}
	return fileID, nil
}

func uploadedFileID(files *schemas.Files, name string) (string, error) {
	if files == nil || len(files.Entries) == 0 || strings.TrimSpace(files.Entries[0].Id) == "" {
		return "", fmt.Errorf("Box upload response for %s did not include a file ID", name)
	}
	return files.Entries[0].Id, nil
}

func resolveUploadedFileID(ctx context.Context, files *schemas.Files, name string, attempts int, delay time.Duration, lookup func(context.Context) (string, bool, error)) (string, error) {
	fileID, responseErr := uploadedFileID(files, name)
	if responseErr == nil {
		return fileID, nil
	}
	var lookupErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}
		fileID, found, err := lookup(ctx)
		if err != nil {
			lookupErr = err
			continue
		}
		if found && strings.TrimSpace(fileID) != "" {
			return fileID, nil
		}
	}
	if lookupErr != nil {
		return "", fmt.Errorf("%w; follow-up lookup failed: %v", responseErr, lookupErr)
	}
	return "", fmt.Errorf("%w; follow-up lookup did not find the uploaded file", responseErr)
}

func (sdk *boxSDK) metadataTemplateDisplayNames(ctx context.Context) (map[string]string, error) {
	templates := map[string]string{}
	for template, err := range sdk.client.MetadataTemplates.ListEnterprise(ctx, nil) {
		if err != nil {
			return nil, err
		}
		if template.TemplateKey != nil {
			templates[*template.TemplateKey] = firstNonEmpty(pointerString(template.DisplayName), *template.TemplateKey)
		}
	}
	return templates, nil
}

func (sdk *boxSDK) applyMetadataTemplate(ctx context.Context, template boxMetadataTemplate) error {
	existing, err := sdk.metadataTemplateDisplayNames(ctx)
	if err != nil {
		return err
	}
	if displayName, found := existing[template.TemplateKey]; found {
		if displayName == template.DisplayName {
			return nil
		}
		_, err = sdk.client.MetadataTemplates.UpdateSchema(ctx, schemas.GetFileIdMetadataIdScopeEnterprise, template.TemplateKey, []schemas.UpdateSchemaRequest{{
			Op:   schemas.PutIdSchemaOpEditTemplate,
			Data: map[string]any{"displayName": template.DisplayName},
		}})
		return err
	}

	fields := make([]schemas.PostSchemaFields, 0, len(template.Fields))
	for _, field := range template.Fields {
		fieldType := schemas.CreateSchemaRequestFieldsType(field.Type)
		switch fieldType {
		case schemas.CreateSchemaRequestFieldsTypeString, schemas.CreateSchemaRequestFieldsTypeFloat, schemas.CreateSchemaRequestFieldsTypeDate, schemas.CreateSchemaRequestFieldsTypeEnum, schemas.CreateSchemaRequestFieldsTypeMultiSelect:
		default:
			continue
		}
		options := make([]schemas.FieldsOptions, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, schemas.FieldsOptions{Key: option})
		}
		fields = append(fields, schemas.PostSchemaFields{Type: fieldType, Key: field.Key, DisplayName: field.DisplayName, Options: options})
	}
	_, err = sdk.client.MetadataTemplates.CreateSchema(ctx, &schemas.CreateSchemaRequest{
		Scope: "enterprise", TemplateKey: &template.TemplateKey, DisplayName: template.DisplayName, Fields: fields,
	})
	return err
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	return sdk.client.MetadataTemplates.DeleteSchema(ctx, schemas.GetFileIdMetadataIdScopeEnterprise, templateKey)
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

// isBoxPermissionError reports whether a Box API call was refused for access
// reasons, which callers degrade on rather than treating as a hard failure.
func isBoxPermissionError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "403") || strings.Contains(message, "401") || strings.Contains(message, "Forbidden") || strings.Contains(message, "Unauthorized")
}

// localFileSHA1 computes the hex SHA-1 of a local file, matching the hash Box
// stores server-side, so the two can be compared to detect content changes.
func localFileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
