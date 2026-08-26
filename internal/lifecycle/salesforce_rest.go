package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

func salesforceRESTCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	settings = settings.HydrateSalesforceOrgs()
	clientID, clientSecret := settings.SalesforceClientID, settings.SalesforceClientSecret
	if org, ok := settings.SelectedSalesforceOrg(); ok && strings.TrimSpace(org.ClientID) != "" {
		clientID, clientSecret = org.ClientID, org.ClientSecret
	}
	clientID = firstSalesforceValue(clientID, salesforceapi.LoginClientID)
	if clientID == salesforceapi.LoginClientID {
		clientSecret = ""
	}
	return salesforceapi.Credential{
		InstanceURL:  settings.SalesforceInstanceURL,
		AccessToken:  settings.SalesforceAccessToken,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
}

// salesforceRESTSession keeps validation on the same browser OAuth connection
// the user selected. It intentionally has no dependency on the Salesforce CLI.
type salesforceRESTSession struct {
	settings   config.ConnectionSettings
	credential salesforceapi.Credential
	client     *salesforceapi.Client
}

func newSalesforceRESTSession(settings config.ConnectionSettings) *salesforceRESTSession {
	settings = settings.HydrateSalesforceOrgs()
	return &salesforceRESTSession{settings: settings, credential: salesforceRESTCredential(settings), client: salesforceapi.NewClient()}
}

func (session *salesforceRESTSession) refresh(ctx context.Context) error {
	refreshToken := strings.TrimSpace(session.settings.SalesforceRefreshToken)
	if refreshToken == "" {
		return session.invalidate(fmt.Errorf("the Salesforce session expired and this connection has no refresh token; reconnect the selected org"))
	}
	clientID := firstSalesforceValue(session.credential.ClientID, session.settings.SalesforceClientID, salesforceapi.LoginClientID)
	var lastErr error
	for _, loginURL := range salesforceRefreshLoginURLs(session.settings.SalesforceInstanceURL) {
		refreshed, err := session.client.RefreshAccessToken(ctx, salesforceapi.RefreshRequest{
			LoginURL: loginURL, ClientID: clientID, ClientSecret: session.credential.ClientSecret, RefreshToken: refreshToken,
		})
		if err != nil {
			lastErr = err
			continue
		}
		session.settings.SalesforceAccessToken = refreshed.AccessToken
		if refreshed.RefreshToken != "" {
			session.settings.SalesforceRefreshToken = refreshed.RefreshToken
		}
		if refreshed.InstanceURL != "" {
			session.settings.SalesforceInstanceURL = strings.TrimRight(refreshed.InstanceURL, "/")
		}
		session.settings = session.settings.SyncSelectedSalesforceOrg()
		if err := shellstate.SaveConnectionSettings(session.settings); err != nil {
			return fmt.Errorf("save refreshed Salesforce connection: %w", err)
		}
		session.credential = salesforceRESTCredential(session.settings)
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Salesforce token refresh failed")
	}
	return session.invalidate(fmt.Errorf("refresh the selected Salesforce connection: %w", lastErr))
}

func (session *salesforceRESTSession) check(ctx context.Context, report Reporter) (salesforceapi.OrgStatus, error) {
	org, err := session.client.Check(ctx, session.credential)
	if err != nil && salesforceSessionExpired(err) {
		report.step("Refreshing the selected Salesforce session")
		if refreshErr := session.refresh(ctx); refreshErr != nil {
			return salesforceapi.OrgStatus{}, refreshErr
		}
		org, err = session.client.Check(ctx, session.credential)
	}
	if err != nil {
		return salesforceapi.OrgStatus{}, session.invalidate(err)
	}
	org = session.enrichOrgStatus(org)
	if strings.TrimSpace(org.InstanceURL) != "" {
		session.credential.InstanceURL = strings.TrimRight(org.InstanceURL, "/")
	}
	return org, nil
}

func (session *salesforceRESTSession) enrichOrgStatus(org salesforceapi.OrgStatus) salesforceapi.OrgStatus {
	selected, _ := session.settings.SelectedSalesforceOrg()
	if strings.TrimSpace(org.Username) == "" {
		org.Username = firstSalesforceValue(selected.Username, session.settings.VerifiedConnections["salesforce"].Identity)
	}
	if strings.TrimSpace(org.OrgID) == "" {
		org.OrgID = firstSalesforceValue(selected.OrgID, session.settings.SalesforceOrgID)
	}
	if strings.TrimSpace(org.InstanceURL) == "" {
		org.InstanceURL = firstSalesforceValue(selected.InstanceURL, session.settings.SalesforceInstanceURL)
	}
	return org
}

func (session *salesforceRESTSession) invalidate(authenticationErr error) error {
	session.settings = session.settings.InvalidateSelectedSalesforceVerification("Unavailable")
	if err := shellstate.SaveConnectionSettings(session.settings); err != nil {
		return fmt.Errorf("%w; clear stale Salesforce readiness: %v", authenticationErr, err)
	}
	return authenticationErr
}

func salesforceSessionExpired(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_session_id") || strings.Contains(message, "session expired") || strings.Contains(message, "session invalid")
}

func salesforceRefreshLoginURLs(instanceURL string) []string {
	urls := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, value := range []string{instanceURL, salesforceapi.DefaultProductionLogin, salesforceapi.DefaultSandboxLogin} {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		urls = append(urls, value)
	}
	return urls
}

func firstSalesforceValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func validateSalesforceREST(root string, item Item, report Reporter, settings config.ConnectionSettings) (Item, error) {
	project := findSalesforceProject(root)
	if project == "" {
		item.Status, item.Detail = StatusManual, "No Salesforce project was found in the package."
		return item, nil
	}
	ctx := context.Background()
	session := newSalesforceRESTSession(settings)
	report.step("Checking Salesforce org availability")
	org, err := session.check(ctx, report)
	if err != nil {
		return salesforceRESTFailure(item, "Salesforce validation stopped before reading metadata", err), nil
	}
	alias := salesforceRESTLabel(settings, org)
	manifest, err := solution.Load(root)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to read Salesforce deployment prerequisites", err), nil
	}

	item.ComponentOrder = []string{"Managed Package"}
	for index, requirement := range manifest.Salesforce.RequiredPackages {
		report.component(salesforcePackageComponent(requirement), ProgressRunning, "Checking installed package version", index, len(manifest.Salesforce.RequiredPackages))
	}
	report.step("Checking required Salesforce managed packages")
	apiPackages, err := session.client.ListInstalledPackages(ctx, session.credential)
	if err != nil && salesforceSessionExpired(err) {
		report.step("Refreshing the selected Salesforce session")
		if refreshErr := session.refresh(ctx); refreshErr != nil {
			return salesforceRESTFailure(item, "Unable to refresh Salesforce session", refreshErr), nil
		}
		apiPackages, err = session.client.ListInstalledPackages(ctx, session.credential)
	}
	if err != nil {
		return salesforceRESTFailure(item, "Unable to inspect installed Salesforce packages", err), nil
	}
	installedPackages := installedSalesforcePackages(apiPackages)
	for index, requirement := range manifest.Salesforce.RequiredPackages {
		message := "Required version is installed"
		if len(missingSalesforcePackages([]solution.SalesforcePackageRequirement{requirement}, installedPackages)) > 0 {
			message = "Package installation required"
		}
		report.component(salesforcePackageComponent(requirement), ProgressCompleted, message, index+1, len(manifest.Salesforce.RequiredPackages))
	}

	for index, requirement := range manifest.Salesforce.RequiredPermissionSets {
		report.component(salesforcePermissionSetComponent(requirement), ProgressRunning, "Checking assignment for the deployment user", index, len(manifest.Salesforce.RequiredPermissionSets))
	}
	report.step("Checking required Salesforce permission sets")
	permissionInventory, err := session.client.ReadPermissionInventory(ctx, session.credential, org.Username)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to inspect Salesforce permission-set assignments", err), nil
	}
	for index, requirement := range manifest.Salesforce.RequiredPermissionSets {
		message := "Assigned to the deployment user"
		if !permissionInventory.Assigned[strings.ToLower(strings.TrimSpace(requirement.Name))] {
			message = "Assignment required"
		}
		report.component(salesforcePermissionSetComponent(requirement), ProgressCompleted, message, index+1, len(manifest.Salesforce.RequiredPermissionSets))
	}
	if !strings.EqualFold(permissionInventory.Profile, "System Administrator") {
		item.Status = StatusFailed
		item.Detail = fmt.Sprintf("The authenticated Salesforce deployment user %s has profile %q. Select a System Administrator connection so Dispatch can install prerequisites and assign the required permission sets.", org.Username, permissionInventory.Profile)
		return item, nil
	}

	report.step("Building Salesforce UI bundles")
	if err := buildSalesforceUIBundles(project); err != nil {
		return salesforceRESTFailure(item, "Unable to prepare packaged Salesforce UI Bundles", err), nil
	}
	expected, apiVersion, err := salesforceapi.InventorySource(project)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to inventory packaged Salesforce metadata", err), nil
	}
	components := sortedInventoryComponents(expected)
	for index, component := range components {
		report.component(component, ProgressRunning, "Finding and comparing Salesforce configuration", index, len(components))
	}
	report.step(fmt.Sprintf("Comparing %d packaged metadata components with Salesforce", len(components)))
	matching, diff, err := compareSalesforceMetadataState(ctx, session.client, session.credential, project, apiVersion, components, report)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to compare Salesforce metadata", err), nil
	}
	report.step(fmt.Sprintf("Salesforce comparison complete: %d matching, %d changed, %d missing", len(diff.Matching), len(diff.Changed), len(diff.Missing)))
	result := classifySalesforceInventory(item, expected, matching, alias)
	result.Changes = append([]salesforceapi.MetadataFileDiff(nil), diff.Files...)
	result = addSalesforcePackageResults(result, manifest.Salesforce.RequiredPackages, installedPackages, alias)
	result = addSalesforcePermissionSetResults(result, manifest.Salesforce.RequiredPermissionSets, permissionInventory.Assigned, alias)
	result, err = inspectSalesforceSeedScripts(ctx, session.client, session.credential, result, manifest.Salesforce.SeedScripts, alias, report)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to inspect Salesforce sample records", err), nil
	}
	return result, nil
}

func deploySalesforceREST(root string, item Item, settings config.ConnectionSettings, report Reporter) Item {
	project := findSalesforceProject(root)
	if project == "" {
		item.Status, item.Detail = StatusFailed, "No Salesforce project was found in the package."
		return item
	}
	ctx := context.Background()
	session := newSalesforceRESTSession(settings)
	const orgAvailabilityComponent = "Salesforce org availability"
	report.component(orgAvailabilityComponent, ProgressRunning, "Checking the selected Salesforce org", 0, 1)
	org, err := session.check(ctx, report)
	if err != nil {
		report.component(orgAvailabilityComponent, ProgressFailed, "Salesforce org is unavailable", 1, 1)
		return salesforceRESTFailure(item, "Salesforce deployment stopped before sending metadata", err)
	}
	report.component(orgAvailabilityComponent, ProgressCompleted, "Salesforce org is available", 1, 1)
	manifest, err := solution.Load(root)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to read Salesforce deployment prerequisites", err)
	}
	if err := ensureSalesforceRESTPackages(ctx, session.client, session.credential, manifest.Salesforce.RequiredPackages, report); err != nil {
		return salesforceRESTFailure(item, "Salesforce deployment stopped before sending metadata", err)
	}

	report.step("Building Salesforce UI bundles")
	if err := buildSalesforceUIBundles(project); err != nil {
		return salesforceRESTFailure(item, "Unable to prepare packaged Salesforce UI Bundles", err)
	}
	metadata := missingSalesforceMetadata(item.Missing)
	expected, sourceAPIVersion, inventoryErr := salesforceapi.InventorySource(project)
	if inventoryErr != nil {
		return salesforceRESTFailure(item, "Unable to inventory packaged Salesforce metadata", inventoryErr)
	}
	// Experience Cloud settings are not reliably described by ListMetadata: the
	// settings container can exist while Digital Experiences itself is disabled.
	// Whenever this solution deploys Salesforce work, send the enabling settings
	// before the application/site stages instead of treating container presence as
	// proof that the feature is on.
	experienceSiteExpected := sourceIncludesSalesforceExperience(expected)
	if experienceSiteExpected {
		metadata = includeSalesforceOrgSettings(metadata, expected)
	}
	experienceSiteDeployment := hasSalesforceExperienceSiteMetadata(metadata)
	var salesforceAPIVersion string
	if experienceSiteDeployment {
		salesforceAPIVersion = sourceAPIVersion
		for component := range expected {
			if strings.HasPrefix(component, "UIBundle:") && !slices.Contains(metadata, component) {
				metadata = append(metadata, component)
			}
		}
		slices.Sort(metadata)
	}
	deployIDs := []string{}
	if len(metadata) > 0 {
		for index, component := range metadata {
			report.component(component, ProgressRunning, "Queued for Salesforce deployment", index, len(metadata))
		}
		for _, stage := range salesforceMetadataStages(metadata) {
			zipData, apiVersion, buildErr := salesforceapi.BuildMetadataPackage(project, stage.Components)
			if buildErr != nil {
				return salesforceRESTFailure(item, "Unable to build the Salesforce Metadata API package", buildErr)
			}
			report.step(fmt.Sprintf("Deploying %s (%d components)", stage.Label, len(stage.Components)))
			deployment, deployErr := session.client.DeployMetadata(ctx, session.credential, apiVersion, zipData, func(progress salesforceapi.MetadataProgress) {
				message := "Salesforce " + stage.Label + " deployment " + strings.ToLower(strings.TrimSpace(progress.Status))
				if progress.ComponentsTotal > 0 {
					message = fmt.Sprintf("Salesforce deployed %d of %d %s components", progress.ComponentsDeployed, progress.ComponentsTotal, stage.Label)
				}
				report.step(message)
			})
			if strings.TrimSpace(deployment.ID) != "" {
				deployIDs = append(deployIDs, deployment.ID)
			}
			if deployErr != nil {
				return salesforceRESTFailure(item, "Salesforce "+stage.Label+" deployment failed", deployErr)
			}
		}
		report.step("Verifying deployed Salesforce configuration")
		_, postDeployDiff, verifyErr := compareSalesforceMetadataState(ctx, session.client, session.credential, project, sourceAPIVersion, metadata, report)
		if verifyErr != nil {
			return salesforceRESTFailure(item, "Salesforce metadata deployed, but the resulting configuration could not be verified", verifyErr)
		}
		if len(postDeployDiff.Changed) > 0 || len(postDeployDiff.Missing) > 0 {
			unverified := append([]string(nil), postDeployDiff.Changed...)
			unverified = append(unverified, postDeployDiff.Missing...)
			slices.Sort(unverified)
			return salesforceRESTFailure(item, "Salesforce metadata deployed, but the resulting configuration does not match the package", fmt.Errorf("unverified components: %s", strings.Join(unverified, ", ")))
		}
		for index, component := range metadata {
			report.component(component, ProgressCompleted, "Deployed and verified in Salesforce", index+1, len(metadata))
		}
	} else {
		report.step("Salesforce metadata is already present; skipping metadata deployment")
	}

	instanceURL := strings.TrimRight(session.credential.InstanceURL, "/")
	addResource(&item, "Salesforce org", "organization", org.Username, org.OrgID, instanceURL)
	for index, deployID := range deployIDs {
		addResource(&item, "Salesforce metadata", "metadata_deployment", fmt.Sprintf("Salesforce metadata deployment %d", index+1), deployID, instanceURL+"/lightning/setup/DeployStatus/home")
	}
	if experienceSiteExpected {
		networkName, networkErr := salesforceExperienceNetworkName(sortedInventoryComponents(expected))
		if networkErr != nil {
			return salesforceRESTFailure(item, "Salesforce metadata deployed, but the external application could not be published", networkErr)
		}
		report.step("Publishing the external Salesforce Experience Cloud application")
		published, publishErr := session.client.PublishExperienceSite(ctx, session.credential, salesforceAPIVersion, networkName, func(status string) {
			report.step("Salesforce Experience Cloud publish: " + strings.ToLower(strings.TrimSpace(status)))
		})
		if publishErr != nil {
			return salesforceRESTFailure(item, "Salesforce metadata deployed, but the external application publish failed", publishErr)
		}
		addResource(&item, "Salesforce Experience", "experience_site", published.Name, published.ID, published.URL)
	}
	report.step("Assigning required Salesforce permission sets")
	if err := ensureSalesforceRESTPermissionSets(ctx, session.client, session.credential, org.Username, manifest.Salesforce.RequiredPermissionSets, report); err != nil {
		return salesforceRESTFailure(item, "Salesforce metadata deployed, but required permission-set assignment failed", err)
	}
	if err := deploySalesforceSeedScripts(ctx, project, session.client, session.credential, manifest.Salesforce.SeedScripts, instanceURL, &item, report); err != nil {
		return salesforceRESTFailure(item, "Salesforce metadata deployed, but sample record creation failed", err)
	}
	item.Status, item.Detail = StatusPresent, "Salesforce metadata, external application, prerequisites, and sample records deployed successfully."
	item.Present = append(item.Present, item.Missing...)
	slices.Sort(item.Present)
	item.Present = slices.Compact(item.Present)
	item.Missing = nil
	item.DeployableComponents = nil
	return item
}

type salesforceMetadataStage struct {
	Label      string
	Components []string
}

func salesforceMetadataStages(components []string) []salesforceMetadataStage {
	groups := []salesforceMetadataStage{{Label: "org settings"}, {Label: "application metadata"}, {Label: "Experience Cloud site"}}
	for _, component := range components {
		metadataType, _, _ := strings.Cut(component, ":")
		switch metadataType {
		case "Settings":
			groups[0].Components = append(groups[0].Components, component)
		case "CustomSite", "DigitalExperience", "DigitalExperienceBundle", "DigitalExperienceConfig", "Network":
			groups[2].Components = append(groups[2].Components, component)
		default:
			groups[1].Components = append(groups[1].Components, component)
		}
	}
	stages := make([]salesforceMetadataStage, 0, len(groups))
	for _, group := range groups {
		if len(group.Components) > 0 {
			slices.Sort(group.Components)
			stages = append(stages, group)
		}
	}
	return stages
}

func hasSalesforceExperienceSiteMetadata(components []string) bool {
	for _, component := range components {
		for _, prefix := range []string{"CustomSite:", "DigitalExperience:", "DigitalExperienceBundle:", "DigitalExperienceConfig:", "Network:"} {
			if strings.HasPrefix(component, prefix) {
				return true
			}
		}
	}
	return false
}

func sourceIncludesSalesforceExperience(inventory map[string]bool) bool {
	for component := range inventory {
		if hasSalesforceExperienceSiteMetadata([]string{component}) {
			return true
		}
	}
	return false
}

func includeSalesforceOrgSettings(components []string, inventory map[string]bool) []string {
	result := append([]string(nil), components...)
	for _, component := range salesforceOrgSettingsComponents(inventory) {
		if !slices.Contains(result, component) {
			result = append(result, component)
		}
	}
	slices.Sort(result)
	return result
}

func salesforceOrgSettingsComponents(inventory map[string]bool) []string {
	result := []string{}
	for _, component := range []string{"Settings:Communities", "Settings:ExperienceBundle"} {
		if inventory[component] {
			result = append(result, component)
		}
	}
	return result
}

func salesforceExperienceNetworkName(components []string) (string, error) {
	names := []string{}
	for _, component := range components {
		if name, ok := strings.CutPrefix(component, "Network:"); ok && strings.TrimSpace(name) != "" {
			names = append(names, strings.TrimSpace(name))
		}
	}
	names = slices.Compact(names)
	if len(names) != 1 {
		return "", fmt.Errorf("expected exactly one Network component, found %d", len(names))
	}
	return names[0], nil
}

func salesforceSeedComponent(script solution.SalesforceSeedScript) string {
	label := strings.TrimSpace(script.Label)
	if label == "" {
		label = filepath.Base(script.Path)
	}
	return "Sample Records:" + label
}

func inspectSalesforceSeedScripts(ctx context.Context, client *salesforceapi.Client, credential salesforceapi.Credential, item Item, scripts []solution.SalesforceSeedScript, alias string, report Reporter) (Item, error) {
	for index, script := range scripts {
		component := salesforceSeedComponent(script)
		item.ComponentOrder = append(item.ComponentOrder, component)
		if slices.Contains(item.Missing, "CustomObject:"+strings.TrimSpace(script.Object)) {
			item.Missing = append(item.Missing, component)
			item.DeployableComponents = append(item.DeployableComponents, component)
			item.Status, item.Deployable = StatusMissing, true
			item.Detail = fmt.Sprintf("%d components already exist; %d need deployment to Salesforce org %s.", len(item.Present), len(item.Missing), alias)
			report.component(component, ProgressCompleted, fmt.Sprintf("%d sample records will be created after the object is deployed", len(script.ExternalIDs)), index+1, len(scripts))
			continue
		}
		if hasMissingSalesforcePermissionAssignment(item.Missing) {
			item.Missing = append(item.Missing, component)
			item.DeployableComponents = append(item.DeployableComponents, component)
			item.Status, item.Deployable = StatusMissing, true
			item.Detail = fmt.Sprintf("%d components already exist; %d need deployment to Salesforce org %s.", len(item.Present), len(item.Missing), alias)
			report.component(component, ProgressCompleted, fmt.Sprintf("%d sample records will be checked after required permissions are assigned", len(script.ExternalIDs)), index+1, len(scripts))
			continue
		}
		report.component(component, ProgressRunning, "Checking sample records", index, len(scripts))
		records, err := client.FindRecordsByStringField(ctx, credential, script.Object, script.ExternalIDField, script.ExternalIDs)
		if err != nil {
			return item, err
		}
		if len(records) == len(script.ExternalIDs) {
			item.Present = append(item.Present, component)
			report.component(component, ProgressCompleted, fmt.Sprintf("%d sample records are present", len(records)), index+1, len(scripts))
			continue
		}
		item.Missing = append(item.Missing, component)
		item.DeployableComponents = append(item.DeployableComponents, component)
		item.Status, item.Deployable = StatusMissing, true
		item.Detail = fmt.Sprintf("%d components already exist; %d need deployment to Salesforce org %s.", len(item.Present), len(item.Missing), alias)
		report.component(component, ProgressCompleted, fmt.Sprintf("%d of %d sample records need creation", len(script.ExternalIDs)-len(records), len(script.ExternalIDs)), index+1, len(scripts))
	}
	slices.Sort(item.Present)
	slices.Sort(item.Missing)
	slices.Sort(item.DeployableComponents)
	item.Present = slices.Compact(item.Present)
	item.Missing = slices.Compact(item.Missing)
	item.DeployableComponents = slices.Compact(item.DeployableComponents)
	return item, nil
}

func hasMissingSalesforcePermissionAssignment(components []string) bool {
	return slices.ContainsFunc(components, func(component string) bool {
		return strings.HasPrefix(component, "Permission Set Assignment:")
	})
}

func deploySalesforceSeedScripts(ctx context.Context, project string, client *salesforceapi.Client, credential salesforceapi.Credential, scripts []solution.SalesforceSeedScript, instanceURL string, item *Item, report Reporter) error {
	for index, script := range scripts {
		component := salesforceSeedComponent(script)
		path, err := salesforceSeedScriptPath(project, script.Path)
		if err != nil {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", component, err)
		}
		report.component(component, ProgressRunning, "Creating or updating sample records", index, len(scripts))
		report.step("Populating " + strings.TrimPrefix(component, "Sample Records:"))
		if err := client.ExecuteAnonymous(ctx, credential, "", string(source)); err != nil {
			return err
		}
		records, err := client.FindRecordsByStringField(ctx, credential, script.Object, script.ExternalIDField, script.ExternalIDs)
		if err != nil {
			return err
		}
		if len(records) != len(script.ExternalIDs) {
			return fmt.Errorf("%s created %d of %d expected records", component, len(records), len(script.ExternalIDs))
		}
		for _, record := range records {
			name := firstNonEmpty(record.Name, record.Key)
			addResource(item, component, "salesforce_record", name, record.ID, strings.TrimRight(instanceURL, "/")+"/lightning/r/"+script.Object+"/"+record.ID+"/view")
		}
		report.component(component, ProgressCompleted, fmt.Sprintf("%d sample records are ready", len(records)), index+1, len(scripts))
	}
	return nil
}

func salesforceSeedScriptPath(project, reference string) (string, error) {
	reference = filepath.Clean(strings.TrimSpace(reference))
	if reference == "." || filepath.IsAbs(reference) || reference == ".." || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Salesforce seed script path must stay inside the Salesforce project")
	}
	return filepath.Join(project, reference), nil
}

func ensureSalesforceRESTPackages(ctx context.Context, client *salesforceapi.Client, credential salesforceapi.Credential, required []solution.SalesforcePackageRequirement, report Reporter) error {
	installed, err := client.ListInstalledPackages(ctx, credential)
	if err != nil {
		return err
	}
	missing := missingSalesforcePackages(required, installedSalesforcePackages(installed))
	for index, requirement := range missing {
		component := salesforcePackageComponent(requirement)
		if strings.TrimSpace(requirement.VersionID) == "" {
			return fmt.Errorf("required Salesforce package %s does not declare an installable 04t version ID in dispatch.bcl", firstNonEmpty(requirement.Name, requirement.Namespace))
		}
		report.component(component, ProgressRunning, "Submitting the managed-package install to Salesforce", index, len(missing))
		if err := client.InstallPackageWithProgress(ctx, credential, requirement.VersionID, requirement.SecurityType, func(progress salesforceapi.PackageInstallProgress) {
			report.component(component, ProgressRunning, salesforcePackageInstallMessage(progress), index, len(missing))
		}); err != nil {
			return fmt.Errorf("install %s: %w", firstNonEmpty(requirement.Name, requirement.Namespace), err)
		}
		report.component(component, ProgressCompleted, "Managed package installed", index+1, len(missing))
	}
	if len(missing) == 0 {
		return nil
	}
	installed, err = client.ListInstalledPackages(ctx, credential)
	if err != nil {
		return err
	}
	if remaining := missingSalesforcePackages(required, installedSalesforcePackages(installed)); len(remaining) > 0 {
		return fmt.Errorf("Salesforce accepted the package install, but %s is still not present at the required version", firstNonEmpty(remaining[0].Name, remaining[0].Namespace))
	}
	return nil
}

func salesforcePackageInstallMessage(progress salesforceapi.PackageInstallProgress) string {
	status := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(progress.Status)), "_", " ")
	if status == "" {
		status = "processing"
	}
	message := "Salesforce reports " + status
	if progress.Elapsed >= time.Second {
		message += " · " + progress.Elapsed.Round(time.Second).String() + " elapsed"
	}
	if progress.RequestID != "" && progress.Polls == 0 {
		message += " · request " + progress.RequestID
	}
	return message
}

func ensureSalesforceRESTPermissionSets(ctx context.Context, client *salesforceapi.Client, credential salesforceapi.Credential, username string, required []solution.SalesforcePermissionSetRequirement, report Reporter) error {
	if len(required) == 0 {
		return nil
	}
	inventory, err := client.ReadPermissionInventory(ctx, credential, username)
	if err != nil {
		return err
	}
	if !strings.EqualFold(inventory.Profile, "System Administrator") {
		return fmt.Errorf("the authenticated Salesforce deployment user %s has profile %q, not System Administrator", username, inventory.Profile)
	}
	missing := missingSalesforcePermissionSets(required, inventory.Assigned)
	names := make([]string, 0, len(missing))
	for index, requirement := range missing {
		names = append(names, requirement.Name)
		report.component(salesforcePermissionSetComponent(requirement), ProgressRunning, "Assigning to the deployment user", index, len(missing))
	}
	if err := client.AssignPermissionSets(ctx, credential, inventory.UserID, names); err != nil {
		return err
	}
	verified, err := client.ReadPermissionInventory(ctx, credential, username)
	if err != nil {
		return err
	}
	if remaining := missingSalesforcePermissionSets(required, verified.Assigned); len(remaining) > 0 {
		return fmt.Errorf("%s is still not assigned to the authenticated System Administrator", firstNonEmpty(remaining[0].Label, remaining[0].Name))
	}
	for index, requirement := range missing {
		report.component(salesforcePermissionSetComponent(requirement), ProgressCompleted, "Assigned to the deployment user", index+1, len(missing))
	}
	return nil
}

func installedSalesforcePackages(packages []salesforceapi.InstalledPackage) []installedSalesforcePackage {
	result := make([]installedSalesforcePackage, 0, len(packages))
	for _, item := range packages {
		result = append(result, installedSalesforcePackage{
			SubscriberPackageID: item.PackageID, SubscriberPackageName: item.Name,
			SubscriberPackageNamespace: item.Namespace, SubscriberPackageVersionID: item.VersionID,
			SubscriberPackageVersionName: item.VersionName, SubscriberPackageVersionNumber: item.VersionNumber,
		})
	}
	return result
}

func sortedInventoryComponents(inventory map[string]bool) []string {
	components := make([]string, 0, len(inventory))
	for component := range inventory {
		components = append(components, component)
	}
	slices.Sort(components)
	return components
}

func metadataTypes(components []string) []string {
	unique := map[string]bool{}
	for _, component := range components {
		metadataType, _, ok := strings.Cut(component, ":")
		if ok && metadataType != "" {
			unique[metadataType] = true
		}
	}
	return sortedInventoryComponents(unique)
}

func compareSalesforceMetadataState(ctx context.Context, client *salesforceapi.Client, credential salesforceapi.Credential, project, apiVersion string, components []string, report Reporter) (map[string]bool, salesforceapi.MetadataDiff, error) {
	componentIndexes := make(map[string]int, len(components))
	for index, component := range components {
		componentIndexes[component] = index
	}
	existing, err := client.ListMetadataWithProgress(ctx, credential, apiVersion, metadataTypes(components), func(metadataType string, fullNames []string) {
		present := make(map[string]bool, len(fullNames))
		for _, fullName := range fullNames {
			present[metadataType+":"+fullName] = true
		}
		for _, component := range components {
			if !strings.HasPrefix(component, metadataType+":") {
				continue
			}
			message := "Not present"
			if present[component] {
				message = "Found; comparing configuration"
			}
			report.component(component, ProgressRunning, message, componentIndexes[component]+1, len(components))
		}
	})
	if err != nil {
		return nil, salesforceapi.MetadataDiff{}, err
	}
	existingComponents := []string{}
	for _, component := range components {
		if existing[component] {
			existingComponents = append(existingComponents, component)
		} else {
			report.component(component, ProgressCompleted, "Not present; will be deployed", componentIndexes[component]+1, len(components))
		}
	}
	diff := salesforceapi.MetadataDiff{}
	var zipData []byte
	if len(existingComponents) > 0 {
		report.step(fmt.Sprintf("Retrieving %d existing Salesforce components for comparison", len(existingComponents)))
		var retrieveErr error
		zipData, retrieveErr = client.RetrieveMetadata(ctx, credential, apiVersion, existingComponents, func(progress salesforceapi.MetadataRetrieveProgress) {
			status := strings.TrimSpace(progress.Status)
			if status != "" {
				report.step("Salesforce metadata retrieval " + strings.ToLower(status))
			}
		})
		if retrieveErr != nil {
			return nil, salesforceapi.MetadataDiff{}, retrieveErr
		}
	}
	compared, compareErr := salesforceapi.CompareMetadataSource(project, components, zipData)
	if compareErr != nil {
		return nil, salesforceapi.MetadataDiff{}, compareErr
	}
	diff = compared
	for _, component := range compared.Matching {
		report.component(component, ProgressCompleted, "Matches packaged configuration", componentIndexes[component]+1, len(components))
	}
	for _, component := range compared.Changed {
		report.component(component, ProgressCompleted, "Configuration differs; will be updated", componentIndexes[component]+1, len(components))
	}
	for _, component := range compared.Missing {
		report.component(component, ProgressCompleted, "Not returned by Salesforce; will be deployed", componentIndexes[component]+1, len(components))
	}
	slices.Sort(diff.Matching)
	slices.Sort(diff.Changed)
	slices.Sort(diff.Missing)
	matching := make(map[string]bool, len(diff.Matching))
	for _, component := range diff.Matching {
		matching[component] = true
	}
	return matching, diff, nil
}

func salesforceRESTLabel(settings config.ConnectionSettings, org salesforceapi.OrgStatus) string {
	for _, value := range []string{settings.SalesforceAlias, org.Username, org.OrgID} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "selected org"
}

func salesforceRESTFailure(item Item, prefix string, err error) Item {
	item.Status = StatusFailed
	item.Detail = strings.TrimSpace(prefix) + ": " + err.Error()
	item.Diagnostic = err.Error()
	return item
}
