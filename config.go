package goappleads

import (
	geo "github.com/ndx-technologies/geo"
)

type CampaignConfig struct {
	ID        CampaignID      `json:"id"`
	Name      string          `json:"name"`
	AdGroups  []AdGroupConfig `json:"adgroups"`
	Countries []geo.Country   `json:"countries"`
	Status    Status          `json:"status"`
}

func (s CampaignConfig) IsZero() bool { return s.ID == "" && s.Name == "" && len(s.AdGroups) == 0 }

type AdGroupConfig struct {
	ID            AdGroupID `json:"id"`
	Name          string    `json:"name"`
	Status        Status    `json:"status"`
	SearchMatch   bool      `json:"search_match"`
	DefaultMaxBid float64   `json:"default_max_bid"`
}

type Config struct {
	Campaigns []CampaignConfig `json:"campaigns"`

	campaignByID        map[CampaignID]CampaignConfig
	campaignByAdGroupID map[AdGroupID]CampaignID
	adgroupByID         map[AdGroupID]AdGroupConfig
}

func (c *Config) Init() {
	c.campaignByID = make(map[CampaignID]CampaignConfig)
	c.campaignByAdGroupID = make(map[AdGroupID]CampaignID)
	c.adgroupByID = make(map[AdGroupID]AdGroupConfig)

	for _, camp := range c.Campaigns {
		c.campaignByID[camp.ID] = camp
		for _, ag := range camp.AdGroups {
			c.adgroupByID[ag.ID] = ag
			c.campaignByAdGroupID[ag.ID] = camp.ID
		}
	}
}

func (s Config) GetCampaign(id CampaignID) CampaignConfig { return s.campaignByID[id] }

func (s Config) GetAdGroup(id AdGroupID) AdGroupConfig { return s.adgroupByID[id] }

func (s Config) GetCampaignForAdGroup(id AdGroupID) CampaignConfig {
	return s.campaignByID[s.campaignByAdGroupID[id]]
}

func (s Config) IsAdGroupPaused(adgroup AdGroupID) bool {
	if a := s.GetAdGroup(adgroup); !a.ID.IsZero() && a.Status == Paused {
		return true
	}
	if c := s.GetCampaignForAdGroup(adgroup); !c.ID.IsZero() && c.Status == Paused {
		return true
	}
	return false
}

func (s Config) IsAdGroupPausedAll(adgroups []AdGroupID) bool {
	if len(adgroups) == 0 {
		return false
	}
	for _, agid := range adgroups {
		if !s.IsAdGroupPaused(agid) {
			return false
		}
	}
	return true
}
