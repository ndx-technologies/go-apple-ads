package appleadsanalysiscampaigns

import (
	"flag"
	"io"
	"log"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/timex"
)

func printBaselines(w io.StringWriter, showID, showPaused bool, baselines map[goappleads.CampaignID]goappleads.BaselineMetrics, overall goappleads.BaselineMetrics, config goappleads.Config) {
	fmtx.HeaderTo(w, "CAMPAIGN STATS")

	campaigns := slices.Collect(maps.Keys(baselines))

	sort.Slice(campaigns, func(i, j int) bool {
		ci := baselines[campaigns[i]].CPI
		cj := baselines[campaigns[j]].CPI
		if ci == 0 {
			ci = 9999
		}
		if cj == 0 {
			cj = 9999
		}
		return ci < cj
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 32},
			{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CVR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Installs", Width: 20},
			{Header: "Spend(USD)", Width: 12, Alignment: fmtx.AlignRight},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{{Header: "ID", Width: 10}}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteSubHeader(
		"",
		strconv.FormatFloat(overall.CPI, 'f', 2, 64),
		strconv.FormatFloat(overall.CVR*100, 'f', 1, 64)+"%",
		strconv.FormatFloat(overall.CTR*100, 'f', 2, 64)+"%",
		strconv.Itoa(overall.Inst),
		"",
		strconv.FormatFloat(overall.Spend, 'f', 2, 64),
	)
	tw.WriteHeaderLine()

	for _, c := range campaigns {
		campaign := config.GetCampaign(c)

		cpiStr := fmtx.DimS(strconv.FormatFloat(baselines[c].CPI, 'f', 2, 64))
		if baselines[c].Inst > 0 {
			cpiStr = fmtx.ColorizeDist(strconv.FormatFloat(baselines[c].CPI, 'f', 2, 64), baselines[c].CPI, []float64{overall.CPI}, []string{fmtx.Green, fmtx.Red})
		}

		cvrStr := fmtx.DimS(strconv.FormatFloat(baselines[c].CVR*100, 'f', 1, 64) + "%")
		if baselines[c].Imp > 0 {
			cvrStr = fmtx.ColorizeDist(strconv.FormatFloat(baselines[c].CVR*100, 'f', 1, 64)+"%", baselines[c].CVR, []float64{overall.CVR * 0.7, overall.CVR}, []string{fmtx.Red, fmtx.Yellow, fmtx.Green})
		}

		ctrStr := fmtx.DimS(strconv.FormatFloat(baselines[c].CTR*100, 'f', 2, 64) + "%")
		if baselines[c].Taps > 0 {
			ctrStr = fmtx.ColorizeDist(strconv.FormatFloat(baselines[c].CTR*100, 'f', 2, 64)+"%", baselines[c].CTR, []float64{overall.CTR * 0.7, overall.CTR}, []string{fmtx.Red, fmtx.Yellow, fmtx.Green})
		}

		name := campaign.Name
		if showPaused && campaign.Status == goappleads.Paused {
			name += " " + fmtx.DimS("⏸")
		}

		row := []string{
			name,
			cpiStr,
			cvrStr,
			ctrStr,
			strconv.Itoa(baselines[c].Inst),
			fmtx.VolumeBar(baselines[c].Inst, overall.Inst, 20.0),
			strconv.FormatFloat(baselines[c].Spend, 'f', 2, 64),
		}

		if showID {
			row = append([]string{fmtx.DimS(c.String())}, row...)
		}

		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

const DocShort string = "campaigns stats (CPI, CVR, CTR, Installs, Spend, ...)"
const doc string = "Apple Ads Campaigns Analysis — stats\n\n"

func Run(args []string) {
	flag := flag.NewFlagSet("analyse campaigns", flag.ExitOnError)
	var (
		applePath          string
		keywordStatsCSV    string
		campaignStatsCSV   string
		showID, showPaused bool
		campaignIDsStr     string
		from, until        time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple_path", "apple-ads", "path to apple ads dir")
	flag.StringVar(&keywordStatsCSV, "keyword_stats_csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.StringVar(&campaignStatsCSV, "campaign_stats_csv", "data/apple_ads_campaign_stats_by_day.csv", "path to campaign stats by day CSV")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&showPaused, "paused", false, "include paused campaigns")
	flag.BoolVar(&fmtx.EnableColor, "color", true, "colorize output")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2025-12-31)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, _, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load config and keywords:", err)
	}

	var keepCampaignIDs map[goappleads.CampaignID]bool

	if len(campaignIDsStr) > 0 {
		keepCampaignIDs = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaignIDs[goappleads.CampaignID(id)] = true
		}
	}

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		if !showPaused {
			campaign := config.GetCampaign(e.CampaignID)
			if campaign.Status == goappleads.Paused {
				continue
			}
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
		if !showPaused {
			campaign := config.GetCampaign(e.CampaignID)
			if campaign.Status == goappleads.Paused {
				continue
			}
		}
		campaigns = append(campaigns, e)
	}

	baselines, overall := goappleads.ComputeBaselines(keywordsStats)

	w := os.Stdout

	printBaselines(w, showID, showPaused, baselines, overall, *config)
}
