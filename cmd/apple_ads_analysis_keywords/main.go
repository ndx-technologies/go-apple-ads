package appleadsanalysiskeywords

import (
	"flag"
	"fmt"
	"log"
	"math"
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

type CampaignID = goappleads.CampaignID
type AdGroupID = goappleads.AdGroupID
type KeywordID = goappleads.KeywordID

type Agg struct {
	Spend     float64
	Imp       int
	Taps      int
	Inst      int
	Days      map[time.Time]struct{}
	AdGroups  map[AdGroupID]struct{}
	Campaigns map[CampaignID]struct{}
	Bids      map[float64]struct{}
}

// numeric measures are right aligned for easy comparison.
// float numerics have fixed number of decimals for easy comparison.
var columnDefaults = map[string]fmtx.TablCol{
	"#":          {Width: 4, Alignment: fmtx.AlignRight},
	"Ad Group":   {Width: 20},
	"Bid":        {Width: 6, Alignment: fmtx.AlignRight},
	"Campaign":   {Width: 20},
	"CPI":        {Width: 7, Alignment: fmtx.AlignRight},
	"CPI/Base":   {Width: 9, Alignment: fmtx.AlignRight},
	"CTR":        {Width: 7, Alignment: fmtx.AlignRight},
	"CTR/Base":   {Width: 9, Alignment: fmtx.AlignRight},
	"CVR":        {Width: 7, Alignment: fmtx.AlignRight},
	"CVR/Base":   {Width: 9, Alignment: fmtx.AlignRight},
	"D":          {Width: 3, Alignment: fmtx.AlignRight},
	"Imp":        {Width: 6, Alignment: fmtx.AlignRight},
	"Inst":       {Width: 6, Alignment: fmtx.AlignRight},
	"Keyword":    {Width: 28},
	"Keyword ID": {Width: 10},
	"ID":         {Width: 10},
	"P(0)":       {Width: 6, Alignment: fmtx.AlignRight},
	"Spend":      {Width: 9, Alignment: fmtx.AlignRight},
	"Taps":       {Width: 6, Alignment: fmtx.AlignRight},
	"Conf":       {Width: 4},
}

type Confidence float32

func (c Confidence) Format() string {
	if c >= 0.9 {
		return fmtx.GreenS("high")
	}
	if c >= 0.7 {
		return fmtx.YellowS("med")
	}
	return fmtx.RedS("low")
}

func confidenceFromPZero(pZero float64) Confidence { return Confidence(1 - pZero) }

func divSafe[T int | float32 | float64](num, denom T) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func pZeroInstalls(taps int, convRate float64) float64 {
	if taps <= 0 || convRate <= 0 {
		return 1.0
	}
	return math.Pow(1-convRate, float64(taps))
}

func ratioVsBaseline(value, baseline float64, higherIsBetter bool, ok bool) string {
	if baseline <= 0 || !ok {
		return fmtx.DimS("n/a")
	}

	ratio := value / baseline

	if higherIsBetter {
		if ratio >= 1.0 {
			return fmtx.GreenS(fmt.Sprintf("▲%.1fx", ratio))
		} else if ratio >= 0.7 {
			return fmtx.YellowS(fmt.Sprintf("▼%.1fx", ratio))
		}
		return fmtx.RedS(fmt.Sprintf("▼%.1fx", ratio))
	} else {
		if ratio <= 1.0 {
			return fmtx.GreenS(fmt.Sprintf("▼%.1fx", ratio))
		} else if ratio <= 1.5 {
			return fmtx.YellowS(fmt.Sprintf("▲%.1fx", ratio))
		}
		return fmtx.RedS(fmt.Sprintf("▲%.1fx", ratio))
	}
}

func isKeywordNegative(kws []goappleads.KeywordInfo) bool {
	for _, k := range kws {
		if k.IsNegative {
			return true
		}
	}
	return false
}

func isKeywordPaused(ki goappleads.KeywordInfo, config goappleads.Config) bool {
	return ki.Status == goappleads.Paused || config.IsAdGroupPaused(ki.AdGroupID)
}

func aggregateByKeywords(rows []goappleads.KeywordRow) map[KeywordID]Agg {
	agg := make(map[KeywordID]Agg)
	for _, r := range rows {
		key := r.KeywordID
		a := agg[key]
		if a.Days == nil {
			a.Days = make(map[time.Time]struct{})
			a.AdGroups = make(map[AdGroupID]struct{})
			a.Bids = make(map[float64]struct{})
		}
		a.Spend += r.Spend
		a.Imp += r.Impressions
		a.Taps += r.Taps
		a.Inst += r.Installs
		a.Days[r.Day] = struct{}{}
		if r.AdGroupID != "" {
			a.AdGroups[r.AdGroupID] = struct{}{}
		}
		a.Bids[r.MaxCPTBid] = struct{}{}
		agg[key] = a
	}
	return agg
}

type KeywordStats struct {
	keyword KeywordID
	stats   Agg
}

func printBestWorstKeywords(keywords []goappleads.KeywordRow, overall goappleads.BaselineMetrics, n int, showNegativeKeywords, showID, showPaused bool, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) {
	fmtx.HeaderTo(os.Stdout, "KEYWORD RANKING")
	fmt.Println(" (only showing keywords with LOW or higher confidence in conversion rates)")

	var converting []KeywordStats

	for k, v := range aggregateByKeywords(keywords) {
		if v.Inst > 0 {
			keyword := keywordsDB.GetKeywordInfo(k)
			if kws := keywordsDB.GetKeywordsByText(keyword.Keyword); !showNegativeKeywords && isKeywordNegative(kws) {
				continue
			}
			pZero := pZeroInstalls(v.Taps, overall.CVR)
			if confidenceFromPZero(pZero) >= 0.5 {
				converting = append(converting, KeywordStats{keyword: k, stats: v})
			}
		}
	}

	sort.Slice(converting, func(i, j int) bool {
		ci := converting[i].stats.Spend / float64(converting[i].stats.Inst)
		cj := converting[j].stats.Spend / float64(converting[j].stats.Inst)
		return ci < cj
	})

	if len(converting) == 0 {
		fmt.Println(" (no converting keywords)")
		return
	}

	maxInst := 0
	for _, c := range converting {
		if c.stats.Inst > maxInst {
			maxInst = c.stats.Inst
		}
	}

	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        os.Stdout,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "#"},
			{Header: "Keyword"},
			{Header: "Campaign"},
			{Header: "Ad Group"},
			{Header: "CPI"},
			{Header: "CPI/Base"},
			{Header: "CVR"},
			{Header: "CTR"},
			{Header: "Conf"},
			{Header: "Inst"},
			{Header: "Taps"},
			{Header: "Imp"},
			{Header: "Installs", Width: 12},
		},
	}
	if showID {
		tw.Cols = slices.Insert(tw.Cols, 1, fmtx.TablCol{Header: "ID"})
	}
	if showNegativeKeywords {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "", Width: 1})
	}
	if showPaused {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "", Width: 1})
	}
	tw.WriteHeader()
	tw.WriteHeaderLine()

	topEnd := min(n, len(converting))
	worstStart := max(topEnd, len(converting)-n)

	for i := range topEnd {
		printKeywordRow(tw, i, converting[i], overall, maxInst, showID, showNegativeKeywords, showPaused, config, keywordsDB)
	}

	if worstStart > topEnd {
		fmt.Printf(fmtx.DimS("\n  ... %d keywords skipped ...\n\n"), worstStart-topEnd)
	}

	for i := worstStart; i < len(converting); i++ {
		printKeywordRow(tw, i, converting[i], overall, maxInst, showID, showNegativeKeywords, showPaused, config, keywordsDB)
	}

	fmt.Println()
}

func printKeywordRow(tw fmtx.TableWriter, i int, item KeywordStats, overall goappleads.BaselineMetrics, maxInst int, showID, showNegativeKeywords, showPaused bool, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) {
	cpi := item.stats.Spend / float64(item.stats.Inst)
	cvr := divSafe(item.stats.Inst, item.stats.Taps)
	ctr := divSafe(item.stats.Taps, item.stats.Imp)

	adgroups := make([]string, 0, len(item.stats.AdGroups))
	for ag := range item.stats.AdGroups {
		adgroup := config.GetAdGroup(ag)
		adgroups = append(adgroups, adgroup.Name)
	}
	sort.Strings(adgroups)

	keyword := keywordsDB.GetKeywordInfo(item.keyword)
	campaign := config.GetCampaign(keyword.CampaignID)

	row := []string{
		fmt.Sprintf("%d.", i+1),
		keyword.Label(),
		campaign.Name,
		strings.Join(adgroups, ", "),
		fmt.Sprintf("$%.2f", cpi),
		ratioVsBaseline(cpi, overall.CPI, false, item.stats.Inst > 0),
		fmt.Sprintf("%.1f%%", cvr*100),
		fmt.Sprintf("%.2f%%", ctr*100),
		confidenceFromPZero(pZeroInstalls(item.stats.Taps, overall.CVR)).Format(),
		strconv.Itoa(item.stats.Inst),
		strconv.Itoa(item.stats.Taps),
		strconv.Itoa(item.stats.Imp),
		fmtx.VolumeBar(item.stats.Inst, maxInst, 12),
	}

	if showID {
		row = slices.Insert(row, 1, fmtx.DimS(item.keyword.String()))
	}

	if showNegativeKeywords {
		negS := ""
		if isKeywordNegative(keywordsDB.GetKeywordsByText(keyword.Keyword)) {
			negS = fmtx.RedS("✖")
		}
		row = append(row, negS)
	}
	if showPaused {
		psdS := ""
		if isKeywordPaused(keyword, config) {
			psdS = fmtx.DimS("⏸")
		}
		row = append(row, psdS)
	}
	tw.WriteRow(row...)
}

func printWastefulWithConfidence(keywords []goappleads.KeywordRow, baselines map[CampaignID]goappleads.BaselineMetrics, overall goappleads.BaselineMetrics, minSpend float64, showNegativeKeywords, showID, showPaused bool, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) {
	fmtx.HeaderTo(os.Stdout, fmt.Sprintf("NON-CONVERTING KEYWORDS (spend > $%.2f) — CONFIDENCE ANALYSIS", minSpend))

	var wasteful []KeywordStats

	for k, v := range aggregateByKeywords(keywords) {
		if v.Inst == 0 && v.Spend > minSpend {
			wasteful = append(wasteful, KeywordStats{keyword: k, stats: v})
		}
	}

	if len(wasteful) == 0 {
		fmt.Println(" (none)")
		return
	}

	sort.Slice(wasteful, func(i, j int) bool { return wasteful[i].stats.Spend > wasteful[j].stats.Spend })

	maxImp := 0
	for _, w := range wasteful {
		if w.stats.Imp > maxImp {
			maxImp = w.stats.Imp
		}
	}
	fmt.Println()
	fmt.Printf(" Baselines: overall CVR=%.1f%%, CTR=%.2f%%\n", overall.CVR*100, overall.CTR*100)
	fmt.Println(" Confidence = 1 - P(0 installs by chance) = 1 - (1-CVR)^taps")
	fmt.Println(" Campaign-specific CVR used when available")
	cols := []fmtx.TablCol{
		{Header: "Keyword"},
		{Header: "Campaign"},
		{Header: "Ad Group"},
		{Header: "Spend"},
		{Header: "Imp"},
		{Header: "Taps"},
		{Header: "CTR"},
		{Header: "CTR/Base"},
		{Header: "P(0)"},
		{Header: "Conf"},
		{Header: "Bid"},
		{Header: "D"},
		{Header: "Impressions", Width: 12},
	}
	if showID {
		cols = append([]fmtx.TablCol{{Header: "ID"}}, cols...)
	}
	if showNegativeKeywords {
		cols = append(cols, fmtx.TablCol{Header: "", Width: 1})
	}
	if showPaused {
		cols = append(cols, fmtx.TablCol{Header: "", Width: 1})
	}
	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        os.Stdout,
		Cols:       cols,
		ColDefault: columnDefaults,
	}
	fmt.Println()
	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, w := range wasteful {
		keyword := keywordsDB.GetKeywordInfo(w.keyword)

		if keyword.Status == goappleads.Paused {
			continue
		}

		campBase, exists := baselines[keyword.CampaignID]
		if !exists {
			campBase = overall
		}
		pZero := pZeroInstalls(w.stats.Taps, campBase.CVR)
		conf := confidenceFromPZero(pZero).Format()

		ctr := divSafe(w.stats.Taps, w.stats.Imp)
		agList := make([]AdGroupID, 0, len(w.stats.AdGroups))
		for ag := range w.stats.AdGroups {
			agList = append(agList, ag)
		}
		slices.Sort(agList)
		agNamesList := make([]string, 0, len(agList))
		for _, a := range agList {
			adgroup := config.GetAdGroup(a)
			agNamesList = append(agNamesList, adgroup.Name)
		}
		ag := strings.Join(agNamesList, ", ")

		bidList := make([]float64, 0, len(w.stats.Bids))
		for b := range w.stats.Bids {
			bidList = append(bidList, b)
		}
		sort.Float64s(bidList)
		bidStrings := make([]string, 0, len(bidList))
		for _, b := range bidList {
			bidStrings = append(bidStrings, fmt.Sprintf("%.2f", b))
		}

		campaign := config.GetCampaign(keyword.CampaignID)

		row := []string{
			keyword.Label(),
			campaign.Name,
			ag,
			fmt.Sprintf("$%.2f", w.stats.Spend),
			strconv.Itoa(w.stats.Imp),
			strconv.Itoa(w.stats.Taps),
			fmt.Sprintf("%.1f%%", ctr*100),
			ratioVsBaseline(ctr, campBase.CTR, true, w.stats.Imp > 0),
			fmt.Sprintf("%.0f%%", pZero*100),
			conf,
			strings.Join(bidStrings, ", "),
			strconv.Itoa(len(w.stats.Days)),
			fmtx.VolumeBar(w.stats.Imp, maxImp, 12),
		}

		if showID {
			row = append([]string{fmtx.DimS(w.keyword.String())}, row...)
		}

		isNegative := isKeywordNegative(keywordsDB.GetKeywordsByText(keyword.Keyword))
		if showNegativeKeywords {
			var negS string
			if isNegative {
				negS = fmtx.RedS("✖")
			}
			row = append(row, negS)
		} else {
			if isNegative {
				continue
			}
		}
		if showPaused {
			psdS := ""
			if isKeywordPaused(keyword, config) {
				psdS = fmtx.DimS("⏸")
			}
			row = append(row, psdS)
		}

		tw.WriteRow(row...)
	}

	fmt.Println()
	alreadyNeg := 0
	for _, w := range wasteful {
		if isKeywordNegative(keywordsDB.GetKeywordsByText(keywordsDB.GetKeywordInfo(w.keyword).Keyword)) {
			alreadyNeg++
		}
	}
	fmt.Printf(" %sAlready negated: %d/%d keywords\n", fmtx.DimS(""), alreadyNeg, len(wasteful))
	fmt.Printf(" %s Keywords with LOW confidence (P(0) > 50%%) — top 20 by spend:\n", fmtx.YellowS("⚠"))

	type KeywordStatsWithConfidence struct {
		keyword KeywordID
		stats   Agg
		pZero   float64
	}

	var lowConf []KeywordStatsWithConfidence
	for _, w := range wasteful {
		keyword := keywordsDB.GetKeywordInfo(w.keyword)
		campBase, exists := baselines[keyword.CampaignID]
		if !exists {
			campBase = overall
		}
		pz := pZeroInstalls(w.stats.Taps, campBase.CVR)
		if pz > 0.50 {
			lowConf = append(lowConf, KeywordStatsWithConfidence{keyword: w.keyword, stats: w.stats, pZero: pz})
		}
	}
	sort.Slice(lowConf, func(i, j int) bool { return lowConf[i].stats.Spend > lowConf[j].stats.Spend })
	for i := 0; i < min(20, len(lowConf)); i++ {
		l := lowConf[i]
		keyword := keywordsDB.GetKeywordInfo(l.keyword)
		campaign := config.GetCampaign(keyword.CampaignID)

		fmt.Printf(" %s %s %s taps=%d P(0)=%.0f%% spend=$%.2f — %s\n",
			fmtx.YellowS("•"), keyword.Label(), campaign.Name, l.stats.Taps, l.pZero*100, l.stats.Spend, fmtx.YellowS("may need more data before negating"))
	}
	if len(lowConf) > 20 {
		fmt.Printf(" ... and %d more\n", len(lowConf)-20)
	}
	fmt.Printf("\n Total: %d non-converting keywords, %d with low confidence\n\n", len(wasteful), len(lowConf))
}

func printBidAnalysis(keywords []goappleads.KeywordRow, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB, showID, showPaused bool) {
	fmtx.HeaderTo(os.Stdout, "BID ANALYSIS")

	var defaultBid, setBid []KeywordStats

	for k, d := range aggregateByKeywords(keywords) {
		hasDefault := false
		for b := range d.Bids {
			if b == 0 {
				hasDefault = true
				break
			}
		}
		if hasDefault || len(d.Bids) == 0 {
			defaultBid = append(defaultBid, KeywordStats{k, d})
		} else {
			setBid = append(setBid, KeywordStats{k, d})
		}
	}
	total := len(defaultBid) + len(setBid)
	fmt.Printf(" Default bid ('--'): %s / %d keywords (%.0f%%)\n", fmtx.YellowS(strconv.Itoa(len(defaultBid))), total, divSafe(len(defaultBid), total)*100)
	fmt.Printf(" Explicit bid set: %s / %d keywords (%.0f%%)\n", fmtx.GreenS(strconv.Itoa(len(setBid))), total, divSafe(len(setBid), total)*100)

	defInst := 0
	defTaps := 0
	for _, d := range defaultBid {
		defInst += d.stats.Inst
		defTaps += d.stats.Taps
	}
	setInst := 0
	setTaps := 0
	for _, d := range setBid {
		setInst += d.stats.Inst
		setTaps += d.stats.Taps
	}
	fmt.Printf("\n Default bid → CVR=%.1f%% (%d inst / %d taps)\n", divSafe(defInst, defTaps)*100, defInst, defTaps)
	fmt.Printf(" Explicit bid → CVR=%.1f%% (%d inst / %d taps)\n", divSafe(setInst, setTaps)*100, setInst, setTaps)

	var defaultNoInst []struct {
		keyword KeywordID
		stats   Agg
	}
	for _, d := range defaultBid {
		if d.stats.Inst == 0 && d.stats.Spend > 0.3 {
			defaultNoInst = append(defaultNoInst, d)
		}
	}
	sort.Slice(defaultNoInst, func(i, j int) bool {
		return defaultNoInst[i].stats.Spend > defaultNoInst[j].stats.Spend
	})
	if len(defaultNoInst) > 0 {
		fmt.Printf("\n Top spenders with default bid and 0 installs:\n")
		for i := 0; i < min(10, len(defaultNoInst)); i++ {
			item := defaultNoInst[i]
			keyword := keywordsDB.GetKeywordInfo(item.keyword)
			campaign := config.GetCampaign(keyword.CampaignID)
			fmt.Printf(" $%.2f %s %s\n", item.stats.Spend, keyword.Label(), campaign.Name)
		}
	}
	fmt.Println()
}

func printNonConvertingKeywords(stats []goappleads.KeywordRow, config goappleads.Config, keywordsDB *goappleads.KeywordCSVDB, showID, showPaused bool) {
	fmtx.HeaderTo(os.Stdout, "KEYWORDS WITH NO IMPRESSIONS")

	impressions := make(map[KeywordID]int)

	for _, s := range stats {
		impressions[s.KeywordID] += s.Impressions
	}

	totalKeywords := 0
	noImpressionsCount := 0

	for _, ki := range keywordsDB.Keywords {
		if isKeywordNegative(keywordsDB.GetKeywordsByText(ki.Keyword)) {
			continue
		}
		totalKeywords++
		imp := impressions[ki.ID]
		if imp == 0 {
			noImpressionsCount++
		}
	}

	fmt.Printf(" Total keywords: %d, No impressions: %d (%.1f%%)\n\n", totalKeywords, noImpressionsCount, divSafe(noImpressionsCount, totalKeywords)*100)

	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        os.Stdout,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "Keyword"},
			{Header: "Campaign"},
			{Header: "Ad Group"},
			{Header: "Bid"},
		},
	}
	if showID {
		tw.Cols = append([]fmtx.TablCol{{Header: "ID"}}, tw.Cols...)
	}
	if showPaused {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "", Width: 1})
	}
	tw.WriteHeader()
	tw.WriteHeaderLine()

	count := 0
	for _, ki := range keywordsDB.Keywords {
		if isKeywordNegative(keywordsDB.GetKeywordsByText(ki.Keyword)) {
			continue
		}
		imp := impressions[ki.ID]
		if imp == 0 && count < 10 {
			campaignName := config.GetCampaign(ki.CampaignID).Name
			adGroupName := config.GetAdGroup(ki.AdGroupID).Name

			row := []string{
				ki.Label(),
				campaignName,
				adGroupName,
				fmt.Sprintf("$%.2f", ki.Bid),
			}

			if showID {
				row = append([]string{fmtx.DimS(ki.ID.String())}, row...)
			}
			if showPaused {
				psdS := ""
				if isKeywordPaused(ki, config) {
					psdS = fmtx.DimS("⏸")
				}
				row = append(row, psdS)
			}

			tw.WriteRow(row...)
			count++
		}
	}

	fmt.Println()
}

const DocShort string = "keywords stats (CPI, CVR, CTR, Installs, Spend, ...), bids analysis"
const doc string = `
Apple Ads Keywords Analysis — best/worst performers, bids analysis.

Timezone: UTC
Currency: USD

`

func Run(args []string) {
	flag := flag.NewFlagSet("analyse keywords", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		showID, showPaused            bool
		showNegativeKeywords          bool
		minSpend                      float64
		topN                          int
		campaignIDsStr, adGroupIDsStr string
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "data", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.StringVar(&keywordStatsCSV, "keyword_stats_csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.BoolVar(&showNegativeKeywords, "negative-keywords", false, "show negative keywords")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&showPaused, "paused", false, "include paused keywords, adgroups, campaigns")
	flag.Float64Var(&minSpend, "min-spend", 0.40, "min spend threshold for wasteful keywords")
	flag.IntVar(&topN, "top-n", 300, "number of best/worst keywords to show")
	flag.BoolVar(&fmtx.EnableColor, "color", true, "colorize output")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of ad group IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var keepCampaign map[CampaignID]bool
	if len(campaignIDsStr) > 0 {
		keepCampaign = make(map[CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaign[CampaignID(id)] = true
		}
	}

	var keepAdGroup map[AdGroupID]bool
	if len(adGroupIDsStr) > 0 {
		keepAdGroup = make(map[AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroup[AdGroupID(id)] = true
		}
	}

	var keywordsStats []goappleads.KeywordRow
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaign != nil && !keepCampaign[e.CampaignID] {
			continue
		}
		if keepAdGroup != nil && !keepAdGroup[e.AdGroupID] {
			continue
		}
		if !showPaused {
			keywordInfo := keywordsDB.GetKeywordInfo(e.KeywordID)
			if keywordInfo.Status == goappleads.Paused || config.IsAdGroupPaused(e.AdGroupID) {
				continue
			}
		}
		keywordsStats = append(keywordsStats, e)
	}

	// seed keywordsDB with keyword text from the stats CSV for any IDs not already loaded from config CSVs
	var missingKeywords []goappleads.KeywordRow
	for _, r := range keywordsStats {
		if _, ok := keywordsDB.Keywords[r.KeywordID]; !ok {
			keyword := goappleads.KeywordInfo{
				ID:         r.KeywordID,
				CampaignID: r.CampaignID,
				AdGroupID:  r.AdGroupID,
				Keyword:    r.Keyword,
				MatchType:  r.MatchType,
			}
			keywordsDB.Keywords[r.KeywordID] = keyword
			missingKeywords = append(missingKeywords, r)
		}
	}
	if len(missingKeywords) > 0 {
		fmt.Fprintf(os.Stderr, "keywords not in keywords list but in stats: %d\n", len(missingKeywords))
	}

	baselines, overall := goappleads.ComputeBaselines(keywordsStats)

	printBestWorstKeywords(keywordsStats, overall, topN, showNegativeKeywords, showID, showPaused, *config, keywordsDB)
	printWastefulWithConfidence(keywordsStats, baselines, overall, minSpend, showNegativeKeywords, showID, showPaused, *config, keywordsDB)
	printBidAnalysis(keywordsStats, *config, keywordsDB, showID, showPaused)
	printNonConvertingKeywords(keywordsStats, *config, keywordsDB, showID, showPaused)
}
