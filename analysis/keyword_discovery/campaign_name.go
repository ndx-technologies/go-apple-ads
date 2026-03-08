package appleadsanalysiskeyworddiscovery

import "strings"

const discoverySuffix = " - Discovery"

func isDiscoveryCampaign(name string) bool { return strings.Contains(name, "Discovery") }

func targetedCampaignName(name string, discoveryCampaigns []string) string {
	for _, d := range discoveryCampaigns {
		prefix := strings.TrimSpace(strings.TrimSuffix(d, discoverySuffix))
		if strings.HasPrefix(name, prefix) {
			return prefix
		}
	}
	return ""
}
