package appleadstimeline

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "daily performance"
const doc string = "Apple Ads Timeline of daily performance\n\n"

func Run(args []string) {
	var (
		applePath                                    string
		keywordStatsCSV                              string
		campaignStatsCSV                             string
		campaignIDsStr, adGroupIDsStr, keywordsIDStr string
		from, until                                  time.Time
	)

	flag := flag.NewFlagSet("build", flag.ExitOnError)

	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to apple ads db")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.StringVar(&campaignStatsCSV, "campaign-stats-csv", "data/apple_ads_campaign_stats_by_day.csv", "path to campaign stats by day CSV")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of adgroup IDs to keep")
	flag.StringVar(&keywordsIDStr, "keyword-ids", "", "comma-separated list of keyword IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	var (
		keepCampaignIDs map[goappleads.CampaignID]bool
		keepAdGroupIDs  map[goappleads.AdGroupID]bool
		keepKeywordIDs  map[goappleads.KeywordID]bool
	)

	if len(campaignIDsStr) > 0 {
		keepCampaignIDs = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaignIDs[goappleads.CampaignID(id)] = true
		}
	}
	if len(adGroupIDsStr) > 0 {
		keepAdGroupIDs = make(map[goappleads.AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroupIDs[goappleads.AdGroupID(id)] = true
		}
	}

	if len(keywordsIDStr) > 0 {
		keepKeywordIDs = make(map[goappleads.KeywordID]bool)
		for id := range strings.SplitSeq(keywordsIDStr, ",") {
			keepKeywordIDs[goappleads.KeywordID(id)] = true
		}
	}

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepKeywordIDs != nil && !keepKeywordIDs[e.KeywordID] {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		if keepAdGroupIDs != nil && !keepAdGroupIDs[e.AdGroupID] {
			continue
		}
		keywordsStats = append(keywordsStats, e)
	}

	var campaigns []goappleads.CampaignRow
	for e := range iterx.FromFile(campaignStatsCSV, goappleads.ParseCampaignsStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		campaigns = append(campaigns, e)
	}

	w := os.Stdout

	_, overall := goappleads.ComputeBaselines(keywordsStats)

	byDay := make(map[time.Time]goappleads.Agg)
	for _, r := range campaigns {
		a := byDay[r.Day]
		if a.Days == nil {
			a.Days = make(map[time.Time]struct{})
		}
		a.Spend += r.Spend
		a.Imp += r.Impressions
		a.Taps += r.Taps
		a.Inst += r.Installs
		a.Days[r.Day] = struct{}{}
		byDay[r.Day] = a
	}

	days := make([]time.Time, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}

	sort.Slice(days, func(i, j int) bool { return days[i].After(days[j]) })

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Date(UTC)", Width: 10},
			{Header: "Spend(USD)", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Installs", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Imp", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CVR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
		},
	}
	tw.WriteHeader()
	tw.WriteHeaderLine()

	maxInst := 0
	for _, d := range byDay {
		if d.Inst > maxInst {
			maxInst = d.Inst
		}
	}

	for _, day := range days {
		d := byDay[day]

		var ctr, cvr, cpi float64
		if d.Imp > 0 {
			ctr = float64(d.Taps) / float64(d.Imp)
		}
		if d.Taps > 0 {
			cvr = float64(d.Inst) / float64(d.Taps)
		}
		if d.Inst > 0 {
			cpi = d.Spend / float64(d.Inst)
		}

		instS := strconv.Itoa(d.Inst)
		instColor := instS
		if d.Inst >= int(float64(maxInst)*0.8) {
			instColor = fmtx.GreenS(instS)
		} else if d.Inst < int(float64(maxInst)*0.5) {
			instColor = fmtx.RedS(instS)
		}

		cpiS := "∞"
		cpiColor := fmtx.DimS(cpiS)
		if cpi > 0 {
			cpiS = strconv.FormatFloat(cpi, 'f', 2, 64)
			if overall.CPI > 0 {
				if cpi < overall.CPI {
					cpiColor = fmtx.GreenS(cpiS)
				} else if cpi > overall.CPI*1.5 {
					cpiColor = fmtx.RedS(cpiS)
				} else {
					cpiColor = fmtx.YellowS(cpiS)
				}
			}
		}

		tw.WriteRow(
			day.Format(time.DateOnly),
			strconv.FormatFloat(d.Spend, 'f', 2, 64),
			instColor,
			fmtx.VolumeBar(d.Inst, int(float64(maxInst)*1.1), 10),
			strconv.Itoa(d.Taps),
			strconv.Itoa(d.Imp),
			cpiColor,
			strconv.FormatFloat(cvr*100, 'f', 2, 64)+"%",
			strconv.FormatFloat(ctr*100, 'f', 2, 64)+"%",
		)
	}

	fmt.Println()
}
