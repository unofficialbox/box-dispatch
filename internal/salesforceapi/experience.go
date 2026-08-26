package salesforceapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ExperienceSite struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"siteUrl"`
}

type ExperiencePublishProgressFunc func(status string)

// ExperienceEmployeePath resolves the internal Salesforce handoff for a
// published Experience Cloud network. Opening this path through frontdoor.jsp
// lets an authenticated Salesforce employee enter the site without a second
// username and password prompt.
func (c *Client) ExperienceEmployeePath(ctx context.Context, credential Credential, apiVersion, networkID string) (string, error) {
	credential, apiVersion, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return "", err
	}
	var result struct {
		Communities []ExperienceSite `json:"communities"`
	}
	path := "/services/data/" + apiVersion + "/connect/communities"
	if err := c.doJSON(ctx, credential, http.MethodGet, path, nil, &result); err != nil {
		return "", fmt.Errorf("list Salesforce Experience Cloud sites: %w", err)
	}
	var siteURL string
	for _, site := range result.Communities {
		if strings.EqualFold(strings.TrimSpace(site.ID), strings.TrimSpace(networkID)) {
			siteURL = strings.TrimSpace(site.URL)
			break
		}
	}
	if siteURL == "" {
		return "", fmt.Errorf("Salesforce Experience Cloud site %q was not found", networkID)
	}
	parsed, err := url.Parse(siteURL)
	if err != nil {
		return "", fmt.Errorf("Salesforce Experience Cloud site URL is invalid")
	}
	prefix := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if prefix == "" || strings.Contains(prefix, "/") {
		return "", fmt.Errorf("Salesforce Experience Cloud site URL has no usable path")
	}
	var sites struct {
		Records []struct {
			ID string `json:"Id"`
		} `json:"records"`
	}
	soql := "SELECT Id FROM Site WHERE UrlPathPrefix='" + escapeSOQL(prefix) + "' AND Status='Active' LIMIT 2"
	if err := c.query(ctx, credential, apiVersion, soql, &sites); err != nil {
		return "", fmt.Errorf("resolve Salesforce Experience Cloud employee login: %w", err)
	}
	if len(sites.Records) != 1 || strings.TrimSpace(sites.Records[0].ID) == "" {
		return "", fmt.Errorf("Salesforce did not return exactly one active site for %q", prefix)
	}
	return "/servlet/networks/session/create?" + url.Values{"site": {sites.Records[0].ID}}.Encode(), nil
}

// PublishExperienceSite publishes the Experience Cloud site created by the
// packaged external-app metadata and waits for Salesforce's async publish job.
func (c *Client) PublishExperienceSite(ctx context.Context, credential Credential, apiVersion, name string, progress ExperiencePublishProgressFunc) (ExperienceSite, error) {
	credential, apiVersion, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return ExperienceSite{}, err
	}
	site, err := c.findExperienceSite(ctx, credential, apiVersion, name)
	if err != nil {
		return ExperienceSite{}, err
	}
	var published struct {
		ID      string `json:"id"`
		JobID   string `json:"jobId"`
		Message string `json:"message"`
		Name    string `json:"name"`
		URL     string `json:"url"`
	}
	path := "/services/data/" + apiVersion + "/connect/communities/" + url.PathEscape(site.ID) + "/publish"
	if err := c.doJSON(ctx, credential, http.MethodPost, path, nil, &published); err != nil {
		return ExperienceSite{}, fmt.Errorf("publish Salesforce Experience Cloud site %q: %w", name, err)
	}
	if progress != nil {
		progress("queued")
	}
	if strings.TrimSpace(published.JobID) != "" {
		if err := c.waitForExperiencePublish(ctx, credential, apiVersion, published.JobID, progress); err != nil {
			return ExperienceSite{}, err
		}
	}
	if strings.TrimSpace(published.URL) != "" {
		site.URL = published.URL
	}
	site.Status = "Live"
	return site, nil
}

func (c *Client) findExperienceSite(ctx context.Context, credential Credential, apiVersion, name string) (ExperienceSite, error) {
	var result struct {
		Communities []ExperienceSite `json:"communities"`
	}
	path := "/services/data/" + apiVersion + "/connect/communities"
	if err := c.doJSON(ctx, credential, http.MethodGet, path, nil, &result); err != nil {
		return ExperienceSite{}, fmt.Errorf("list Salesforce Experience Cloud sites: %w", err)
	}
	for _, site := range result.Communities {
		if strings.EqualFold(strings.TrimSpace(site.Name), strings.TrimSpace(name)) {
			return site, nil
		}
	}
	return ExperienceSite{}, fmt.Errorf("Salesforce Experience Cloud site %q was not created by the metadata deployment", name)
}

func (c *Client) waitForExperiencePublish(ctx context.Context, credential Credential, apiVersion, jobID string, progress ExperiencePublishProgressFunc) error {
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	for {
		var result struct {
			Records []struct {
				Status string `json:"Status"`
			} `json:"records"`
		}
		// BackgroundOperation exposes Status consistently, while Message is not
		// available in every Salesforce edition/API shape.
		soql := "SELECT Status FROM BackgroundOperation WHERE Id = '" + strings.ReplaceAll(jobID, "'", "\\'") + "'"
		if err := c.query(ctx, credential, apiVersion, soql, &result); err != nil {
			return fmt.Errorf("read Salesforce Experience Cloud publish status: %w", err)
		}
		if len(result.Records) == 0 {
			return fmt.Errorf("Salesforce returned no publish job for %s", jobID)
		}
		status := strings.TrimSpace(result.Records[0].Status)
		if progress != nil {
			progress(status)
		}
		switch strings.ToLower(status) {
		case "complete", "completed", "succeeded", "success":
			return nil
		case "error", "failed", "failure", "aborted":
			return fmt.Errorf("Salesforce Experience Cloud publish failed with status %s", status)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Salesforce Experience Cloud publish: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("Salesforce Experience Cloud publish did not finish within 15 minutes")
		case <-time.After(c.pollInterval()):
		}
	}
}
