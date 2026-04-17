package ocicredentials

import (
	"context"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

func (identityConnectionTester) Test(ctx context.Context, provider common.ConfigurationProvider, tenancyOCID string) ([]string, []string, error) {
	client, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, nil, err
	}
	regionsResponse, err := client.ListRegionSubscriptions(ctx, identity.ListRegionSubscriptionsRequest{
		TenancyId: common.String(strings.TrimSpace(tenancyOCID)),
	})
	if err != nil {
		return nil, nil, err
	}
	regions := make([]string, 0, len(regionsResponse.Items))
	for _, item := range regionsResponse.Items {
		name := strings.TrimSpace(valueOrEmpty(item.RegionName))
		if name == "" {
			name = strings.TrimSpace(valueOrEmpty(item.RegionKey))
		}
		if name == "" {
			continue
		}
		regions = append(regions, name)
	}

	availabilityDomains := []string{}
	availabilityDomainsResponse, err := client.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(strings.TrimSpace(tenancyOCID)),
	})
	if err == nil {
		availabilityDomains = make([]string, 0, len(availabilityDomainsResponse.Items))
		for _, item := range availabilityDomainsResponse.Items {
			name := strings.TrimSpace(valueOrEmpty(item.Name))
			if name == "" {
				continue
			}
			availabilityDomains = append(availabilityDomains, name)
		}
	}
	return regions, availabilityDomains, nil
}

func (p parsedCredential) configurationProvider() (common.ConfigurationProvider, error) {
	provider := common.NewRawConfigurationProvider(
		p.TenancyOCID,
		p.UserOCID,
		p.Region,
		p.Fingerprint,
		p.PrivateKeyPEM,
		optionalString(p.Passphrase),
	)
	return provider, nil
}
