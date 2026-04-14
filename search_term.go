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

		m, header, err := ReadCSVMetadata(csvr)
		if err != nil {
			slog.Error("failed to read CSV header", "error", err)
			return
		}
		colIndex := make(map[string]int, len(header))
		for i, h := range header {
			colIndex[h] = i
		}
		if m.TimeZone != "UTC" || m.Currency != "USD" {
			slog.Error("unsupported time zone or currency", "timeZone", m.TimeZone, "currency", m.Currency)
			return
		}

		for {
			rec, err := csvr.Read()
			if err != nil {
				break
			}
			if len(rec) < len(colIndex) {
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

			spend, err := strconv.ParseFloat(strings.TrimPrefix(rec[colIndex["Spend"]], "$"), 64)
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
			spopCol := colIndex["Search Popularity (1-5)"]
			if spopCol == 0 {
				spopCol = colIndex["Search Popularity"]
			}
			searchPop, err := strconv.Atoi(rec[spopCol])
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

		m, header, err := ReadCSVMetadata(r)
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
			rec, err := r.Read()
			if err != nil {
				break
			}
			imp, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(rec[colIndex["Impressions"]]), ",", ""))
			if err != nil {
				slog.Error("failed to parse Impressions", "error", err)
				return
			}
			sp, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(rec[colIndex["Spend"]]), "$"), ",", ""), 64)
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

			ts, err := time.Parse(time.DateOnly, rec[colIndex["Date"]])
			if err != nil {
				ts, err = time.Parse("01/02/2006", rec[colIndex["Date"]])
			}
			if err != nil {
				slog.Error("failed to parse Date", "error", err)
				return
			}

			row := SearchTermRow{
				Day:         ts,
				CampaignID:  CampaignID(rec[colIndex["Campaign ID"]]),
				AdGroupID:   AdGroupID(rec[colIndex["Ad Group ID"]]),
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
