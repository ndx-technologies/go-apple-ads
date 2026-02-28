package goappleads

import (
	"encoding/csv"
	"io"
	"iter"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type KeywordRow struct {
	Day         time.Time
	CampaignID  CampaignID
	AdGroupID   AdGroupID
	Budget      string
	KeywordID   KeywordID
	Keyword     string
	MaxCPTBid   float64
	MatchType   MatchType
	Spend       float64
	Impressions int
	Taps        int
	Installs    int
}

func ParseKeywordStatsCSV(r io.Reader) iter.Seq[KeywordRow] {
	return func(yield func(KeywordRow) bool) {
		r := csv.NewReader(r)
		r.FieldsPerRecord = -1

		var timeGranularity, timeZone, currency string
		var header []string
		for {
			rec, err := r.Read()
			if err != nil {
				slog.Error("failed to read CSV header", "error", err)
				return
			}

			if len(rec) >= 2 {
				switch rec[0] {
				case "Time Granularity":
					timeGranularity = strings.TrimSpace(rec[1])
				case "Time Zone":
					timeZone = strings.TrimSpace(rec[1])
				case "Currency":
					currency = strings.TrimSpace(rec[1])
				}
			}

			if len(rec) > 0 && rec[0] == "Day" {
				header = rec
				break
			}
		}

		if timeGranularity != "DAILY" || timeZone != "UTC" || currency != "USD" {
			slog.Error("unsupported time granularity, time zone, or currency", "timeGranularity", timeGranularity, "timeZone", timeZone, "currency", currency)
			return
		}

		if len(header) == 0 {
			slog.Error("no header found")
			return
		}
		colIndex := make(map[string]int)
		for i, h := range header {
			colIndex[h] = i
		}

		for {
			rec, err := r.Read()
			if err != nil {
				break
			}
			sp, err := strconv.ParseFloat(rec[colIndex["Spend"]], 64)
			if err != nil {
				slog.Error("failed to parse Spend", "error", err)
				return
			}
			imp, err := strconv.Atoi(rec[colIndex["Impressions"]])
			if err != nil {
				slog.Error("failed to parse Impressions", "error", err)
				return
			}
			taps, err := strconv.Atoi(rec[colIndex["Taps"]])
			if err != nil {
				slog.Error("failed to parse Taps", "error", err)
				return
			}
			inst, err := strconv.Atoi(rec[colIndex["Installs (Tap-Through)"]])
			if err != nil {
				slog.Error("failed to parse Installs", "error", err)
				return
			}

			ts, err := time.Parse(time.DateOnly, rec[colIndex["Day"]])
			if err != nil {
				slog.Error("failed to parse Day", "error", err)
				return
			}

			var maxCPTBid float64
			if s := rec[colIndex["Keyword Max CPT Bid"]]; s != "" && s != "--" && s != "null" {
				maxCPTBid, err = strconv.ParseFloat(s, 64)
				if err != nil {
					slog.Error("failed to parse Keyword Max CPT Bid", "error", err)
					return
				}
			}

			row := KeywordRow{
				Day:         ts,
				CampaignID:  CampaignID(rec[colIndex["Campaign ID"]]),
				AdGroupID:   AdGroupID(rec[colIndex["Ad group ID"]]),
				Budget:      rec[colIndex["Daily Budget"]],
				Keyword:     rec[colIndex["Keyword"]],
				KeywordID:   KeywordID(rec[colIndex["Keyword ID"]]),
				MaxCPTBid:   maxCPTBid,
				MatchType:   MatchType(rec[colIndex["Keyword Match Type"]]),
				Spend:       sp,
				Impressions: imp,
				Taps:        taps,
				Installs:    inst,
			}

			if !yield(row) {
				return
			}
		}
	}
}
