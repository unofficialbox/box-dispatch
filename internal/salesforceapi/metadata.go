package salesforceapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type MetadataProgress struct {
	ID                 string
	Done               bool
	Success            bool
	Status             string
	ComponentsDeployed int
	ComponentsTotal    int
	ComponentFailures  []string
}

type MetadataProgressFunc func(MetadataProgress)

// MetadataInventoryProgressFunc reports one completed metadata-type query. A
// type is complete even when fullNames is empty, which lets browser clients
// resolve every requested source component as present or missing while the
// larger inventory is still running.
type MetadataInventoryProgressFunc func(metadataType string, fullNames []string)

func (c *Client) ListMetadata(ctx context.Context, credential Credential, apiVersion string, metadataTypes []string) (map[string]bool, error) {
	return c.ListMetadataWithProgress(ctx, credential, apiVersion, metadataTypes, nil)
}

func (c *Client) ListMetadataWithProgress(ctx context.Context, credential Credential, apiVersion string, metadataTypes []string, progress MetadataInventoryProgressFunc) (map[string]bool, error) {
	if !credential.Valid() {
		return nil, fmt.Errorf("Salesforce connection is incomplete")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	types := append([]string(nil), metadataTypes...)
	sort.Strings(types)
	for start := 0; start < len(types); start += 3 {
		end := min(start+3, len(types))
		queries := strings.Builder{}
		for _, metadataType := range types[start:end] {
			queries.WriteString("<met:queries><met:type>")
			_ = xml.EscapeText(&queries, []byte(metadataType))
			queries.WriteString("</met:type></met:queries>")
		}
		body := queries.String() + "<met:asOfVersion>" + apiVersion + "</met:asOfVersion>"
		data, err := c.metadataSOAP(ctx, credential, apiVersion, "listMetadata", body)
		if err != nil {
			return nil, fmt.Errorf("list Salesforce metadata: %w", err)
		}
		var envelope struct {
			Body struct {
				Response struct {
					Results []struct {
						FullName string `xml:"fullName"`
						Type     string `xml:"type"`
					} `xml:"result"`
				} `xml:"listMetadataResponse"`
			} `xml:"Body"`
		}
		if err := xml.Unmarshal(data, &envelope); err != nil {
			return nil, fmt.Errorf("parse Salesforce metadata inventory: %w", err)
		}
		foundByType := make(map[string][]string, end-start)
		for _, property := range envelope.Body.Response.Results {
			if property.Type != "" && property.FullName != "" {
				result[property.Type+":"+property.FullName] = true
				foundByType[property.Type] = append(foundByType[property.Type], property.FullName)
			}
		}
		if progress != nil {
			for _, metadataType := range types[start:end] {
				fullNames := append([]string(nil), foundByType[metadataType]...)
				sort.Strings(fullNames)
				progress(metadataType, fullNames)
			}
		}
	}
	return result, nil
}

func (c *Client) DeployMetadata(ctx context.Context, credential Credential, apiVersion string, zipData []byte, progress MetadataProgressFunc) (MetadataProgress, error) {
	if !credential.Valid() {
		return MetadataProgress{}, fmt.Errorf("Salesforce connection is incomplete")
	}
	if len(zipData) == 0 {
		return MetadataProgress{}, fmt.Errorf("Salesforce metadata package is empty")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return MetadataProgress{}, err
	}
	body := "<met:ZipFile>" + base64.StdEncoding.EncodeToString(zipData) + "</met:ZipFile>" +
		"<met:DeployOptions><met:allowMissingFiles>false</met:allowMissingFiles><met:autoUpdatePackage>false</met:autoUpdatePackage>" +
		"<met:checkOnly>false</met:checkOnly><met:ignoreWarnings>false</met:ignoreWarnings><met:performRetrieve>false</met:performRetrieve>" +
		"<met:purgeOnDelete>false</met:purgeOnDelete><met:rollbackOnError>true</met:rollbackOnError><met:singlePackage>true</met:singlePackage>" +
		"<met:testLevel>NoTestRun</met:testLevel></met:DeployOptions>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "deploy", body)
	if err != nil {
		return MetadataProgress{}, fmt.Errorf("start Salesforce metadata deployment: %w", err)
	}
	var started struct {
		Body struct {
			Response struct {
				Result struct {
					ID    string `xml:"id"`
					State string `xml:"state"`
				} `xml:"result"`
			} `xml:"deployResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &started); err != nil || started.Body.Response.Result.ID == "" {
		return MetadataProgress{}, fmt.Errorf("parse Salesforce metadata deployment response")
	}
	current := MetadataProgress{ID: started.Body.Response.Result.ID, Status: started.Body.Response.Result.State}
	if progress != nil {
		progress(current)
	}
	for {
		select {
		case <-ctx.Done():
			return current, fmt.Errorf("wait for Salesforce metadata deployment: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		current, err = c.checkMetadataDeploy(ctx, credential, apiVersion, current.ID)
		if err != nil {
			return current, err
		}
		if progress != nil {
			progress(current)
		}
		if !current.Done {
			continue
		}
		if !current.Success {
			detail := strings.Join(current.ComponentFailures, "; ")
			if detail == "" {
				detail = current.Status
			}
			return current, fmt.Errorf("Salesforce metadata deployment failed: %s", detail)
		}
		return current, nil
	}
}

func (c *Client) checkMetadataDeploy(ctx context.Context, credential Credential, apiVersion, id string) (MetadataProgress, error) {
	body := "<met:asyncProcessId>" + id + "</met:asyncProcessId><met:includeDetails>true</met:includeDetails>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "checkDeployStatus", body)
	if err != nil {
		return MetadataProgress{ID: id}, fmt.Errorf("read Salesforce metadata deployment status: %w", err)
	}
	var envelope struct {
		Body struct {
			Response struct {
				Result struct {
					ID       string `xml:"id"`
					Done     bool   `xml:"done"`
					Success  bool   `xml:"success"`
					Status   string `xml:"status"`
					Deployed int    `xml:"numberComponentsDeployed"`
					Total    int    `xml:"numberComponentsTotal"`
					Details  struct {
						Failures []struct {
							FullName string `xml:"fullName"`
							Problem  string `xml:"problem"`
						} `xml:"componentFailures"`
					} `xml:"details"`
				} `xml:"result"`
			} `xml:"checkDeployStatusResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return MetadataProgress{ID: id}, fmt.Errorf("parse Salesforce metadata deployment status: %w", err)
	}
	result := envelope.Body.Response.Result
	current := MetadataProgress{ID: firstNonEmpty(result.ID, id), Done: result.Done, Success: result.Success, Status: result.Status, ComponentsDeployed: result.Deployed, ComponentsTotal: result.Total}
	for _, failure := range result.Details.Failures {
		message := strings.TrimSpace(failure.Problem)
		if failure.FullName != "" {
			message = failure.FullName + ": " + message
		}
		if message != "" {
			current.ComponentFailures = append(current.ComponentFailures, message)
		}
	}
	return current, nil
}

func (c *Client) metadataVersion(ctx context.Context, credential Credential, version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version != "" {
		return version, nil
	}
	latest, err := c.latestAPIVersion(ctx, credential)
	return strings.TrimPrefix(latest, "v"), err
}

func (c *Client) metadataSOAP(ctx context.Context, credential Credential, apiVersion, operation, operationBody string) ([]byte, error) {
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:met="http://soap.sforce.com/2006/04/metadata">` +
		`<soapenv:Header><met:SessionHeader><met:sessionId>` + xmlEscape(credential.AccessToken) + `</met:sessionId></met:SessionHeader></soapenv:Header>` +
		`<soapenv:Body><met:` + operation + `>` + operationBody + `</met:` + operation + `></soapenv:Body></soapenv:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(credential.InstanceURL, "/")+"/services/Soap/m/"+apiVersion, strings.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("SOAPAction", operation)
	response, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || bytes.Contains(data, []byte("<faultcode>")) {
		var fault struct {
			Body struct {
				Fault struct {
					Code   string `xml:"faultcode"`
					String string `xml:"faultstring"`
				} `xml:"Fault"`
			} `xml:"Body"`
		}
		_ = xml.Unmarshal(data, &fault)
		message := strings.TrimSpace(fault.Body.Fault.String)
		if message == "" {
			message = fmt.Sprintf("HTTP %d from Salesforce Metadata API", response.StatusCode)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return data, nil
}

func xmlEscape(value string) string {
	var output strings.Builder
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}
