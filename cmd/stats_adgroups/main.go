package statsadgroups

import (
	"flag"
	"io"
	"log"
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

func ratioVsBaseline(value, baseline float64, higherIsBetter bool, ok bool) string {
	if baseline <= 0 || !ok {
		return fmtx.DimS("n/a")
	}

	ratio := value / baseline

	if higherIsBetter {
		if ratio >= 1.0 {
			return fmtx.GreenS("▲" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
		} else if ratio >= 0.7 {
			return fmtx.YellowS("▼" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
		}
		return fmtx.RedS("▼" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
	} else {
		if ratio <= 1.0 {
			return fmtx.GreenS("▼" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
		} else if ratio <= 1.5 {
			return fmtx.YellowS("▲" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
		}
		return fmtx.RedS("▲" + strconv.FormatFloat(ratio, 'f', 1, 64) + "x")
	}
}

func printAdGroupPerformance(w io.StringWriter, showID, showPaused bool, keywords []goappleads.KeywordRow, baselines map[goappleads.CampaignID]goappleads.BaselineMetrics, overall goappleads.BaselineMetrics, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) {
	fmtx.HeaderTo(w, "ADGROUP PERFORMANCE")

	type Agg struct {
		goappleads.Agg
		AdGroups map[goappleads.AdGroupID]struct{}
	}

	byCampAG := make(map[goappleads.CampaignID]map[goappleads.AdGroupID]Agg)

	for _, r := range keywords {
		agKey := r.AdGroupID

		if byCampAG[r.CampaignID] == nil {
			byCampAG[r.CampaignID] = make(map[goappleads.AdGroupID]Agg)
		}

		a := byCampAG[r.CampaignID][agKey]

		if a.Days == nil {
			a.Days = make(map[time.Time]struct{})
			a.AdGroups = make(map[goappleads.AdGroupID]struct{})
		}

		a.Spend += r.Spend
		a.Imp += r.Impressions
		a.Taps += r.Taps
		a.Inst += r.Installs
		a.Days[r.Day] = struct{}{}
		a.AdGroups[r.AdGroupID] = struct{}{}
		byCampAG[r.CampaignID][agKey] = a
	}

	type AdGroupStats struct {
		adGroup goappleads.AdGroupID
		stats   Agg
	}

	campSpend := make(map[goappleads.CampaignID]float64)
	for camp, groups := range byCampAG {
		for _, a := range groups {
			campSpend[camp] += a.Spend
		}
	}

	camps := make([]goappleads.CampaignID, 0, len(byCampAG))
	for camp := range byCampAG {
		camps = append(camps, camp)
	}
	sort.Slice(camps, func(i, j int) bool { return campSpend[camps[i]] > campSpend[camps[j]] })

	for _, camp := range camps {
		campBaseline, exists := baselines[camp]
		if !exists {
			campBaseline = overall
		}
		groups := byCampAG[camp]

		var groupList []AdGroupStats
		for ag, d := range groups {
			if d.Taps == 0 {
				continue
			}
			groupList = append(groupList, AdGroupStats{adGroup: ag, stats: d})
		}

		sort.Slice(groupList, func(i, j int) bool {
			ci := groupList[i].stats.Spend / float64(groupList[i].stats.Inst)
			if groupList[i].stats.Inst == 0 {
				ci = 9999
			}
			cj := groupList[j].stats.Spend / float64(groupList[j].stats.Inst)
			if groupList[j].stats.Inst == 0 {
				cj = 9999
			}
			return ci < cj
		})

		totalInst := 0
		for _, g := range groupList {
			totalInst += g.stats.Inst
		}
		if totalInst == 0 {
			continue
		}

		campaign := config.GetCampaign(camp)
		w.WriteString("\n")

		w.WriteString(" " + campaign.Name)
		if showID {
			w.WriteString(" " + fmtx.DimS("("+campaign.ID.String()+")"))
		}
		if showPaused {
			if campaign.Status == goappleads.Paused {
				w.WriteString(" " + fmtx.DimS("⏸"))
			}
		}
		w.WriteString("\n")

		maxInst := 0
		for _, g := range groupList {
			if g.stats.Inst > maxInst {
				maxInst = g.stats.Inst
			}
		}

		tw := fmtx.TableWriter{
			Indent: "    ",
			Out:    w,
			Cols: []fmtx.TablCol{
				{Header: "Ad Group", Width: 20},
				{Header: "Keywords", Width: 9, Alignment: fmtx.AlignRight},
				{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
				{Header: "Installs", Width: 10},
				{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
				{Header: "CPI/Base", Width: 9, Alignment: fmtx.AlignRight},
				{Header: "CVR", Width: 7, Alignment: fmtx.AlignRight},
				{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
				{Header: "Taps", Width: 7, Alignment: fmtx.AlignRight},
				{Header: "Spend(USD)", Width: 10, Alignment: fmtx.AlignRight},
			},
		}

		if showID {
			tw.Cols = append([]fmtx.TablCol{{Header: "ID", Width: 10}}, tw.Cols...)
		}

		tw.WriteHeader()

		subheader := []string{
			"",
			strconv.Itoa(keywordsDB.NumKeywordsInCampaign(camp)),
			strconv.Itoa(campBaseline.Inst),
			"",
			strconv.FormatFloat(campBaseline.CPI, 'f', 2, 64),
			"",
			strconv.FormatFloat(campBaseline.CVR*100, 'f', 1, 64) + "%",
			strconv.FormatFloat(campBaseline.CVR*100, 'f', 1, 64) + "%",
			strconv.FormatFloat(campBaseline.CTR*100, 'f', 2, 64) + "%",
			strconv.Itoa(campBaseline.Taps),
			strconv.FormatFloat(campBaseline.Spend, 'f', 2, 64),
		}

		if showID {
			subheader = append([]string{""}, subheader...)
		}

		tw.WriteSubHeader(subheader...)
		tw.WriteHeaderLine()

		for _, g := range groupList {
			d := g.stats

			var cvr, ctr, cpi float64

			if d.Taps > 0 {
				cvr = float64(d.Inst) / float64(d.Taps)
			}
			if d.Imp > 0 {
				ctr = float64(d.Taps) / float64(d.Imp)
			}
			if d.Inst > 0 {
				cpi = d.Spend / float64(d.Inst)
			}

			cpiS := "∞"
			if cpi > 0 && campBaseline.CPI > 0 {
				cpiS = strconv.FormatFloat(cpi, 'f', 2, 64)
			}

			adgroup := config.GetAdGroup(g.adGroup)

			name := adgroup.Name
			if showPaused && adgroup.Status == goappleads.Paused {
				name += " " + fmtx.DimS("⏸")
			}

			row := []string{
				name,
				strconv.Itoa(keywordsDB.NumKeywordsInAdGroup(g.adGroup)),
				strconv.Itoa(d.Inst),
				fmtx.VolumeBar(d.Inst, maxInst, 10),
				cpiS,
				ratioVsBaseline(cpi, campBaseline.CPI, false, d.Inst > 0),
				strconv.FormatFloat(cvr*100, 'f', 1, 64) + "%",
				strconv.FormatFloat(ctr*100, 'f', 2, 64) + "%",
				strconv.Itoa(d.Taps),
				strconv.FormatFloat(d.Spend, 'f', 2, 64),
			}

			if showID {
				row = append([]string{fmtx.DimS(g.adGroup.String())}, row...)
			}

			tw.WriteRow(row...)
		}
	}
	w.WriteString("\n")
}

const DocShort string = "adgroups stats (CPI, CVR, CTR, Installs, Spend, ...)"
const doc string = "Apple Ads AdGropup Analysis — stats\n\n"

func Run(args []string) {
	flag := flag.NewFlagSet("analyse adgroups", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		campaignIDsStr, adGroupIDsStr string
		showID, showPaused            bool
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to apple ads dir")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of ad group IDs to keep")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&showPaused, "paused", false, "include paused adgroups, campaigns")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load config and keywords:", err)
	}

	var (
		keepCampaignIDs map[goappleads.CampaignID]bool
		keepAdGroupIDs  map[goappleads.AdGroupID]bool
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

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaignIDs != nil && !keepCampaignIDs[e.CampaignID] {
			continue
		}
		if keepAdGroupIDs != nil && !keepAdGroupIDs[e.AdGroupID] {
			continue
		}
		if config.IsAdGroupPaused(e.AdGroupID) && !showPaused {
			continue
		}
		keywordsStats = append(keywordsStats, e)
	}

	baselines, overall := goappleads.ComputeBaselines(keywordsStats)

	w := os.Stdout

	printAdGroupPerformance(w, showID, showPaused, keywordsStats, baselines, overall, *config, keywordsDB)
}
