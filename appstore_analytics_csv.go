package goappleads

import (
	"encoding/csv"
	"io"
	"iter"
	"strconv"
	"strings"
	"time"
)

// AppStoreAnalyticsCampaignRow represents a single day's campaign-level first-time downloads
// from Apple's App Store Connect Analytics dashboard CSV export.
type AppStoreAnalyticsCampaignRow struct {
	Day      time.Time
	Campaign string // CT tag (e.g. "g-d-pf-1", "fb-ndxone-1")
	Installs int
}

// ParseAppStoreAnalyticsCampaignCSV parses Apple's App Store Connect Analytics dashboard CSV.
// The CSV format has metadata rows (Name, Start Date, End Date, blank) followed by a header row
// starting with "Date" and then data rows.
func ParseAppStoreAnalyticsCampaignCSV(r io.Reader) iter.Seq[AppStoreAnalyticsCampaignRow] {
	return func(yield func(AppStoreAnalyticsCampaignRow) bool) {
		csvr := csv.NewReader(r)
		csvr.FieldsPerRecord = -1

		// Skip metadata rows until we find the header starting with "Date"
		var header []string
		for {
			row, err := csvr.Read()
			if err != nil {
				return
			}
			if len(row) == 0 {
				continue
			}
			if row[0] == "Date" {
				header = row
				break
			}
		}

		// Extract CT tags from column headers: "g-d-pf-1 First-Time Downloads" → "g-d-pf-1"
		ctTags := make([]string, len(header))
		for i, col := range header {
			if i == 0 || col == "Date" {
				continue
			}
			parts := strings.SplitN(col, " ", 2)
			ctTags[i] = parts[0]
		}

		for {
			rec, err := csvr.Read()
			if err != nil {
				break
			}
			if len(rec) < 2 {
				continue
			}

			day, err := time.Parse("1/2/06", rec[0])
			if err != nil {
				continue
			}

			for i := 1; i < len(rec) && i < len(ctTags); i++ {
				ct := ctTags[i]
				if ct == "" {
					continue
				}
				val, err := strconv.ParseFloat(strings.TrimSpace(rec[i]), 64)
				if err != nil || val == 0 {
					continue
				}

				if !yield(AppStoreAnalyticsCampaignRow{
					Day:      day,
					Campaign: ct,
					Installs: int(val),
				}) {
					return
				}
			}
		}
	}
}
