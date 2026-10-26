package boxdiff

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-open-go-sdk/auth"
	boxclient "github.com/unofficialbox/box-open-go-sdk/client"
	"github.com/unofficialbox/box-open-go-sdk/managers"
)

type capturedSnapshot struct {
	snapshot Snapshot
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
	connection, err := boxconn.ResolveAuthForConnection(connectionID)
	if err != nil {
		return Snapshot{}, err
	}
	accessToken, err := connection.AccessToken(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("authenticate: %w", err)
	}
	client := boxclient.NewClient(auth.DeveloperToken(accessToken))
	user, err := client.Users.GetMe(ctx, &managers.UsersGetMeOptions{Fields: []string{"id", "login", "name", "enterprise", "hostname"}})
	if err != nil {
		return Snapshot{}, fmt.Errorf("read live identity: %w", err)
	}
	environment := Environment{ConnectionID: connectionID, Alias: alias, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Identity: stringValue(user.Login), Domain: stringValue(user.Hostname)}
	if environment.Identity == "" {
		environment.Identity = stringValue(user.Name)
	}
	if user.Enterprise != nil {
		environment.EnterpriseID = stringValue(user.Enterprise.Id)
	}
	if environment.EnterpriseID == "" {
		return Snapshot{}, fmt.Errorf("live identity did not include an enterprise ID")
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
