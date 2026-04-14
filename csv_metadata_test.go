package goappleads

import (
	_ "embed"
	"encoding/csv"
	"slices"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/search_term_info.csv
var csvMetadataSearchTermInfoCSV string

func TestReadCSVMetadata(t *testing.T) {
	r := csv.NewReader(strings.NewReader(csvMetadataSearchTermInfoCSV))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	m, header, err := ReadCSVMetadata(r)
	if err != nil {
		t.Fatal(err)
	}

	if m.ReportName != "apple_ads_search_term_impression_share_by_day_merged" {
		t.Fatalf("ReportName = %q", m.ReportName)
	}
	if m.TimeZone != "UTC" {
		t.Fatalf("TimeZone = %q", m.TimeZone)
	}
	if m.Currency != "USD" {
		t.Fatalf("Currency = %q", m.Currency)
	}
	if m.From != (time.Date(2026, time.January, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("From = %s", m.From.Format(time.DateOnly))
	}
	if m.Until != (time.Date(2026, time.February, 23, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Until = %s", m.Until.Format(time.DateOnly))
	}

	expectedHeader := []string{
		"Day",
		"App Name",
		"App ID",
		"Country or Region",
		"Search Term",
		"Search Popularity (1-5)",
		"Impression Share",
		"Rank",
		"Spend",
		"Impressions",
		"Taps",
		"Campaign Group ID",
		"Installs (Total)",
	}
	if !slices.Equal(header, expectedHeader) {
		t.Fatalf("header = %v", header)
	}
}
