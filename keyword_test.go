package goappleads

import (
	_ "embed"
	"maps"
	"strings"
	"testing"
)

//go:embed testdata/keywords.csv
var positiveCSV string

//go:embed testdata/keywords_negative.csv
var negativeCSV string

func TestKeywordCSVDB_LoadFromCSV(t *testing.T) {
	tests := []struct {
		name     string
		csvData  string
		expected map[KeywordID]KeywordInfo
	}{
		{
			csvData: positiveCSV,
			expected: map[KeywordID]KeywordInfo{
				KeywordID("2204670087"): {ID: "2204670087", Keyword: "ajfans", MatchType: Exact, Status: Active, Bid: 0.3, CampaignID: "2142845006", AdGroupID: "2143951277", IsNegative: false},
				KeywordID("2183598762"): {ID: "2183598762", Keyword: "99 motorista", MatchType: Exact, Status: Paused, Bid: 0.8, CampaignID: "2142845006", AdGroupID: "2143951277", IsNegative: false},
				KeywordID("2175126639"): {ID: "2175126639", Keyword: "gas station", MatchType: Broad, Status: Active, Bid: 0.2, CampaignID: "2142845006", AdGroupID: "2143951277", IsNegative: false},
				KeywordID("2175126638"): {ID: "2175126638", Keyword: "gas", MatchType: Broad, Status: Active, Bid: 0.2, CampaignID: "2142845006", AdGroupID: "2143951277", IsNegative: false},
				KeywordID("2175126637"): {ID: "2175126637", Keyword: "station", MatchType: Broad, Status: Active, Bid: 0.2, CampaignID: "2142845006", AdGroupID: "2143951277", IsNegative: false},
			},
		},
		{
			name:    "negative",
			csvData: negativeCSV,
			expected: map[KeywordID]KeywordInfo{
				KeywordID("2249583763"): {ID: "2249583763", Keyword: "door dash", MatchType: Broad, CampaignID: "2142845006", IsNegative: true},
				KeywordID("2249583762"): {ID: "2249583762", Keyword: "ionity", MatchType: Broad, CampaignID: "2142845006", IsNegative: true},
				KeywordID("2249583761"): {ID: "2249583761", Keyword: "red note", MatchType: Broad, CampaignID: "2142845006", IsNegative: true},
				KeywordID("2249583760"): {ID: "2249583760", Keyword: "evie", MatchType: Broad, CampaignID: "2142845006", IsNegative: true},
				KeywordID("2249583759"): {ID: "2249583759", Keyword: "evgo", MatchType: Broad, CampaignID: "2142845006", IsNegative: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var db KeywordCSVDB

			if err := db.LoadFromCSV(strings.NewReader(tt.csvData)); err != nil {
				t.Error(err)
			}

			if !maps.Equal(db.Keywords, tt.expected) {
				t.Error(db.Keywords, tt.expected)
			}
		})
	}
}

func TestKeywordCSVDB_LoadFromCSV_MultipleNewKeywords(t *testing.T) {
	csvData := `Action,Keyword ID,Keyword,Match Type,Status,Bid,Campaign ID,Ad Group ID
,,buscapé,BROAD,ACTIVE,0.2,2142845006,2143949666
,,méliuz,BROAD,ACTIVE,0.1,2142845006,2143949666
,,cuponomia,BROAD,ACTIVE,0.2,2142845006,2143949666
`
	var db KeywordCSVDB
	if err := db.LoadFromCSV(strings.NewReader(csvData)); err != nil {
		t.Fatal(err)
	}

	if len(db.Keywords) != 3 {
		t.Errorf("expected 3 keywords, got %d: %v", len(db.Keywords), db.Keywords)
	}
}

func TestKeywordCSVDB_LoadFromCSV_MultipleFilesWithNewKeywords(t *testing.T) {
	file1 := `Action,Keyword ID,Keyword,Match Type,Status,Bid,Campaign ID,Ad Group ID
,,grabpay,BROAD,ACTIVE,0.2,2142805170,2143751228
,,true money,BROAD,ACTIVE,0.2,2142805170,2143751228
`
	file2 := `Action,Keyword ID,Keyword,Match Type,Status,Bid,Campaign ID,Ad Group ID
,,buscapé,BROAD,ACTIVE,0.2,2142845006,2143949666
,,méliuz,BROAD,ACTIVE,0.1,2142845006,2143949666
`
	var db KeywordCSVDB
	if err := db.LoadFromCSV(strings.NewReader(file1)); err != nil {
		t.Fatal(err)
	}
	if err := db.LoadFromCSV(strings.NewReader(file2)); err != nil {
		t.Fatal(err)
	}

	if len(db.Keywords) != 4 {
		t.Errorf("expected 4 keywords, got %d: %v", len(db.Keywords), db.Keywords)
	}
}
