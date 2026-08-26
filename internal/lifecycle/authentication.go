package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

// ValidateProviderAuthentication proves that the selected provider session is
// usable before validation starts reading or comparing deployment components.
func ValidateProviderAuthentication(ctx context.Context, provider string) error {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "box":
		sdk, err := newBoxSDK()
		if err != nil {
			return err
		}
		user, err := sdk.client.Users.GetMe(ctx, nil)
		if err != nil {
			return fmt.Errorf("Box authentication check failed: %w", err)
		}
		if strings.TrimSpace(user.Id) == "" {
			return fmt.Errorf("Box authentication check returned no acting user")
		}
		return nil
	case "salesforce":
		settings, err := shellstate.LoadConnectionSettings()
		if err != nil {
			return fmt.Errorf("load the selected Salesforce connection: %w", err)
		}
		settings = settings.HydrateSalesforceOrgs()
		if !settings.HasSalesforceREST() {
			return fmt.Errorf("connect a Salesforce org in Dispatch before validation")
		}
		if _, err := newSalesforceRESTSession(settings).check(ctx, nil); err != nil {
			return fmt.Errorf("Salesforce authentication check failed: %w", err)
		}
		return nil
	default:
		return nil
	}
}
