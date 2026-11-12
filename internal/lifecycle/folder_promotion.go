package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
)

type folderPromotionSpec struct {
	DestinationRootID string                `json:"destination_root_id"`
	Items             []folderPromotionItem `json:"items"`
}

type folderPromotionItem struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Source string `json:"source,omitempty"`
	URL    string `json:"url,omitempty"`
}

func readFolderPromotion(root string) (folderPromotionSpec, error) {
	data, err := readBoxConfigObject(filepath.Join(root, "config", "box"), "folder-promotion")
	if err != nil {
		return folderPromotionSpec{}, err
	}
	var spec folderPromotionSpec
	if raw := data["destination_root_id"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &spec.DestinationRootID); err != nil {
			return spec, fmt.Errorf("parse folder-promotion destination root: %w", err)
		}
	}
	if err := json.Unmarshal(data["items"], &spec.Items); err != nil {
		return spec, fmt.Errorf("parse folder-promotion items: %w", err)
	}
	if strings.TrimSpace(spec.DestinationRootID) == "" {
		spec.DestinationRootID = "0"
	}
	for index := range spec.Items {
		clean, err := cleanPromotionPath(spec.Items[index].Path)
		if err != nil {
			return spec, err
		}
		spec.Items[index].Path = clean
		switch spec.Items[index].Type {
		case "folder":
		case "file":
			if strings.TrimSpace(spec.Items[index].Source) == "" {
				return spec, fmt.Errorf("promoted file %s has no packaged source", clean)
			}
			if _, err := promotionSourcePath(root, spec.Items[index].Source); err != nil {
				return spec, fmt.Errorf("promoted file %s: %w", clean, err)
			}
		case "web_link":
			if err := validatePromotionURL(spec.Items[index].URL); err != nil {
				return spec, fmt.Errorf("promoted web link %s: %w", clean, err)
			}
		default:
			return spec, fmt.Errorf("unsupported folder promotion type %q", spec.Items[index].Type)
		}
	}
	sort.SliceStable(spec.Items, func(i, j int) bool {
		leftDepth, rightDepth := strings.Count(spec.Items[i].Path, "/"), strings.Count(spec.Items[j].Path, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if spec.Items[i].Type != spec.Items[j].Type {
			return spec.Items[i].Type == "folder"
		}
		return spec.Items[i].Path < spec.Items[j].Path
	})
	return spec, nil
}

func cleanPromotionPath(value string) (string, error) {
	raw := strings.TrimSpace(value)
	for _, segment := range strings.Split(strings.TrimPrefix(raw, "/"), "/") {
		if segment == ".." {
			return "", fmt.Errorf("folder promotion path %q is invalid", value)
		}
	}
	clean := path.Clean("/" + strings.TrimPrefix(raw, "/"))
	if clean == "/" {
		return "", fmt.Errorf("folder promotion path %q is invalid", value)
	}
	return clean, nil
}

func promotionSourcePath(root, source string) (string, error) {
	raw := strings.TrimSpace(source)
	if raw == "" || filepath.IsAbs(raw) {
		return "", fmt.Errorf("packaged source %q is invalid", source)
	}
	for _, segment := range strings.FieldsFunc(filepath.ToSlash(raw), func(r rune) bool { return r == '/' }) {
		if segment == ".." {
			return "", fmt.Errorf("packaged source %q leaves the deployment package", source)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." {
		return "", fmt.Errorf("packaged source %q is invalid", source)
	}
	return filepath.Join(root, clean), nil
}

func validatePromotionURL(value string) error {
	target, err := url.Parse(strings.TrimSpace(value))
	if err != nil || target.Host == "" || (!strings.EqualFold(target.Scheme, "http") && !strings.EqualFold(target.Scheme, "https")) {
		return fmt.Errorf("web link target must be an absolute HTTP or HTTPS URL")
	}
	return nil
}

func folderPromotionComponent(item folderPromotionItem) string {
	typeName := map[string]string{"folder": "Folder", "file": "File", "web_link": "Web Link"}[item.Type]
	return typeName + ":" + item.Path
}

func folderPromotionComponents(root string) ([]string, error) {
	spec, err := readFolderPromotion(root)
	if err != nil {
		return nil, err
	}
	components := make([]string, 0, len(spec.Items))
	for _, item := range spec.Items {
		components = append(components, folderPromotionComponent(item))
	}
	return components, nil
}

func validateFolderPromotion(ctx context.Context, root string, api boxAPI, item *Item, report Reporter, components []string) error {
	spec, err := readFolderPromotion(root)
	if err != nil {
		return err
	}
	for _, promoted := range spec.Items {
		component := folderPromotionComponent(promoted)
		report.component(component, ProgressRunning, "Inspecting the destination path", componentIndex(components, component), len(components))
		present, deployable, err := inspectFolderPromotionItem(ctx, root, api, spec.DestinationRootID, promoted)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", component, err)
		}
		classifyBoxComponent(item, component, present, deployable)
		if deployable {
			if err := appendFolderPromotionChange(item, promoted); err != nil {
				return err
			}
		}
		report.component(component, ProgressCompleted, validationResultMessage(present, deployable), componentIndex(components, component)+1, len(components))
	}
	return nil
}

func inspectFolderPromotionItem(ctx context.Context, root string, api boxAPI, rootID string, item folderPromotionItem) (present, deployable bool, err error) {
	parentID, parentFound, err := resolvePromotionFolder(ctx, api, rootID, path.Dir(item.Path))
	if err != nil {
		return false, false, err
	}
	if !parentFound {
		return false, true, nil
	}
	name := path.Base(item.Path)
	switch item.Type {
	case "folder":
		_, found, err := api.findFolder(ctx, parentID, name)
		return found, !found, err
	case "file":
		_, remoteSHA, found, err := api.fileDigest(ctx, parentID, name)
		if err != nil || !found {
			return false, !found, err
		}
		source, err := promotionSourcePath(root, item.Source)
		if err != nil {
			return false, false, err
		}
		localSHA, err := localFileSHA1(source)
		if err != nil {
			return false, false, err
		}
		return remoteSHA == "" || remoteSHA == localSHA, false, nil
	case "web_link":
		_, target, found, err := api.findWebLink(ctx, parentID, name)
		if err != nil || !found {
			return false, !found, err
		}
		return target == strings.TrimSpace(item.URL), false, nil
	default:
		return false, false, fmt.Errorf("unsupported type %q", item.Type)
	}
}

func appendFolderPromotionChange(item *Item, promoted folderPromotionItem) error {
	after, err := json.MarshalIndent(map[string]string{"path": promoted.Path, "type": promoted.Type}, "", "  ")
	if err != nil {
		return err
	}
	item.Changes = append(item.Changes, salesforceapi.MetadataFileDiff{
		Component: folderPromotionComponent(promoted), Path: "folder-promotion" + promoted.Path + ".json",
		Kind: "add", After: string(after), Previewable: true,
	})
	return nil
}

func deployFolderPromotion(ctx context.Context, root string, api boxAPI, deployable []string, report Reporter) ([]string, []ResourceReference, error) {
	spec, err := readFolderPromotion(root)
	if err != nil {
		return nil, nil, err
	}
	deployed := []string{}
	resources := []ResourceReference{}
	for _, promoted := range spec.Items {
		component := folderPromotionComponent(promoted)
		if !slices.Contains(deployable, component) {
			continue
		}
		parentID, createdParents, err := ensurePromotionFolder(ctx, api, spec.DestinationRootID, path.Dir(promoted.Path))
		if err != nil {
			return deployed, resources, fmt.Errorf("prepare parent for %s: %w", component, err)
		}
		resources = append(resources, createdParents...)
		name := path.Base(promoted.Path)
		switch promoted.Type {
		case "folder":
			if _, found, findErr := api.findFolder(ctx, parentID, name); findErr != nil {
				return deployed, resources, findErr
			} else if found {
				return deployed, resources, fmt.Errorf("destination folder %s appeared after validation; validate again", promoted.Path)
			}
			report.step("Creating Box folder " + promoted.Path)
			id, createErr := api.ensureFolder(ctx, parentID, name)
			if createErr != nil {
				return deployed, resources, createErr
			}
			resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "folder", Name: name, ID: id, URL: "https://app.box.com/folder/" + id})
		case "file":
			if _, found, findErr := api.findFile(ctx, parentID, name); findErr != nil {
				return deployed, resources, findErr
			} else if found {
				return deployed, resources, fmt.Errorf("destination file %s appeared after validation; validate again", promoted.Path)
			}
			report.step("Uploading Box file " + promoted.Path)
			source, sourceErr := promotionSourcePath(root, promoted.Source)
			if sourceErr != nil {
				return deployed, resources, sourceErr
			}
			id, uploadErr := api.uploadFile(ctx, parentID, source)
			if uploadErr != nil {
				return deployed, resources, uploadErr
			}
			resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "file", Name: name, ID: id, URL: "https://app.box.com/file/" + id})
		case "web_link":
			if _, _, found, findErr := api.findWebLink(ctx, parentID, name); findErr != nil {
				return deployed, resources, findErr
			} else if found {
				return deployed, resources, fmt.Errorf("destination web link %s appeared after validation; validate again", promoted.Path)
			}
			report.step("Creating Box web link " + promoted.Path)
			id, createErr := api.createWebLink(ctx, parentID, name, promoted.URL)
			if createErr != nil {
				return deployed, resources, createErr
			}
			resources = append(resources, ResourceReference{Provider: "box", Component: component, Kind: "web_link", Name: name, ID: id, URL: "https://app.box.com/web_link/" + id})
		}
		deployed = append(deployed, component)
	}
	return deployed, dedupeResources(resources), nil
}

func resolvePromotionFolder(ctx context.Context, api boxAPI, rootID, folderPath string) (string, bool, error) {
	current := strings.TrimSpace(rootID)
	if current == "" {
		current = "0"
	}
	clean := path.Clean("/" + strings.TrimPrefix(folderPath, "/"))
	if clean == "/" || clean == "." {
		return current, true, nil
	}
	for _, segment := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		next, found, err := api.findFolder(ctx, current, segment)
		if err != nil || !found {
			return "", found, err
		}
		current = next
	}
	return current, true, nil
}

func ensurePromotionFolder(ctx context.Context, api boxAPI, rootID, folderPath string) (string, []ResourceReference, error) {
	current := strings.TrimSpace(rootID)
	if current == "" {
		current = "0"
	}
	clean := path.Clean("/" + strings.TrimPrefix(folderPath, "/"))
	if clean == "/" || clean == "." {
		return current, nil, nil
	}
	created := []ResourceReference{}
	currentPath := ""
	for _, segment := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		currentPath += "/" + segment
		next, found, err := api.findFolder(ctx, current, segment)
		if err != nil {
			return "", created, err
		}
		if !found {
			next, err = api.ensureFolder(ctx, current, segment)
			if err != nil {
				return "", created, err
			}
			component := "Folder:" + currentPath
			created = append(created, ResourceReference{Provider: "box", Component: component, Kind: "folder", Name: segment, ID: next, URL: "https://app.box.com/folder/" + next})
		}
		current = next
	}
	return current, created, nil
}

func dedupeResources(resources []ResourceReference) []ResourceReference {
	seen := map[string]bool{}
	result := make([]ResourceReference, 0, len(resources))
	for _, resource := range resources {
		key := resource.Kind + ":" + resource.ID
		if resource.ID == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, resource)
	}
	return result
}
