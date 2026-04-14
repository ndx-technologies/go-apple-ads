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
	Day                         time.Time
	CampaignID                  CampaignID
	AdGroupID                   AdGroupID
	KeywordID                   KeywordID
	Keyword                     string
	MatchType                   MatchType
	Budget                      string
	MaxCPTBid, Spend            float64
	Impressions, Taps, Installs int
}

func ParseKeywordStatsCSV(r io.Reader) iter.Seq[KeywordRow] {
	return func(yield func(KeywordRow) bool) {
		csvr := csv.NewReader(r)
		csvr.FieldsPerRecord = -1

		m, header, err := ReadCSVMetadata(csvr)
		if err != nil {
			slog.Error("failed to read CSV header", "error", err)
			return
		}
		colIndex := make(map[string]int, len(header))
		for i, h := range header {
			colIndex[h] = i
		}
		if m.TimeZone != "UTC" && m.TimeZone != "ORTZ" || m.Currency != "USD" {
			slog.Error("unsupported time zone or currency", "timeZone", m.TimeZone, "currency", m.Currency)
			return
		}

		for {
			rec, err := csvr.Read()
			if err != nil {
				break
			}
			sp, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(rec[colIndex["Spend"]]), "$"), ",", ""), 64)
			if err != nil {
				slog.Error("failed to parse Spend", "error", err)
				return
			}
			imp, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(rec[colIndex["Impressions"]]), ",", ""))
			if err != nil {
				slog.Error("failed to parse Impressions", "error", err)
				return
			}
			taps, err := strconv.Atoi(rec[colIndex["Taps"]])
			if err != nil {
				slog.Error("failed to parse Taps", "error", err)
				return
			}
			inst, err := strconv.Atoi(rec[colIndex["Installs (Total)"]])
			if err != nil {
				slog.Error("failed to parse Installs", "error", err)
				return
			}

			ts, err := time.Parse(time.DateOnly, rec[colIndex["Date"]])
			if err != nil {
				ts, err = time.Parse("01/02/2006", rec[colIndex["Date"]])
			}
			if err != nil {
				slog.Error("failed to parse Date", "error", err)
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
				AdGroupID:   AdGroupID(rec[colIndex["Ad Group ID"]]),
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
