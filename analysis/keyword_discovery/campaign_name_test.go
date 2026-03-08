package appleadsanalysiskeyworddiscovery

import "testing"

func TestIsDiscoveryCampaign(t *testing.T) {
	tests := []struct {
		name        string
		isDiscovery bool
	}{
		{"", false},
		{"Search Results UK - Discovery", true},
		{"Search Results Discovery SG", true},
		{"Search Results UK", false},
		{"Search Results US", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if v := isDiscoveryCampaign(tc.name); v != tc.isDiscovery {
				t.Error(v, tc.isDiscovery, tc.name)
			}
		})
	}
}

func TestTargetedCampaignName(t *testing.T) {
	discoveryCampaigns := []string{
		"Search Results UK - Discovery",
	}

	tests := []struct {
		name string
		key  string
	}{
		{"", ""},
		{"Search Results UK - Discovery", "Search Results UK"},
		{"Search Results UK Banana - Discovery", "Search Results UK"},
		{"Search Results UK", "Search Results UK"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if key := targetedCampaignName(tc.name, discoveryCampaigns); key != tc.key {
				t.Error(key, tc.key)
			}
		})
	}
}
