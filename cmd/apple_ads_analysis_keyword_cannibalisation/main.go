package appleadsanalysiskeywordcannibalisation

import (
	"flag"
	"io"
	"log"
	"math"
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

// cannibGroupColor colors the best entry green and worst red within a group.
// lowerIsBetter swaps directions. Zeros are excluded from min/max but their presence
// provides contrast: if only one distinct positive value exists alongside zeros,
// that positive entry is still marked green. All values equal → no color.
func cannibGroupColor(s string, i int, vals []float64, lowerIsBetter bool) string {
	minI, maxI := -1, -1
	minV, maxV := math.MaxFloat64, -math.MaxFloat64
	hasZero := false
	for j, v := range vals {
		if v > 0 {
			if v < minV {
				minV = v
				minI = j
			}
			if v > maxV {
				maxV = v
				maxI = j
			}
		} else {
			hasZero = true
		}
	}
	if minI < 0 {
		return s // no positive values at all
	}
	// All positive values equal and no zeros → truly uniform, skip coloring.
	if minV == maxV && !hasZero {
		return s
	}
	greenI, redI := maxI, minI
	if lowerIsBetter {
		greenI, redI = minI, maxI
	}
	if i == greenI {
		return fmtx.GreenS(s)
	}
	// Only mark red if there are distinct positive values (not just zeros providing contrast).
	if minV != maxV && i == redI {
		return fmtx.RedS(s)
	}
	return s
}

// cannibCPIColor treats cpis[i]==0 as ∞ (no installs): marks it red when others have a real
// CPI, and marks the lowest real CPI green.
func cannibCPIColor(s string, i int, cpis []float64) string {
	anyReal := false
	for _, v := range cpis {
		if v > 0 {
			anyReal = true
			break
		}
	}
	if anyReal && cpis[i] == 0 {
		return fmtx.RedS(s)
	}
	return cannibGroupColor(s, i, cpis, true)
}

// cannibRatioColor applies cannibGroupColor (higher is better) but also marks a zero-ratio
// entry red when at least one other entry has a positive ratio.
// The denominator is not checked: if another entry has a real ratio, zero is always worse.
func cannibRatioColor(s string, i int, ratios []float64) string {
	anyPositive := false
	for _, v := range ratios {
		if v > 0 {
			anyPositive = true
			break
		}
	}
	if anyPositive && ratios[i] == 0 {
		return fmtx.RedS(s)
	}
	return cannibGroupColor(s, i, ratios, false)
}

type AdGroupStats struct {
	OrigKeyword string
	AdGroupID   goappleads.AdGroupID
	KeywordID   goappleads.KeywordID
	Spend       float64
	Impressions int
	Taps        int
	Installs    int
	MaxBid      float64
}

type CannibGroup struct {
	Keyword     string
	OrigKeyword string
	CampaignID  goappleads.CampaignID
	Entries     []AdGroupStats
}

type CannibalizationAnalyzer interface {
	Add(goappleads.KeywordRow)
	Finalize() []CannibGroup
}

type AdGroupKey struct {
	Keyword    string
	CampaignID goappleads.CampaignID
	AdGroupID  goappleads.AdGroupID
}

type AppleAdsKeywordsCannibalisationAnalyzer struct {
	Config     *goappleads.Config
	KeywordsDB *goappleads.KeywordCSVDB
	byKey      map[AdGroupKey]AdGroupStats
}

func NewAppleAdsKeywordsCannibalisationAnalyzer(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) *AppleAdsKeywordsCannibalisationAnalyzer {
	return &AppleAdsKeywordsCannibalisationAnalyzer{
		Config:     config,
		KeywordsDB: keywordsDB,
		byKey:      make(map[AdGroupKey]AdGroupStats),
	}
}

func (a *AppleAdsKeywordsCannibalisationAnalyzer) Add(r goappleads.KeywordRow) {
	if r.Keyword == "" || r.MatchType == goappleads.Auto || r.Keyword == "Search Match" || r.Keyword == "--" {
		return
	}

	key := AdGroupKey{
		Keyword:    strings.ToLower(r.Keyword),
		CampaignID: r.CampaignID,
		AdGroupID:  r.AdGroupID,
	}
	if _, ok := a.byKey[key]; !ok {
		a.byKey[key] = AdGroupStats{
			OrigKeyword: r.Keyword,
			AdGroupID:   r.AdGroupID,
			KeywordID:   r.KeywordID,
		}
	}

	s := a.byKey[key]

	if s.KeywordID == "" {
		s.KeywordID = r.KeywordID
	}

	s.Spend += r.Spend
	s.Impressions += r.Impressions
	s.Taps += r.Taps
	s.Installs += r.Installs

	if r.MaxCPTBid > 0 && r.MaxCPTBid > s.MaxBid {
		s.MaxBid = r.MaxCPTBid
	}

	a.byKey[key] = s
}

func (a *AppleAdsKeywordsCannibalisationAnalyzer) Finalize() []CannibGroup {
	type GroupKey struct {
		Keyword    string
		CampaignID goappleads.CampaignID
	}

	groups := make(map[GroupKey][]AdGroupStats)
	for key, stats := range a.byKey {
		gk := GroupKey{key.Keyword, key.CampaignID}
		groups[gk] = append(groups[gk], stats)
	}

	var cannibGroups []CannibGroup
	for gk, entries := range groups {
		kws := make([]AdGroupStats, 0, len(entries))

		for _, e := range entries {
			keyword := a.KeywordsDB.GetKeywordInfo(e.KeywordID)
			if keyword.IsNegative || keyword.IsZero() || keyword.Status == goappleads.Paused {
				continue
			}

			campaign := a.Config.GetCampaign(keyword.CampaignID)
			if campaign.Status == goappleads.Paused {
				continue
			}

			adgroup := a.Config.GetAdGroup(keyword.AdGroupID)
			if adgroup.Status == goappleads.Paused {
				continue
			}

			kws = append(kws, e)
		}

		if len(kws) > 1 {
			cannibGroups = append(cannibGroups, CannibGroup{
				Keyword:     gk.Keyword,
				OrigKeyword: kws[0].OrigKeyword,
				CampaignID:  gk.CampaignID,
				Entries:     kws,
			})
		}
	}

	return cannibGroups
}

func printCannibalizationAnalysis(
	w io.StringWriter,
	config goappleads.Config,
	keywordDB interface {
		GetKeywordInfo(id goappleads.KeywordID) goappleads.KeywordInfo
	},
	showID, showPaused bool,
	cannibGroups []CannibGroup,
) {
	fmtx.HeaderTo(w, "Keyword Cannibalisation Analysis")

	sort.Slice(cannibGroups, func(i, j int) bool {
		if cannibGroups[i].Keyword != cannibGroups[j].Keyword {
			return cannibGroups[i].Keyword < cannibGroups[j].Keyword
		}
		return config.GetCampaign(cannibGroups[i].CampaignID).Name < config.GetCampaign(cannibGroups[j].CampaignID).Name
	})

	for i := range cannibGroups {
		sort.Slice(cannibGroups[i].Entries, func(a, b int) bool {
			return cannibGroups[i].Entries[a].Spend > cannibGroups[i].Entries[b].Spend
		})
	}

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Keyword", Width: 28},
			{Header: "Campaign", Width: 20},
			{Header: "Ad Group", Width: 20},
			{Header: "Bid", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "CPI", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CVR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "CTR", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Imp", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Spend(USD)", Width: 12, Alignment: fmtx.AlignRight},
		},
	}

	subheader := []string{
		strconv.Itoa(len(cannibGroups)),
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{{Header: "ID"}}, tw.Cols...)
		subheader = append([]string{""}, subheader...)
	}

	if showPaused {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "Paused", Width: 6})
	}

	tw.WriteHeader()
	tw.WriteSubHeader(subheader...)
	tw.WriteHeaderLine()

	for _, g := range cannibGroups {
		campaign := config.GetCampaign(g.CampaignID)
		n := len(g.Entries)

		bids := make([]float64, n)
		cpis := make([]float64, n)
		cvrs := make([]float64, n)
		ctrs := make([]float64, n)

		for i, e := range g.Entries {
			bids[i] = e.MaxBid
			if e.Installs > 0 {
				cpis[i] = e.Spend / float64(e.Installs)
			}
			if e.Taps > 0 {
				cvrs[i] = float64(e.Installs) / float64(e.Taps)
			}
			if e.Impressions > 0 {
				ctrs[i] = float64(e.Taps) / float64(e.Impressions)
			}
		}

		for i, e := range g.Entries {
			kwS := ""
			campS := ""
			if i == 0 {
				kwS = g.OrigKeyword
				campS = campaign.Name
			}

			bidStr := "--"
			if e.MaxBid > 0 {
				bidStr = strconv.FormatFloat(e.MaxBid, 'f', 2, 64)
			}

			cpiStr := "∞"
			if e.Installs > 0 {
				cpiStr = strconv.FormatFloat(cpis[i], 'f', 2, 64)
			}

			row := []string{
				kwS,
				campS,
				config.GetAdGroup(e.AdGroupID).Name,
				cannibGroupColor(bidStr, i, bids, true),
				cannibCPIColor(cpiStr, i, cpis),
				cannibRatioColor(strconv.FormatFloat(cvrs[i]*100, 'f', 1, 64)+"%", i, cvrs),
				cannibRatioColor(strconv.FormatFloat(ctrs[i]*100, 'f', 2, 64)+"%", i, ctrs),
				strconv.Itoa(e.Installs),
				strconv.Itoa(e.Taps),
				strconv.Itoa(e.Impressions),
				strconv.FormatFloat(e.Spend, 'f', 2, 64),
			}

			if showID {
				row = append([]string{fmtx.DimS(e.KeywordID.String())}, row...)
			}

			if showPaused {
				pausedStr := ""

				kw := keywordDB.GetKeywordInfo(e.KeywordID)

				if kw.Status == goappleads.Paused || config.IsAdGroupPaused(kw.AdGroupID) {
					pausedStr = fmtx.DimS("⏸")
				}

				row = append(row, pausedStr)
			}

			tw.WriteRow(row...)
		}
		w.WriteString("\n")
	}

	w.WriteString("\n")
}

const DocShort string = "detect collision of keywords"
const doc string = "Keyword Cannibalisation is when the same keyword appears in multiple ad groups within the same campaign.\n\n"

func Run(args []string) {
	flag := flag.NewFlagSet("analyse keywords cannibalisation", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		showID, showPaused            bool
		verbose                       bool
		campaignIDsStr, adGroupIDsStr string
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&fmtx.EnableColor, "color", true, "colorize output")
	flag.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	flag.BoolVar(&showPaused, "paused", false, "include paused keywords, adgroups, campaigns")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of ad group IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var keepAdGroup map[goappleads.AdGroupID]bool
	if len(adGroupIDsStr) > 0 {
		keepAdGroup = make(map[goappleads.AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroup[goappleads.AdGroupID(id)] = true
		}
	}

	var keepCampaign map[goappleads.CampaignID]bool
	if len(campaignIDsStr) > 0 {
		keepCampaign = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaign[goappleads.CampaignID(id)] = true
		}
	}

	analyzer := NewAppleAdsKeywordsCannibalisationAnalyzer(config, keywordsDB)

	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepAdGroup != nil && !keepAdGroup[e.AdGroupID] {
			continue
		}
		if keepCampaign != nil && !keepCampaign[e.CampaignID] {
			continue
		}

		analyzer.Add(e)
	}

	for _, kw := range keywordsDB.Keywords {
		if kw.IsNegative || kw.IsZero() {
			continue
		}
		analyzer.Add(goappleads.KeywordRow{KeywordID: kw.ID, CampaignID: kw.CampaignID, AdGroupID: kw.AdGroupID, Keyword: kw.Keyword})
	}

	groups := analyzer.Finalize()

	w := os.Stdout

	if len(groups) == 0 {
		w.WriteString(fmtx.GreenS("ok") + " each keyword(num=" + strconv.Itoa(len(keywordsDB.Keywords)) + ") appears at most once per campaign(num=" + strconv.Itoa(len(config.Campaigns)) + ")\n")
		return
	}

	if verbose {
		printCannibalizationAnalysis(w, *config, keywordsDB, showID, showPaused, groups)
	} else {
		w.WriteString(fmtx.RedS("error") + " " + strconv.Itoa(len(groups)) + " keyword cannibalisation groups found (run with -v for details)\n")
	}
	os.Exit(1)
}
