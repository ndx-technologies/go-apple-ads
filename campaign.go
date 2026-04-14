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

type CampaignID string

func (s CampaignID) IsZero() bool { return s == "" }

func (s CampaignID) String() string { return string(s) }

type AdGroupID string

func (s AdGroupID) IsZero() bool { return s == "" }

func (s AdGroupID) String() string { return string(s) }

type CampaignRow struct {
	Day         time.Time
	CampaignID  CampaignID
	AdGroupID   AdGroupID
	Budget      string
	Spend       float64
	Impressions int
	Taps        int
	Installs    int
}

func ParseCampaignsStatsCSV(r io.Reader) iter.Seq[CampaignRow] {
	return func(yield func(CampaignRow) bool) {
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
			impF, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(rec[colIndex["Impressions"]]), ",", ""), 64)
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

			row := CampaignRow{
				Day:         ts,
				CampaignID:  CampaignID(rec[colIndex["Campaign ID"]]),
				AdGroupID:   AdGroupID(rec[colIndex["Ad Group ID"]]),
				Budget:      rec[colIndex["Daily Budget"]],
				Spend:       sp,
				Impressions: int(impF),
				Taps:        taps,
				Installs:    inst,
			}

			if !yield(row) {
				return
			}
		}
	}
}
