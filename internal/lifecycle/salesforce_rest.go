package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

func salesforceRESTCredential(settings config.ConnectionSettings) salesforceapi.Credential {
	return salesforceapi.Credential{
		InstanceURL:  settings.SalesforceInstanceURL,
		AccessToken:  settings.SalesforceAccessToken,
		ClientID:     settings.SalesforceClientID,
		ClientSecret: settings.SalesforceClientSecret,
	}
}

func validateSalesforceREST(root string, item Item, report Reporter, settings config.ConnectionSettings) (Item, error) {
	project := findSalesforceProject(root)
	if project == "" {
		item.Status, item.Detail = StatusManual, "No Salesforce project was found in the package."
		return item, nil
	}
	ctx := context.Background()
	client := salesforceapi.NewClient()
	credential := salesforceRESTCredential(settings)
	report.step("Checking Salesforce org availability")
	org, err := client.Check(ctx, credential)
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
	apiPackages, err := client.ListInstalledPackages(ctx, credential)
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
	permissionInventory, err := client.ReadPermissionInventory(ctx, credential, org.Username)
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
		report.component(component, ProgressRunning, "Reading current Salesforce state", index, len(components))
	}
	report.step(fmt.Sprintf("Reading Salesforce state for %d metadata components", len(components)))
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
			message := "Ready to deploy"
			if present[component] {
				message = "Already present"
			}
			report.component(component, ProgressCompleted, message, componentIndexes[component]+1, len(components))
		}
	})
	if err != nil {
		return salesforceRESTFailure(item, "Unable to read Salesforce metadata", err), nil
	}
	result := classifySalesforceInventory(item, expected, existing, alias)
	result = addSalesforcePackageResults(result, manifest.Salesforce.RequiredPackages, installedPackages, alias)
	return addSalesforcePermissionSetResults(result, manifest.Salesforce.RequiredPermissionSets, permissionInventory.Assigned, alias), nil
}

func deploySalesforceREST(root string, item Item, settings config.ConnectionSettings, report Reporter) Item {
	project := findSalesforceProject(root)
	if project == "" {
		item.Status, item.Detail = StatusFailed, "No Salesforce project was found in the package."
		return item
	}
	ctx := context.Background()
	client := salesforceapi.NewClient()
	credential := salesforceRESTCredential(settings)
	report.step("Checking Salesforce org availability")
	org, err := client.Check(ctx, credential)
	if err != nil {
		return salesforceRESTFailure(item, "Salesforce deployment stopped before sending metadata", err)
	}
	manifest, err := solution.Load(root)
	if err != nil {
		return salesforceRESTFailure(item, "Unable to read Salesforce deployment prerequisites", err)
	}
	if err := ensureSalesforceRESTPackages(ctx, client, credential, manifest.Salesforce.RequiredPackages, report); err != nil {
		return salesforceRESTFailure(item, "Salesforce deployment stopped before sending metadata", err)
	}

	report.step("Building Salesforce UI bundles")
	if err := buildSalesforceUIBundles(project); err != nil {
		return salesforceRESTFailure(item, "Unable to prepare packaged Salesforce UI Bundles", err)
	}
	metadata := missingSalesforceMetadata(item.Missing)
	deployID := ""
	if len(metadata) > 0 {
		zipData, apiVersion, err := salesforceapi.BuildMetadataPackage(project, metadata)
		if err != nil {
			return salesforceRESTFailure(item, "Unable to build the Salesforce Metadata API package", err)
		}
		for index, component := range metadata {
			report.component(component, ProgressRunning, "Queued for Salesforce deployment", index, len(metadata))
		}
		report.step(fmt.Sprintf("Deploying %d missing Salesforce metadata components", len(metadata)))
		deployment, err := client.DeployMetadata(ctx, credential, apiVersion, zipData, func(progress salesforceapi.MetadataProgress) {
			message := "Salesforce metadata deployment " + strings.ToLower(strings.TrimSpace(progress.Status))
			if progress.ComponentsTotal > 0 {
				message = fmt.Sprintf("Salesforce deployed %d of %d metadata components", progress.ComponentsDeployed, progress.ComponentsTotal)
			}
			report.step(message)
		})
		deployID = deployment.ID
		if err != nil {
			return salesforceRESTFailure(item, "Salesforce metadata deployment failed", err)
		}
		for index, component := range metadata {
			report.component(component, ProgressCompleted, "Deployed to Salesforce", index+1, len(metadata))
		}
	} else {
		report.step("Salesforce metadata is already present; skipping metadata deployment")
	}

	instanceURL := strings.TrimRight(credential.InstanceURL, "/")
	addResource(&item, "Salesforce org", "organization", org.Username, org.OrgID, instanceURL)
	if deployID != "" {
		addResource(&item, "Salesforce metadata", "metadata_deployment", "Salesforce metadata deployment", deployID, instanceURL+"/lightning/setup/DeployStatus/home")
	}
	report.step("Assigning required Salesforce permission sets")
	if err := ensureSalesforceRESTPermissionSets(ctx, client, credential, org.Username, manifest.Salesforce.RequiredPermissionSets, report); err != nil {
		return salesforceRESTFailure(item, "Salesforce metadata deployed, but required permission-set assignment failed", err)
	}
	item.Status, item.Detail = StatusPresent, "Salesforce metadata and prerequisites deployed successfully."
	item.Present = append(item.Present, item.Missing...)
	slices.Sort(item.Present)
	item.Present = slices.Compact(item.Present)
	item.Missing = nil
	item.DeployableComponents = nil
	return item
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
		report.component(component, ProgressRunning, "Installing managed package before metadata", index, len(missing))
		if err := client.InstallPackage(ctx, credential, requirement.VersionID, requirement.SecurityType); err != nil {
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
