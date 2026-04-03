package appleadsanalysiskeywordcptoutliers_test

import (
	"testing"

	goappleads "github.com/ndx-technologies/go-apple-ads"
	appleadsanalysiskeywordcptoutliers "github.com/ndx-technologies/go-apple-ads/analysis/keyword_cpt_outliers"
)

func TestAnalyze(t *testing.T) {
	t.Run("when adgroup has keyword cpt above 3x p50 then flags outlier", func(t *testing.T) {
		config := &goappleads.Config{Campaigns: []goappleads.CampaignConfig{{
			ID:     "camp-1",
			Name:   "Campaign",
			Status: goappleads.Enabled,
			AdGroups: []goappleads.AdGroupConfig{{
				ID:     "ag-1",
				Name:   "Ad Group",
				Status: goappleads.Enabled,
			}},
		}}}
		config.Init()

		keywordsDB := &goappleads.KeywordCSVDB{Keywords: map[goappleads.KeywordID]goappleads.KeywordInfo{
			"kw-1": {ID: "kw-1", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "alpha", MatchType: goappleads.Exact, Status: goappleads.Active},
			"kw-2": {ID: "kw-2", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "beta", MatchType: goappleads.Exact, Status: goappleads.Active},
			"kw-3": {ID: "kw-3", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "gamma", MatchType: goappleads.Exact, Status: goappleads.Active},
		}}

		issues, summary := appleadsanalysiskeywordcptoutliers.Analyze(config, keywordsDB, map[goappleads.KeywordID]appleadsanalysiskeywordcptoutliers.KeywordStats{
			"kw-1": {Spend: 10, Taps: 10},
			"kw-2": {Spend: 20, Taps: 10},
			"kw-3": {Spend: 80, Taps: 10},
		}, 3)

		if summary.NumKeywords != 3 || summary.NumAdGroups != 1 {
			t.Error(summary)
		}
		if len(issues) != 1 {
			t.Error(len(issues))
		}
		if issues[0].Keyword.ID != "kw-3" {
			t.Error(issues[0].Keyword.ID)
		}
		if issues[0].P50CPT != 2 {
			t.Error(issues[0].P50CPT)
		}
		if issues[0].Ratio != 4 {
			t.Error(issues[0].Ratio)
		}
	})

	t.Run("when keyword is paused negative or has no taps then skips it", func(t *testing.T) {
		config := &goappleads.Config{Campaigns: []goappleads.CampaignConfig{{
			ID:     "camp-1",
			Name:   "Campaign",
			Status: goappleads.Enabled,
			AdGroups: []goappleads.AdGroupConfig{{
				ID:     "ag-1",
				Name:   "Ad Group",
				Status: goappleads.Enabled,
			}},
		}}}
		config.Init()

		keywordsDB := &goappleads.KeywordCSVDB{Keywords: map[goappleads.KeywordID]goappleads.KeywordInfo{
			"kw-1": {ID: "kw-1", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "alpha", MatchType: goappleads.Exact, Status: goappleads.Active},
			"kw-2": {ID: "kw-2", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "beta", MatchType: goappleads.Exact, Status: goappleads.Paused},
			"kw-3": {ID: "kw-3", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "gamma", MatchType: goappleads.Exact, Status: goappleads.Active, IsNegative: true},
			"kw-4": {ID: "kw-4", CampaignID: "camp-1", AdGroupID: "ag-1", Keyword: "delta", MatchType: goappleads.Exact, Status: goappleads.Active},
		}}

		issues, summary := appleadsanalysiskeywordcptoutliers.Analyze(config, keywordsDB, map[goappleads.KeywordID]appleadsanalysiskeywordcptoutliers.KeywordStats{
			"kw-1": {Spend: 10, Taps: 10},
			"kw-2": {Spend: 99, Taps: 1},
			"kw-3": {Spend: 99, Taps: 1},
			"kw-4": {Spend: 50, Taps: 0},
		}, 3)

		if summary.NumKeywords != 1 || summary.NumAdGroups != 1 {
			t.Error(summary)
		}
		if len(issues) != 0 {
			t.Error(len(issues))
		}
	})
}
