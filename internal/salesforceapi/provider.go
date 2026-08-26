package salesforceapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type InstalledPackage struct {
	PackageID     string
	Name          string
	Namespace     string
	VersionID     string
	VersionName   string
	VersionNumber string
}

// PackageInstallProgress is the latest state Salesforce returned for an
// asynchronous managed-package install request.
type PackageInstallProgress struct {
	RequestID string
	Status    string
	Polls     int
	Elapsed   time.Duration
}

type PackageInstallProgressFunc func(PackageInstallProgress)

type PermissionInventory struct {
	UserID   string
	Username string
	Profile  string
	Assigned map[string]bool
}

func (c *Client) ListInstalledPackages(ctx context.Context, credential Credential) ([]InstalledPackage, error) {
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return nil, err
	}
	query := "SELECT SubscriberPackage.Id,SubscriberPackage.Name,SubscriberPackage.NamespacePrefix,SubscriberPackageVersion.Id,SubscriberPackageVersion.Name,SubscriberPackageVersion.MajorVersion,SubscriberPackageVersion.MinorVersion,SubscriberPackageVersion.PatchVersion,SubscriberPackageVersion.BuildNumber FROM InstalledSubscriberPackage"
	var payload struct {
		Records []struct {
			Package struct {
				ID              string `json:"Id"`
				Name            string `json:"Name"`
				NamespacePrefix string `json:"NamespacePrefix"`
			} `json:"SubscriberPackage"`
			Version struct {
				ID           string `json:"Id"`
				Name         string `json:"Name"`
				MajorVersion int    `json:"MajorVersion"`
				MinorVersion int    `json:"MinorVersion"`
				PatchVersion int    `json:"PatchVersion"`
				BuildNumber  int    `json:"BuildNumber"`
			} `json:"SubscriberPackageVersion"`
		} `json:"records"`
	}
	path := "/services/data/" + version + "/tooling/query?q=" + url.QueryEscape(query)
	if err := c.doJSON(ctx, credential, http.MethodGet, path, nil, &payload); err != nil {
		return nil, fmt.Errorf("list installed Salesforce packages: %w", err)
	}
	packages := make([]InstalledPackage, 0, len(payload.Records))
	for _, record := range payload.Records {
		packages = append(packages, InstalledPackage{
			PackageID: record.Package.ID, Name: record.Package.Name, Namespace: record.Package.NamespacePrefix,
			VersionID: record.Version.ID, VersionName: record.Version.Name,
			VersionNumber: fmt.Sprintf("%d.%d.%d.%d", record.Version.MajorVersion, record.Version.MinorVersion, record.Version.PatchVersion, record.Version.BuildNumber),
		})
	}
	return packages, nil
}

func (c *Client) InstallPackage(ctx context.Context, credential Credential, versionID, securityType string) error {
	return c.InstallPackageWithProgress(ctx, credential, versionID, securityType, nil)
}

func (c *Client) InstallPackageWithProgress(ctx context.Context, credential Credential, versionID, securityType string, progress PackageInstallProgressFunc) error {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return fmt.Errorf("Salesforce package version ID is required")
	}
	securityType, err := toolingPackageSecurityType(securityType)
	if err != nil {
		return err
	}
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return err
	}
	var created struct {
		ID      string   `json:"id"`
		Success bool     `json:"success"`
		Errors  []string `json:"errors"`
	}
	path := "/services/data/" + version + "/tooling/sobjects/PackageInstallRequest"
	input := map[string]any{"SubscriberPackageVersionKey": versionID, "NameConflictResolution": "Block", "SecurityType": securityType}
	if err := c.doJSON(ctx, credential, http.MethodPost, path, input, &created); err != nil {
		return fmt.Errorf("install Salesforce managed package: %w", err)
	}
	if !created.Success || created.ID == "" {
		return fmt.Errorf("Salesforce did not accept the managed-package install request: %s", strings.Join(created.Errors, "; "))
	}
	startedAt := time.Now()
	if progress != nil {
		progress(PackageInstallProgress{RequestID: created.ID, Status: "QUEUED"})
	}
	polls := 0
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Salesforce managed-package install: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		var status struct {
			Status string `json:"Status"`
			Errors struct {
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			} `json:"Errors"`
		}
		if err := c.doJSON(ctx, credential, http.MethodGet, path+"/"+url.PathEscape(created.ID), nil, &status); err != nil {
			return fmt.Errorf("read Salesforce managed-package install status: %w", err)
		}
		polls++
		if progress != nil {
			progress(PackageInstallProgress{RequestID: created.ID, Status: status.Status, Polls: polls, Elapsed: time.Since(startedAt)})
		}
		switch strings.ToUpper(strings.TrimSpace(status.Status)) {
		case "SUCCESS":
			return nil
		case "ERROR":
			messages := make([]string, 0, len(status.Errors.Errors))
			for _, installError := range status.Errors.Errors {
				messages = append(messages, installError.Message)
			}
			return fmt.Errorf("Salesforce managed-package install failed: %s", firstNonEmpty(strings.Join(messages, "; "), "unknown Salesforce error"))
		}
	}
}

// toolingPackageSecurityType translates the public CLI-style values used by
// dispatch.bcl into PackageInstallRequest's restricted Tooling API picklist.
func toolingPackageSecurityType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "adminsonly", "none":
		return "None", nil
	case "allusers", "full":
		return "Full", nil
	case "custom":
		return "Custom", nil
	case "push":
		return "Push", nil
	default:
		return "", fmt.Errorf("unsupported Salesforce package security type %q", value)
	}
}

func (c *Client) ReadPermissionInventory(ctx context.Context, credential Credential, username string) (PermissionInventory, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return PermissionInventory{}, fmt.Errorf("Salesforce did not return the authenticated deployment username; reconnect the selected org and retry")
	}
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return PermissionInventory{}, err
	}
	query := "SELECT Id,Username,Profile.Name FROM User WHERE Username='" + escapeSOQL(username) + "'"
	var users struct {
		Records []struct {
			ID       string `json:"Id"`
			Username string `json:"Username"`
			Profile  struct {
				Name string `json:"Name"`
			} `json:"Profile"`
		} `json:"records"`
	}
	if err := c.query(ctx, credential, version, query, &users); err != nil {
		return PermissionInventory{}, fmt.Errorf("read Salesforce deployment user: %w", err)
	}
	if len(users.Records) != 1 {
		return PermissionInventory{}, fmt.Errorf("Salesforce did not return exactly one deployment user for %s", username)
	}
	user := users.Records[0]
	assignmentQuery := "SELECT PermissionSet.Name,PermissionSet.NamespacePrefix FROM PermissionSetAssignment WHERE AssigneeId='" + escapeSOQL(user.ID) + "'"
	var assignments struct {
		Records []struct {
			PermissionSet struct {
				Name            string `json:"Name"`
				NamespacePrefix string `json:"NamespacePrefix"`
			} `json:"PermissionSet"`
		} `json:"records"`
	}
	if err := c.query(ctx, credential, version, assignmentQuery, &assignments); err != nil {
		return PermissionInventory{}, fmt.Errorf("read Salesforce permission-set assignments: %w", err)
	}
	result := PermissionInventory{UserID: user.ID, Username: user.Username, Profile: user.Profile.Name, Assigned: map[string]bool{}}
	for _, assignment := range assignments.Records {
		name := assignment.PermissionSet.Name
		if assignment.PermissionSet.NamespacePrefix != "" {
			name = assignment.PermissionSet.NamespacePrefix + "__" + name
		}
		result.Assigned[strings.ToLower(name)] = true
	}
	return result, nil
}

func (c *Client) AssignPermissionSets(ctx context.Context, credential Credential, userID string, names []string) error {
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return err
	}
	for _, fullName := range names {
		name, namespace := splitPermissionSetName(fullName)
		query := "SELECT Id FROM PermissionSet WHERE Name='" + escapeSOQL(name) + "'"
		if namespace == "" {
			query += " AND NamespacePrefix=null"
		} else {
			query += " AND NamespacePrefix='" + escapeSOQL(namespace) + "'"
		}
		var permissionSets struct {
			Records []struct {
				ID string `json:"Id"`
			} `json:"records"`
		}
		if err := c.query(ctx, credential, version, query, &permissionSets); err != nil {
			return fmt.Errorf("find Salesforce permission set %s: %w", fullName, err)
		}
		if len(permissionSets.Records) != 1 {
			return fmt.Errorf("Salesforce did not return exactly one permission set for %s", fullName)
		}
		var created struct {
			Success bool     `json:"success"`
			Errors  []string `json:"errors"`
		}
		path := "/services/data/" + version + "/sobjects/PermissionSetAssignment"
		if err := c.doJSON(ctx, credential, http.MethodPost, path, map[string]string{"AssigneeId": userID, "PermissionSetId": permissionSets.Records[0].ID}, &created); err != nil {
			return fmt.Errorf("assign Salesforce permission set %s: %w", fullName, err)
		}
		if !created.Success {
			return fmt.Errorf("assign Salesforce permission set %s: %s", fullName, strings.Join(created.Errors, "; "))
		}
	}
	return nil
}

func (c *Client) query(ctx context.Context, credential Credential, version, soql string, out any) error {
	return c.doJSON(ctx, credential, http.MethodGet, "/services/data/"+version+"/query?q="+url.QueryEscape(soql), nil, out)
}

func splitPermissionSetName(value string) (name, namespace string) {
	value = strings.TrimSpace(value)
	if prefix, suffix, ok := strings.Cut(value, "__"); ok {
		return suffix, prefix
	}
	return value, ""
}

func escapeSOQL(value string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
}
