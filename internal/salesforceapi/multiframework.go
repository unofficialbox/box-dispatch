package salesforceapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MultiFrameworkEligibility describes the org properties Salesforce uses to
// decide whether UIBundle metadata is available.
type MultiFrameworkEligibility struct {
	InstanceName     string
	LanguageLocale   string
	APIVersion       string
	Hyperforce       bool
	EnglishDefault   bool
	SupportedRelease bool
}

// ReadMultiFrameworkEligibility reads the target org instead of inferring
// support from the login hostname, which does not expose infrastructure type.
func (c *Client) ReadMultiFrameworkEligibility(ctx context.Context, credential Credential) (MultiFrameworkEligibility, error) {
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return MultiFrameworkEligibility{}, err
	}
	var result struct {
		Records []struct {
			InstanceName      string `json:"InstanceName"`
			LanguageLocaleKey string `json:"LanguageLocaleKey"`
		} `json:"records"`
	}
	if err := c.query(ctx, credential, version, "SELECT InstanceName, LanguageLocaleKey FROM Organization", &result); err != nil {
		return MultiFrameworkEligibility{}, fmt.Errorf("read Salesforce Multi-Framework compatibility: %w", err)
	}
	if len(result.Records) != 1 {
		return MultiFrameworkEligibility{}, fmt.Errorf("read Salesforce Multi-Framework compatibility: Salesforce returned %d Organization records", len(result.Records))
	}
	record := result.Records[0]
	instanceName := strings.ToUpper(strings.TrimSpace(record.InstanceName))
	languageLocale := strings.TrimSpace(record.LanguageLocaleKey)
	apiVersion := strings.TrimPrefix(version, "v")
	apiVersionNumber, _ := strconv.ParseFloat(apiVersion, 64)
	if instanceName == "" {
		return MultiFrameworkEligibility{}, fmt.Errorf("read Salesforce Multi-Framework compatibility: Salesforce returned no instance name")
	}
	if languageLocale == "" {
		return MultiFrameworkEligibility{}, fmt.Errorf("read Salesforce Multi-Framework compatibility: Salesforce returned no default language")
	}
	return MultiFrameworkEligibility{
		InstanceName:     instanceName,
		LanguageLocale:   languageLocale,
		APIVersion:       apiVersion,
		Hyperforce:       isHyperforceInstanceName(instanceName),
		EnglishDefault:   isEnglishLanguageLocale(languageLocale),
		SupportedRelease: apiVersionNumber >= 67,
	}, nil
}

func isEnglishLanguageLocale(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "en" || strings.HasPrefix(value, "en_") || strings.HasPrefix(value, "en-")
}

func isHyperforceInstanceName(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	letters := 0
	for letters < len(value) && value[letters] >= 'A' && value[letters] <= 'Z' {
		letters++
	}
	return letters == 3 && letters < len(value) && value[letters] >= '0' && value[letters] <= '9'
}
