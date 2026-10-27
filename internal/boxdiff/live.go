package boxdiff

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-open-go-sdk/auth"
	boxclient "github.com/unofficialbox/box-open-go-sdk/client"
	"github.com/unofficialbox/box-open-go-sdk/gantryruntime"
	"github.com/unofficialbox/box-open-go-sdk/managers"
	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

type capturedSnapshot struct {
	snapshot Snapshot
	err      error
}

type capturedFolderSnapshot struct {
	snapshot FolderSnapshot
	err      error
}

type capturedAIAgentSnapshot struct {
	snapshot AIAgentSnapshot
	err      error
}

// CompareLive queries both saved Box connections for every request. No
// persisted verification snapshot or prior comparison result is used.
func CompareLive(ctx context.Context, leftConnectionID, rightConnectionID string) (Comparison, error) {
	leftConnectionID = strings.TrimSpace(leftConnectionID)
	rightConnectionID = strings.TrimSpace(rightConnectionID)
	if leftConnectionID == "" || rightConnectionID == "" {
		return Comparison{}, fmt.Errorf("select two saved Box connections")
	}
	if leftConnectionID == rightConnectionID {
		return Comparison{}, fmt.Errorf("select two different Box connections")
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return Comparison{}, fmt.Errorf("load Box connections: %w", err)
	}
	settings = settings.HydrateBoxConnections()
	leftAlias := boxConnectionAlias(settings, leftConnectionID)
	rightAlias := boxConnectionAlias(settings, rightConnectionID)
	if leftAlias == "" || rightAlias == "" {
		return Comparison{}, fmt.Errorf("one or more saved Box connections were not found")
	}

	startedAt := time.Now()
	var wait sync.WaitGroup
	wait.Add(2)
	results := make([]capturedSnapshot, 2)
	for index, input := range []struct{ id, alias string }{{leftConnectionID, leftAlias}, {rightConnectionID, rightAlias}} {
		go func(index int, id, alias string) {
			defer wait.Done()
			results[index].snapshot, results[index].err = captureLive(ctx, id, alias)
		}(index, input.id, input.alias)
	}
	wait.Wait()
	if results[0].err != nil {
		return Comparison{}, fmt.Errorf("read left Box connection: %w", results[0].err)
	}
	if results[1].err != nil {
		return Comparison{}, fmt.Errorf("read right Box connection: %w", results[1].err)
	}
	return CompareSnapshots(startedAt, results[0].snapshot, results[1].snapshot, time.Now())
}

func captureLive(ctx context.Context, connectionID, alias string) (Snapshot, error) {
	environment, client, err := liveClient(ctx, connectionID, alias)
	if err != nil {
		return Snapshot{}, err
	}
	templates := make([]MetadataTemplate, 0)
	for template, listErr := range client.MetadataTemplates.ListEnterprise(ctx, nil) {
		if listErr != nil {
			return Snapshot{}, fmt.Errorf("list enterprise metadata templates: %w", listErr)
		}
		templates = append(templates, NormalizeTemplate(template))
	}
	sortTemplates(templates)
	return Snapshot{Environment: environment, Templates: templates}, nil
}

// CompareFoldersLive compares the connected users' current All Files folder
// structures. Box enterprises do not have one global folder root, so this
// intentionally reflects the content visible to each saved connection.
func CompareFoldersLive(ctx context.Context, leftConnectionID, rightConnectionID string) (FolderComparison, error) {
	leftConnectionID = strings.TrimSpace(leftConnectionID)
	rightConnectionID = strings.TrimSpace(rightConnectionID)
	if leftConnectionID == "" || rightConnectionID == "" {
		return FolderComparison{}, fmt.Errorf("select two saved Box connections")
	}
	if leftConnectionID == rightConnectionID {
		return FolderComparison{}, fmt.Errorf("select two different Box connections")
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return FolderComparison{}, fmt.Errorf("load Box connections: %w", err)
	}
	settings = settings.HydrateBoxConnections()
	leftAlias := boxConnectionAlias(settings, leftConnectionID)
	rightAlias := boxConnectionAlias(settings, rightConnectionID)
	if leftAlias == "" || rightAlias == "" {
		return FolderComparison{}, fmt.Errorf("one or more saved Box connections were not found")
	}

	startedAt := time.Now()
	var wait sync.WaitGroup
	wait.Add(2)
	results := make([]capturedFolderSnapshot, 2)
	for index, input := range []struct{ id, alias string }{{leftConnectionID, leftAlias}, {rightConnectionID, rightAlias}} {
		go func(index int, id, alias string) {
			defer wait.Done()
			results[index].snapshot, results[index].err = captureFoldersLive(ctx, id, alias)
		}(index, input.id, input.alias)
	}
	wait.Wait()
	if results[0].err != nil {
		return FolderComparison{}, fmt.Errorf("read left Box connection: %w", results[0].err)
	}
	if results[1].err != nil {
		return FolderComparison{}, fmt.Errorf("read right Box connection: %w", results[1].err)
	}
	return CompareFolderSnapshots(startedAt, results[0].snapshot, results[1].snapshot, time.Now())
}

func captureFoldersLive(ctx context.Context, connectionID, alias string) (FolderSnapshot, error) {
	environment, client, err := liveClient(ctx, connectionID, alias)
	if err != nil {
		return FolderSnapshot{}, err
	}
	useMarker := true
	limit := int64(1000)
	options := &managers.FoldersListItemsOptions{Fields: []string{"id", "name", "type"}, Usemarker: &useMarker, Limit: &limit}
	list := func(ctx context.Context, folderID string) iter.Seq2[*schemas.Item, error] {
		return client.Folders.ListItems(ctx, folderID, options)
	}
	folders, truncated, err := crawlFolderTree(ctx, list, FolderComparisonMaxDepth, FolderComparisonMaxFolders)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("list live folder tree: %w", err)
	}
	return FolderSnapshot{Environment: environment, Folders: folders, Truncated: truncated}, nil
}

// CompareAIAgentsLive compares the current custom AI Studio agents visible to
// two saved Box connections. These are distinct from Box Automate Extract
// Agents, which are not exposed by this API.
func CompareAIAgentsLive(ctx context.Context, leftConnectionID, rightConnectionID string) (AIAgentComparison, error) {
	leftConnectionID = strings.TrimSpace(leftConnectionID)
	rightConnectionID = strings.TrimSpace(rightConnectionID)
	if leftConnectionID == "" || rightConnectionID == "" {
		return AIAgentComparison{}, fmt.Errorf("select two saved Box connections")
	}
	if leftConnectionID == rightConnectionID {
		return AIAgentComparison{}, fmt.Errorf("select two different Box connections")
	}
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil {
		return AIAgentComparison{}, fmt.Errorf("load Box connections: %w", err)
	}
	settings = settings.HydrateBoxConnections()
	leftAlias := boxConnectionAlias(settings, leftConnectionID)
	rightAlias := boxConnectionAlias(settings, rightConnectionID)
	if leftAlias == "" || rightAlias == "" {
		return AIAgentComparison{}, fmt.Errorf("one or more saved Box connections were not found")
	}

	startedAt := time.Now()
	var wait sync.WaitGroup
	wait.Add(2)
	results := make([]capturedAIAgentSnapshot, 2)
	for index, input := range []struct{ id, alias string }{{leftConnectionID, leftAlias}, {rightConnectionID, rightAlias}} {
		go func(index int, id, alias string) {
			defer wait.Done()
			results[index].snapshot, results[index].err = captureAIAgentsLive(ctx, id, alias)
		}(index, input.id, input.alias)
	}
	wait.Wait()
	if results[0].err != nil {
		return AIAgentComparison{}, fmt.Errorf("read left Box connection: %w", results[0].err)
	}
	if results[1].err != nil {
		return AIAgentComparison{}, fmt.Errorf("read right Box connection: %w", results[1].err)
	}
	return CompareAIAgentSnapshots(startedAt, results[0].snapshot, results[1].snapshot, time.Now())
}

func captureAIAgentsLive(ctx context.Context, connectionID, alias string) (AIAgentSnapshot, error) {
	environment, _, accessToken, err := liveClientWithToken(ctx, connectionID, alias)
	if err != nil {
		return AIAgentSnapshot{}, err
	}
	agents, err := listAIAgentsFromSession(ctx, gantryruntime.New(auth.DeveloperToken(accessToken)))
	if err != nil {
		return AIAgentSnapshot{}, fmt.Errorf("list AI Studio agents: %w", err)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Key < agents[j].Key })
	return AIAgentSnapshot{Environment: environment, Agents: agents}, nil
}

func listAIAgentsFromSession(ctx context.Context, session *gantryruntime.Client) ([]AIAgent, error) {
	fields := []string{"id", "type", "origin", "name", "access_state", "allowed_entities", "ask", "text_gen", "extract"}
	agents := make([]AIAgent, 0)
	marker := ""
	for {
		request := session.NewRequest(http.MethodGet, session.BaseUrl("api")+"/ai_agents")
		request = gantryruntime.WithQuery(request, "fields", strings.Join(fields, ","))
		request = gantryruntime.WithQuery(request, "include_box_default", "false")
		if marker != "" {
			request = gantryruntime.WithQuery(request, "marker", marker)
		}
		response, err := session.Fetch(ctx, request)
		if err != nil {
			return nil, err
		}
		body, err := gantryruntime.ResponseBytes(response)
		if err != nil {
			return nil, err
		}
		status := gantryruntime.StatusCode(response)
		if status < 200 || status >= 300 {
			return nil, aiAgentResponseError(status, body)
		}
		var page schemas.AiMultipleAgentResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode Box response: %w", err)
		}
		for index := range page.Entries {
			agents = append(agents, NormalizeAIAgent(&page.Entries[index]))
		}
		if page.NextMarker == nil || !page.NextMarker.Valid || page.NextMarker.Value == "" {
			return agents, nil
		}
		marker = page.NextMarker.Value
	}
}

func aiAgentResponseError(status int64, body []byte) error {
	var detail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &detail)
	detail.Code = strings.TrimSpace(detail.Code)
	detail.Message = strings.TrimSpace(detail.Message)
	switch {
	case detail.Code == "insufficient_scope" && detail.Message != "":
		return fmt.Errorf("Box API returned %d %s: %s Update the Box application's scopes, reauthorize it in Box, and reconnect this account", status, detail.Code, detail.Message)
	case detail.Code != "" && detail.Message != "":
		return fmt.Errorf("Box API returned %d %s: %s", status, detail.Code, detail.Message)
	case detail.Code != "":
		return fmt.Errorf("Box API returned %d %s", status, detail.Code)
	default:
		return fmt.Errorf("Box API returned status %d", status)
	}
}

type folderListFunc func(context.Context, string) iter.Seq2[*schemas.Item, error]

func crawlFolderTree(ctx context.Context, list folderListFunc, maxDepth, maxFolders int) ([]Folder, bool, error) {
	type pendingFolder struct {
		id, path string
		depth    int
		index    int
	}
	type childFolder struct{ id, name string }
	type listedFolder struct {
		counts   FolderItemCounts
		children []childFolder
		err      error
	}
	level := []pendingFolder{{id: "0", depth: 0, index: -1}}
	folders := make([]Folder, 0)
	truncated := false
	for len(level) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		results := make([]listedFolder, len(level))
		semaphore := make(chan struct{}, 8)
		var wait sync.WaitGroup
		wait.Add(len(level))
		for index, current := range level {
			go func(index int, current pendingFolder) {
				defer wait.Done()
				select {
				case semaphore <- struct{}{}:
					defer func() { <-semaphore }()
				case <-ctx.Done():
					results[index].err = ctx.Err()
					return
				}
				for item, listErr := range list(ctx, current.id) {
					if listErr != nil {
						results[index].err = listErr
						return
					}
					switch {
					case item == nil:
						results[index].counts.Other++
					case item.Folder != nil:
						results[index].counts.Folders++
						name := strings.TrimSpace(stringValue(item.Folder.Name))
						if name == "" {
							results[index].err = fmt.Errorf("folder listing returned an unnamed folder")
							return
						}
						results[index].children = append(results[index].children, childFolder{id: item.Folder.Id, name: name})
					case item.File != nil:
						results[index].counts.Files++
					case item.WebLink != nil:
						results[index].counts.WebLinks++
					default:
						results[index].counts.Other++
					}
				}
			}(index, current)
		}
		wait.Wait()

		next := make([]pendingFolder, 0)
		for index, current := range level {
			result := results[index]
			if result.err != nil {
				return nil, false, result.err
			}
			if current.index >= 0 {
				folders[current.index].Items = result.counts
			}
			if current.depth >= maxDepth {
				truncated = truncated || len(result.children) > 0
				continue
			}
			sort.Slice(result.children, func(i, j int) bool { return result.children[i].name < result.children[j].name })
			for _, child := range result.children {
				if len(folders) >= maxFolders {
					truncated = true
					continue
				}
				path := current.path + "/" + child.name
				folders = append(folders, Folder{Path: path, Name: child.name, Depth: current.depth + 1})
				next = append(next, pendingFolder{id: child.id, path: path, depth: current.depth + 1, index: len(folders) - 1})
			}
		}
		level = next
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	return folders, truncated, nil
}

func liveClient(ctx context.Context, connectionID, alias string) (Environment, *boxclient.Client, error) {
	environment, client, _, err := liveClientWithToken(ctx, connectionID, alias)
	return environment, client, err
}

func liveClientWithToken(ctx context.Context, connectionID, alias string) (Environment, *boxclient.Client, string, error) {
	connection, err := boxconn.ResolveAuthForConnection(connectionID)
	if err != nil {
		return Environment{}, nil, "", err
	}
	accessToken, err := connection.AccessToken(ctx)
	if err != nil {
		return Environment{}, nil, "", fmt.Errorf("authenticate: %w", err)
	}
	client := boxclient.NewClient(auth.DeveloperToken(accessToken))
	user, err := client.Users.GetMe(ctx, &managers.UsersGetMeOptions{Fields: []string{"id", "login", "name", "enterprise", "hostname"}})
	if err != nil {
		return Environment{}, nil, "", fmt.Errorf("read live identity: %w", err)
	}
	environment := Environment{ConnectionID: connectionID, Alias: alias, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Identity: stringValue(user.Login), Domain: stringValue(user.Hostname)}
	if environment.Identity == "" {
		environment.Identity = stringValue(user.Name)
	}
	if user.Enterprise != nil {
		environment.EnterpriseID = stringValue(user.Enterprise.Id)
	}
	if environment.EnterpriseID == "" {
		return Environment{}, nil, "", fmt.Errorf("live identity did not include an enterprise ID")
	}
	return environment, client, accessToken, nil
}

func sortTemplates(templates []MetadataTemplate) {
	sort.Slice(templates, func(i, j int) bool { return templates[i].Key < templates[j].Key })
}

func boxConnectionAlias(settings config.ConnectionSettings, connectionID string) string {
	for _, connection := range settings.BoxConnections {
		if connection.ID == connectionID {
			return firstNonEmpty(connection.Alias, connection.Identity, connection.ID)
		}
	}
	return ""
}
