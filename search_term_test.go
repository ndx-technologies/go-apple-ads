package goappleads

import (
	_ "embed"
	"slices"
	"strings"
	"testing"

	"github.com/ndx-technologies/geo"
)

//go:embed testdata/search_term_info.csv
var searchTermInfoCSV string

func TestParseSearchTermInfoFromCSV(t *testing.T) {
	expected := []SearchTermInfo{
		{
			Day:              "2026-02-03",
			Country:          geo.Brazil,
			SearchTerm:       "farmacia sao joao",
			Spend:            0.19,
			Taps:             1,
			Installs:         1,
			Impressions:      228,
			ImpressionShare:  RatioRange{From: 0.41, To: 0.50},
			Rank:             1,
			SearchPopularity: 3,
		},
		{
			Day:              "2026-02-04",
			Country:          geo.Brazil,
			SearchTerm:       "farmacia sao joao",
			Spend:            0.19,
			Taps:             1,
			Installs:         1,
			Impressions:      228,
			ImpressionShare:  RatioRange{From: 0.41, To: 0.50},
			Rank:             6,
			SearchPopularity: 3,
		},
	}

	rows := slices.Collect(ParseSearchTermInfoFromCSV(strings.NewReader(searchTermInfoCSV)))
	if !slices.Equal(rows, expected) {
		t.Error(rows)
	}
}
