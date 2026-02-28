package goappleads

import geo "github.com/ndx-technologies/geo"

type CampaignConfig struct {
	ID        CampaignID      `json:"id"`
	Name      string          `json:"name"`
	AdGroups  []AdGroupConfig `json:"ad_groups"`
	Countries []geo.Country   `json:"countries"`
	Status    Status          `json:"status"`
}

func (s CampaignConfig) IsZero() bool { return s.ID == "" && s.Name == "" && len(s.AdGroups) == 0 }

type AdGroupConfig struct {
	ID     AdGroupID `json:"id"`
	Name   string    `json:"name"`
	Status Status    `json:"status"`
}

type Config struct {
	Campaigns []CampaignConfig `json:"campaigns"`

	campaignByID map[CampaignID]CampaignConfig
	adgroupByID  map[AdGroupID]AdGroupConfig
}

func (c *Config) Init() {
	c.campaignByID = make(map[CampaignID]CampaignConfig)
	c.adgroupByID = make(map[AdGroupID]AdGroupConfig)
	for _, camp := range c.Campaigns {
		c.campaignByID[camp.ID] = camp
		for _, ag := range camp.AdGroups {
			c.adgroupByID[ag.ID] = ag
		}
	}
}

func (s Config) GetCampaign(id CampaignID) CampaignConfig { return s.campaignByID[id] }

func (s Config) GetAdGroup(id AdGroupID) AdGroupConfig { return s.adgroupByID[id] }
