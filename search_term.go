package goappleads

import (
	"bytes"
	"encoding/csv"
	"io"
	"iter"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/geo"
)

type RatioRange struct{ From, To float64 }

func (s *RatioRange) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}

	if text[len(text)-1] != '%' {
		return nil
	}
	text = text[:len(text)-1]

	parts := bytes.Split(text, []byte("-"))
	if len(parts) != 2 {
		return nil
	}

	lo, err := strconv.Atoi(string(parts[0]))
	if err != nil {
		return err
	}
	hi, err := strconv.Atoi(string(parts[1]))
	if err != nil {
		return err
	}

	s.From = float64(lo) / 100
	s.To = float64(hi) / 100

	return nil
}

type SearchTermInfo struct {
	Day              string
	Country          geo.Country
	SearchTerm       string
	Spend            float64
	Taps             int
	Installs         int
	Impressions      int
	ImpressionShare  RatioRange // Impression share is the share of impressions your ad(s) received from the total impressions served on the same search terms or keywords, in the same App Store countries and regions.
	Rank             int        // Rank is how your app ranks in terms of impression share compared to other apps in the same App Store countries and regions. Rank is displayed as numbers from 1 to 5 or >5 (we track it as 6), with 1 being the highest rank.
	SearchPopularity int        // Search popularity is the popularity of a search term, based on App Store searches. Range from [1:5], with 5 being the most popular. Search popularity can help you identify which keywords may present the greatest opportunities to drive campaign performance.
}

func ParseSearchTermInfoFromCSV(r io.Reader) iter.Seq[SearchTermInfo] {
	return func(yield func(SearchTermInfo) bool) {
		csvr := csv.NewReader(r)
		csvr.FieldsPerRecord = -1
		csvr.LazyQuotes = true

		// scan until CSV header row (starts with "Day")
		var timeGranularity, timeZone, currency string
		var header []string
		for {
			rec, err := csvr.Read()
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
			if len(rec) > 0 && strings.TrimSpace(rec[0]) == "Day" {
				header = rec
				for i := range header {
					header[i] = strings.TrimSpace(header[i])
				}
				break
			}
		}

		if timeGranularity != "DAILY" || timeZone != "UTC" || currency != "USD" {
			slog.Error("unsupported time granularity, time zone, or currency", "timeGranularity", timeGranularity, "timeZone", timeZone, "currency", currency)
			return
		}

		colIndex := make(map[string]int, len(header))
		for i, h := range header {
			colIndex[h] = i
		}

		for {
			rec, err := csvr.Read()
			if err != nil {
				break
			}
			if len(rec) < len(header) {
				continue
			}

			rankStr := strings.TrimSpace(rec[colIndex["Rank"]])
			var rank int
			if rankStr == ">5" {
				rank = 6
			} else {
				rank, err = strconv.Atoi(rankStr)
				if err != nil {
					slog.Error("failed to parse Rank", "error", err)
					return
				}
			}

			var impShare RatioRange
			if err := impShare.UnmarshalText([]byte(rec[colIndex["Impression Share"]])); err != nil {
				slog.Error("failed to parse Impression Share", "error", err)
				return
			}

			spend, err := strconv.ParseFloat(rec[colIndex["Spend"]], 64)
			if err != nil {
				slog.Error("failed to parse Spend", "error", err)
				return
			}
			taps, err := strconv.Atoi(rec[colIndex["Taps"]])
			if err != nil {
				slog.Error("failed to parse Taps", "error", err)
				return
			}
			installs, err := strconv.Atoi(rec[colIndex["Installs (Total)"]])
			if err != nil {
				slog.Error("failed to parse Installs", "error", err)
				return
			}
			impressions, err := strconv.Atoi(rec[colIndex["Impressions"]])
			if err != nil {
				slog.Error("failed to parse Impressions", "error", err)
				return
			}
			searchPop, err := strconv.Atoi(rec[colIndex["Search Popularity"]])
			if err != nil {
				slog.Error("failed to parse Search Popularity", "error", err)
				return
			}

			if !yield(SearchTermInfo{
				Day:              rec[colIndex["Day"]],
				Country:          countryByName[rec[colIndex["Country or Region"]]],
				SearchTerm:       rec[colIndex["Search Term"]],
				Spend:            spend,
				Taps:             taps,
				Installs:         installs,
				Impressions:      impressions,
				ImpressionShare:  impShare,
				Rank:             rank,
				SearchPopularity: searchPop,
			}) {
				return
			}
		}
	}
}

type SearchTermRow struct {
	Day         time.Time
	CampaignID  CampaignID
	AdGroupID   AdGroupID
	KeywordID   KeywordID
	SearchTerm  string
	MatchSource string
	Impressions int
	Spend       float64
	Taps        int
	Installs    int
}

func ParseSearchTermsStatsCSV(r io.Reader) iter.Seq[SearchTermRow] {
	return func(yield func(SearchTermRow) bool) {
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
			imp, err := strconv.Atoi(rec[colIndex["Impressions"]])
			if err != nil {
				slog.Error("failed to parse Impressions", "error", err)
				return
			}
			sp, err := strconv.ParseFloat(rec[colIndex["Spend"]], 64)
			if err != nil {
				slog.Error("failed to parse Spend", "error", err)
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

			ts, err := time.Parse(time.DateOnly, rec[colIndex["Day"]])
			if err != nil {
				slog.Error("failed to parse Day", "error", err)
				return
			}

			row := SearchTermRow{
				Day:         ts,
				CampaignID:  CampaignID(rec[colIndex["Campaign ID"]]),
				AdGroupID:   AdGroupID(rec[colIndex["Ad group ID"]]),
				KeywordID:   KeywordID(rec[colIndex["Keyword ID"]]),
				SearchTerm:  rec[colIndex["Search Term"]],
				MatchSource: rec[colIndex["Search Term Match Source"]],
				Impressions: imp,
				Spend:       sp,
				Taps:        taps,
				Installs:    inst,
			}

			if !yield(row) {
				return
			}
		}
	}
}
