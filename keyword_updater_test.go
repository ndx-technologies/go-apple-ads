package goappleads

import (
	_ "embed"
	"slices"
	"sort"
	"strings"
	"testing"
)

//go:embed testdata/keyword_updater_commands.csv
var updaterCommandsCSV string

//go:embed testdata/keyword_updater_negative_commands.csv
var updaterNegativeCommandsCSV string

func TestKeywordUpdater_UpdateCommands(t *testing.T) {
	from := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"2204670085": {ID: "2204670085", Keyword: "banana", MatchType: Exact, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"2204670087": {ID: "2204670087", Keyword: "ajfans", MatchType: Exact, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"2183598762": {ID: "2183598762", Keyword: "99 motorista", MatchType: Exact, Status: Active, Bid: 0.8, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"2175126639": {ID: "2175126639", Keyword: "gas station", MatchType: Broad, Status: Active, Bid: 0.2, CampaignID: "2142845006", AdGroupID: "2143951277"},
	}}

	to := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"2204670085": {ID: "2204670085", Keyword: "banana", MatchType: Exact, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"2204670087": {ID: "2204670087", Keyword: "ajfans", MatchType: Exact, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"2175126639": {ID: "2175126639", Keyword: "gas station", MatchType: Broad, Status: Active, Bid: 0.5, CampaignID: "2142845006", AdGroupID: "2143951277"},
		"new1":       {Keyword: "market", MatchType: Broad, Status: Active, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
	}}

	cmds := KeywordUpdater{From: from, To: to}.UpdateCommands()

	sort.Slice(cmds, func(i, j int) bool {
		if cmds[i].Action != cmds[j].Action {
			return cmds[i].Action < cmds[j].Action
		}
		return cmds[i].Keyword < cmds[j].Keyword
	})

	expected := []KeywordCommand{
		// banana not changed
		{Action: Create, Keyword: "market", MatchType: Broad, Status: Active, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		{Action: Delete, KeywordID: "2183598762", Keyword: "99 motorista", MatchType: Exact, Status: Active, Bid: 0.8, CampaignID: "2142845006", AdGroupID: "2143951277"},
		{Action: Update, KeywordID: "2175126639", Keyword: "gas station", MatchType: Broad, Status: Active, Bid: 0.5, CampaignID: "2142845006", AdGroupID: "2143951277"},
	}

	if !slices.Equal(cmds, expected) {
		t.Error(cmds, expected)
	}
}

func TestPrintCommandsToCSV(t *testing.T) {
	cmds := []KeywordCommand{
		{Action: Create, Keyword: "market", MatchType: Broad, Status: Active, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277"},
		{Action: Update, KeywordID: "2175126639", Keyword: "gas station", MatchType: Broad, Status: Active, Bid: 0.5, CampaignID: "2142845006", AdGroupID: "2143951277"},
		{Action: Delete, KeywordID: "2183598762", Keyword: "99 motorista", MatchType: Exact, Status: Active, Bid: 0.8, CampaignID: "2142845006", AdGroupID: "2143951277"},
	}

	var sb strings.Builder
	if err := PrintCommandsToCSV(&sb, cmds); err != nil {
		t.Fatal(err)
	}

	if got := sb.String(); got != updaterCommandsCSV {
		t.Error(got)
	}
}

func TestKeywordUpdater_UpdateCommands_NormalToNegative(t *testing.T) {
	// Moving a keyword from regular to negative cannot be matched since IsNegative
	// is part of the key. Result: DELETE the regular entry + CREATE the negative entry.
	from := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"111": {ID: "111", Keyword: "spam", MatchType: Broad, Status: Active, Bid: 0.5, CampaignID: "100", AdGroupID: "200", IsNegative: false},
	}}
	to := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"111": {ID: "111", Keyword: "spam", MatchType: Broad, Status: Active, IsNegative: true, CampaignID: "100", AdGroupID: "200"},
	}}

	cmds := KeywordUpdater{From: from, To: to}.UpdateCommands()

	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Action < cmds[j].Action })

	expected := []KeywordCommand{
		{Action: Create, Keyword: "spam", MatchType: Broad, Status: Active, IsNegative: true, CampaignID: "100", AdGroupID: "200"},
		{Action: Delete, KeywordID: "111", Keyword: "spam", MatchType: Broad, Status: Active, Bid: 0.5, IsNegative: false, CampaignID: "100", AdGroupID: "200"},
	}

	if !slices.Equal(cmds, expected) {
		t.Error(cmds, expected)
	}
}

func TestKeywordUpdater_UpdateCommands_NegativeToNormal(t *testing.T) {
	// Moving a keyword from negative back to regular also cannot be matched.
	// Result: DELETE the negative entry + CREATE the regular entry.
	from := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"111": {ID: "111", Keyword: "spam", MatchType: Broad, Status: Active, IsNegative: true, CampaignID: "100", AdGroupID: "200"},
	}}
	to := KeywordCSVDB{Keywords: map[KeywordID]KeywordInfo{
		"111": {ID: "111", Keyword: "spam", MatchType: Broad, Status: Active, Bid: 0.5, IsNegative: false, CampaignID: "100", AdGroupID: "200"},
	}}

	cmds := KeywordUpdater{From: from, To: to}.UpdateCommands()

	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Action < cmds[j].Action })

	expected := []KeywordCommand{
		{Action: Create, Keyword: "spam", MatchType: Broad, Status: Active, Bid: 0.5, IsNegative: false, CampaignID: "100", AdGroupID: "200"},
		{Action: Delete, KeywordID: "111", Keyword: "spam", MatchType: Broad, Status: Active, IsNegative: true, CampaignID: "100", AdGroupID: "200"},
	}

	if !slices.Equal(cmds, expected) {
		t.Error(cmds, expected)
	}
}

func TestKeywordUpdater_UpdateCommands_MultipleNewKeywordsFromCSV(t *testing.T) {
	newCSV := `Action,Keyword ID,Keyword,Match Type,Status,Bid,Campaign ID,Ad Group ID
,,buscapé,BROAD,ACTIVE,0.2,2142845006,2143949666
,,méliuz,BROAD,ACTIVE,0.1,2142845006,2143949666
,,cuponomia,BROAD,ACTIVE,0.2,2142845006,2143949666
`
	var to KeywordCSVDB
	if err := to.LoadFromCSV(strings.NewReader(newCSV)); err != nil {
		t.Fatal(err)
	}

	cmds := KeywordUpdater{From: KeywordCSVDB{}, To: to}.UpdateCommands()

	var creates []KeywordCommand
	for _, c := range cmds {
		if c.Action == Create {
			creates = append(creates, c)
		}
	}

	if len(creates) != 3 {
		t.Errorf("expected 3 CREATE commands, got %d: %v", len(creates), creates)
	}
}

func TestPrintNegativeCommandsToCSV(t *testing.T) {
	cmds := []KeywordCommand{
		{Action: Create, Keyword: "market", MatchType: Broad, IsNegative: true, CampaignID: "2142845006", AdGroupID: "2143951277"},
		{Action: Delete, KeywordID: "2183598762", Keyword: "99 motorista", MatchType: Exact, IsNegative: true, CampaignID: "2142845006", AdGroupID: "2143951277"},
	}

	var sb strings.Builder
	if err := PrintNegativeCommandsToCSV(&sb, cmds); err != nil {
		t.Fatal(err)
	}

	if got := sb.String(); got != updaterNegativeCommandsCSV {
		t.Error(got)
	}
}
