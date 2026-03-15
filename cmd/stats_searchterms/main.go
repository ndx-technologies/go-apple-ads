package statssearchterms

import (
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	"github.com/ndx-technologies/geo"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/slicex"
	"github.com/ndx-technologies/timex"
)

type Agg struct {
	Spend     float64
	Imp       int
	Taps      int
	Inst      int
	Days      map[time.Time]struct{}
	AdGroups  map[goappleads.AdGroupID]struct{}
	Campaigns map[goappleads.CampaignID]struct{}
	Bids      map[string]struct{}
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
	"Neg":        {Width: 5, Alignment: fmtx.AlignRight},
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

func confidenceFromDaysImpressions[D, I int | int32 | int64 | float32 | float64](days D, impressions I) Confidence {
	switch {
	case days >= 7 && impressions >= 300:
		return 0.9
	case days <= 2 || impressions < 50:
		return 0.1
	default:
		return 0.5
	}
}

type searchTermInfoKey struct {
	Country    geo.Country
	SearchTerm string
}

type searchTermInfoAgg struct {
	Impressions      int
	Spend            float64
	Taps             int
	Installs         int
	Rank             int
	SearchPopularity int
	ImpShareFrom     float64
	ImpShareTo       float64
	Days             int
}

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

func aggregateSearchTerms(rows []goappleads.SearchTermRow) map[string]Agg {
	agg := make(map[string]Agg)
	for _, r := range rows {
		term := r.SearchTerm
		a := agg[term]
		if a.Days == nil {
			a.Days = make(map[time.Time]struct{})
		}
		if a.AdGroups == nil {
			a.AdGroups = make(map[goappleads.AdGroupID]struct{})
		}
		if a.Campaigns == nil {
			a.Campaigns = make(map[goappleads.CampaignID]struct{})
		}
		a.Spend += r.Spend
		a.Imp += r.Impressions
		a.Taps += r.Taps
		a.Inst += r.Installs
		a.Days[r.Day] = struct{}{}
		if r.AdGroupID != "" {
			a.AdGroups[r.AdGroupID] = struct{}{}
		}
		if r.CampaignID != "" {
			a.Campaigns[r.CampaignID] = struct{}{}
		}
		agg[term] = a
	}
	return agg
}

func computeSearchTermBaselines(rows []goappleads.SearchTermRow) goappleads.BaselineMetrics {
	var total Agg
	for _, r := range rows {
		total.Spend += r.Spend
		total.Imp += r.Impressions
		total.Taps += r.Taps
		total.Inst += r.Installs
	}
	return goappleads.BaselineMetrics{
		CTR:   divSafe(total.Taps, total.Imp),
		CVR:   divSafe(total.Inst, total.Taps),
		CPI:   divSafe(total.Spend, float64(total.Inst)),
		Spend: total.Spend,
		Imp:   total.Imp,
		Taps:  total.Taps,
		Inst:  total.Inst,
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

func printSearchTermsOverview(w io.StringWriter, searchTerms []goappleads.SearchTermRow) {
	fmtx.HeaderTo(w, "SEARCH TERMS — OVERVIEW")
	byTerm := aggregateSearchTerms(searchTerms)
	stBase := computeSearchTermBaselines(searchTerms)
	totalTerms := len(byTerm)
	converting := 0
	for _, v := range byTerm {
		if v.Inst > 0 {
			converting++
		}
	}
	nonConverting := totalTerms - converting
	lowVol := byTerm["Low Volume"]
	w.WriteString(fmt.Sprintf(" Total unique terms: %s\n", strconv.Itoa(totalTerms)))
	w.WriteString(fmt.Sprintf(" Converting: %s (%d%%)\n", fmtx.GreenS(strconv.Itoa(converting)), int(divSafe(converting, totalTerms)*100)))
	w.WriteString(fmt.Sprintf(" Non-converting: %s (%d%%)\n\n", fmtx.RedS(strconv.Itoa(nonConverting)), int(divSafe(nonConverting, totalTerms)*100)))
	w.WriteString(fmt.Sprintf(" Overall: CVR=%.1f%%, CTR=%.2f%%, CPI=$%.2f\n\n", stBase.CVR*100, stBase.CTR*100, stBase.CPI))
	w.WriteString(fmt.Sprintf(" Low Volume bucket: %d inst, %d taps, $%.2f spend, %d imp\n", lowVol.Inst, lowVol.Taps, lowVol.Spend, lowVol.Imp))
	w.WriteString(" (aggregated rare terms, cannot be individually analyzed)\n")
	w.WriteString("\n")
}

func printSearchTermsTopPerformers(w io.StringWriter, searchTerms []goappleads.SearchTermRow, n int, showNegativeKeywords, showPaused bool, keywordsDB *goappleads.KeywordCSVDB, config goappleads.Config) {
	fmtx.HeaderTo(w, "SEARCH TERMS — TOP PERFORMERS (promote to [exact] keyword)")
	stBase := computeSearchTermBaselines(searchTerms)
	byTerm := aggregateSearchTerms(searchTerms)
	var converting []struct {
		term string
		data Agg
	}
	for t, v := range byTerm {
		if keywords := keywordsDB.GetKeywordsByText(t); v.Inst > 0 && t != "Low Volume" && len(keywords) > 0 {
			if !showNegativeKeywords && isKeywordNegative(keywords) {
				continue
			}
			converting = append(converting, struct {
				term string
				data Agg
			}{t, v})
		}
	}
	sort.Slice(converting, func(i, j int) bool {
		ci := converting[i].data.Spend / float64(converting[i].data.Inst)
		cj := converting[j].data.Spend / float64(converting[j].data.Inst)
		return ci < cj
	})
	if len(converting) == 0 {
		w.WriteString(" (no converting search terms)\n")
		return
	}
	maxInst := 0
	for _, c := range converting {
		if c.data.Inst > maxInst {
			maxInst = c.data.Inst
		}
	}
	w.WriteString(fmt.Sprintf("\n Baselines (search terms): CVR=%.1f%%, CTR=%.2f%%, CPI=$%.2f\n", stBase.CVR*100, stBase.CTR*100, stBase.CPI))
	w.WriteString(fmtx.GreenS(" ★") + " = great CPI (<50% of baseline), consider promoting to [exact]\n")
	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        w,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "#"},
			{Header: "Search Term", Width: 32},
			{Header: "CPI"},
			{Header: "CPI/Base"},
			{Header: "CVR"},
			{Header: "CTR"},
			{Header: "Inst"},
			{Header: "Taps"},
			{Header: "Imp"},
			{Header: "D"},
			{Header: "Campaign"},
			{Header: "Ad Group"},
			{Header: "Action", Width: 22},
		},
	}

	if showPaused {
		tw.Cols = slices.Insert(tw.Cols, 12, fmtx.TablCol{Header: "", Width: 1})
	}

	w.WriteString("\n")
	tw.WriteHeader()
	tw.WriteHeaderLine()

	for i, item := range converting[:min(n, len(converting))] {
		keywords := keywordsDB.GetKeywordsByText(item.term)
		isExact := false
		for _, k := range keywords {
			if !k.IsNegative {
				isExact = true
				break
			}
			continue
		}

		cpi := item.data.Spend / float64(item.data.Inst)
		cvr := divSafe(item.data.Inst, item.data.Taps)
		ctr := divSafe(item.data.Taps, item.data.Imp)

		var action string
		if !isExact {
			if cpi < stBase.CPI*0.5 && item.data.Inst >= 2 {
				action = fmtx.GreenS("★ promote to [exact]")
			} else if cpi < stBase.CPI*0.7 && item.data.Inst >= 2 {
				action = fmtx.YellowS("consider [exact]")
			} else if item.data.Inst == 1 {
				action = fmtx.DimS("monitor (1 inst)")
			}
		}

		if isKeywordNegative(keywords) {
			action += " " + fmtx.RedS("(currently negated)")
		}

		campNames := make([]string, 0, len(item.data.Campaigns))
		for cid := range item.data.Campaigns {
			campaign := config.GetCampaign(cid)
			campNames = append(campNames, campaign.Name)
		}
		sort.Strings(campNames)

		agNames := make([]string, 0, len(item.data.AdGroups))
		for agid := range item.data.AdGroups {
			ag := config.GetAdGroup(agid)
			name := ag.Name
			if ag.Status == goappleads.Paused {
				name += " ⏸"
			}
			agNames = append(agNames, name)
		}
		sort.Strings(agNames)

		row := []string{
			fmt.Sprintf("%d.", i+1),
			item.term,
			fmt.Sprintf("$%.2f", cpi),
			ratioVsBaseline(cpi, stBase.CPI, false, stBase.CPI > 0),
			fmt.Sprintf("%.1f%%", cvr*100),
			fmt.Sprintf("%.2f%%", ctr*100),
			strconv.Itoa(item.data.Inst),
			strconv.Itoa(item.data.Taps),
			strconv.Itoa(item.data.Imp),
			fmt.Sprintf("%dd", len(item.data.Days)),
			strings.Join(campNames, ", "),
			strings.Join(agNames, ", "),
			action,
		}

		if config.IsAdGroupPausedAll(slices.Collect(maps.Keys(item.data.AdGroups))) {
			if !showPaused {
				continue
			}
			row = slices.Insert(row, 12, fmtx.RedS("⏸"))
		}

		tw.WriteRow(row...)
	}

	great := 0
	for _, c := range converting {
		keywords := keywordsDB.GetKeywordsByText(c.term)
		isExact := false
		for _, k := range keywords {
			if !k.IsNegative {
				isExact = true
				break
			}
			continue
		}
		if isExact {
			continue
		}

		if stBase.CPI > 0 && c.data.Spend/float64(c.data.Inst) < stBase.CPI*0.5 && c.data.Inst >= 2 {
			great++
		}
	}
	if great > 0 {
		w.WriteString(fmt.Sprintf("\n %s %d search terms with CPI <50%% baseline — strong candidates for [exact] keywords\n", fmtx.GreenS("★"), great))
	}
	w.WriteString("\n")
}

func printSearchTermsNewKeywordCandidates(w io.StringWriter, searchTerms []goappleads.SearchTermRow, n int, showPaused bool, keywordsDB *goappleads.KeywordCSVDB, config goappleads.Config) {
	fmtx.HeaderTo(w, "SEARCH TERMS — NEW KEYWORD CANDIDATES (converting but not in keyword list)")
	stBase := computeSearchTermBaselines(searchTerms)
	byTerm := aggregateSearchTerms(searchTerms)

	type candidate struct {
		term string
		data Agg
	}
	var candidates []candidate
	for t, v := range byTerm {
		if t == "Low Volume" {
			continue
		}
		if v.Inst == 0 {
			continue
		}
		if keywords := keywordsDB.GetKeywordsByText(t); len(keywords) > 0 {
			continue
		}
		candidates = append(candidates, candidate{t, v})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].data.Inst != candidates[j].data.Inst {
			return candidates[i].data.Inst > candidates[j].data.Inst
		}
		ci := candidates[i].data.Spend / float64(candidates[i].data.Inst)
		cj := candidates[j].data.Spend / float64(candidates[j].data.Inst)
		return ci < cj
	})

	if len(candidates) == 0 {
		w.WriteString(" (none)\n")
		w.WriteString("\n")
		return
	}

	w.WriteString(fmt.Sprintf("\n Baselines (search terms): CVR=%.1f%%, CTR=%.2f%%, CPI=$%.2f\n", stBase.CVR*100, stBase.CTR*100, stBase.CPI))
	w.WriteString(fmtx.GreenS(" ★") + " = great CPI (<50% of baseline) — strong add candidate\n")
	w.WriteString("\n")

	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        w,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "#"},
			{Header: "Search Term", Width: 36},
			{Header: "CPI"},
			{Header: "CPI/Base"},
			{Header: "CVR"},
			{Header: "Inst"},
			{Header: "Taps"},
			{Header: "Imp"},
			{Header: "Campaign"},
			{Header: "Ad Group"},
			{Header: "Action", Width: 22},
		},
	}
	if showPaused {
		tw.Cols = slices.Insert(tw.Cols, 10, fmtx.TablCol{Header: "", Width: 1})
	}
	tw.WriteHeader()
	tw.WriteHeaderLine()

	for i, item := range candidates[:min(n, len(candidates))] {
		paused := config.IsAdGroupPausedAll(slices.Collect(maps.Keys(item.data.AdGroups)))
		if paused && !showPaused {
			continue
		}

		cpi := item.data.Spend / float64(item.data.Inst)
		cvr := divSafe(item.data.Inst, item.data.Taps)

		var action string
		if !paused {
			if cpi < stBase.CPI*0.5 && item.data.Inst >= 2 {
				action = fmtx.GreenS("★ add as [exact]")
			} else if item.data.Inst >= 3 {
				action = fmtx.GreenS("add as [exact]")
			} else if item.data.Inst >= 2 {
				action = fmtx.YellowS("consider adding")
			} else {
				action = fmtx.DimS("monitor (1 inst)")
			}
		}

		campNames := make([]string, 0, len(item.data.Campaigns))
		for id := range item.data.Campaigns {
			campaign := config.GetCampaign(id)
			campNames = append(campNames, campaign.Name)
		}
		sort.Strings(campNames)

		agNames := make([]string, 0, len(item.data.AdGroups))
		for id := range item.data.AdGroups {
			adgroup := config.GetAdGroup(id)
			agNames = append(agNames, adgroup.Name)
		}
		sort.Strings(agNames)

		row := []string{
			fmt.Sprintf("%d.", i+1),
			item.term,
			fmt.Sprintf("$%.2f", cpi),
			ratioVsBaseline(cpi, stBase.CPI, false, stBase.CPI > 0),
			fmt.Sprintf("%.1f%%", cvr*100),
			strconv.Itoa(item.data.Inst),
			strconv.Itoa(item.data.Taps),
			strconv.Itoa(item.data.Imp),
			strings.Join(campNames, ", "),
			strings.Join(agNames, ", "),
			action,
		}

		if showPaused {
			if paused {
				row = slices.Insert(row, 10, fmtx.RedS("⏸"))
			} else {
				row = slices.Insert(row, 10, "")
			}
		}

		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func printSearchTermsUnderperformers(w io.StringWriter, searchTerms []goappleads.SearchTermRow, minSpend float64, showNegativeKeywords, showPaused bool, keywordsDB *goappleads.KeywordCSVDB, config goappleads.Config) {
	fmtx.HeaderTo(w, fmt.Sprintf("SEARCH TERMS — UNDERPERFORMERS (spend > $%.2f) — negate candidates", minSpend))
	stBase := computeSearchTermBaselines(searchTerms)

	type TermInfo struct {
		term  string
		data  Agg
		pZero float64
	}

	var wasteful []TermInfo

	for t, v := range aggregateSearchTerms(searchTerms) {
		if keywords := keywordsDB.GetKeywordsByText(t); v.Inst == 0 && v.Spend >= minSpend && t != "Low Volume" && len(keywords) > 0 {
			if !showNegativeKeywords && isKeywordNegative(keywords) {
				continue
			}
			wasteful = append(wasteful, TermInfo{term: t, data: v})
		}
	}
	sort.Slice(wasteful, func(i, j int) bool { return wasteful[i].data.Spend > wasteful[j].data.Spend })

	if len(wasteful) == 0 {
		w.WriteString(" (none)\n")
		return
	}

	maxImp := 0
	for _, item := range wasteful {
		if item.data.Imp > maxImp {
			maxImp = item.data.Imp
		}
	}
	w.WriteString(fmt.Sprintf("\n Baselines: CVR=%.1f%%, CTR=%.2f%%\n", stBase.CVR*100, stBase.CTR*100))
	w.WriteString(fmtx.RedS(" ✗") + " = high confidence (P(0)<10%) that term truly doesn't convert → negate\n")
	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        w,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "Search Term", Width: 32},
			{Header: "Spend"},
			{Header: "Imp"},
			{Header: "Taps"},
			{Header: "CTR"},
			{Header: "P(0)"},
			{Header: "Conf"},
			{Header: "D"},
			{Header: "Campaign"},
			{Header: "Ad Group"},
			{Header: "Action", Width: 16},
			{Header: "Impressions", Width: 11},
		},
	}
	w.WriteString("\n")
	tw.WriteHeader()
	tw.WriteHeaderLine()

	var highConf, medConf, lowConf []TermInfo
	for _, item := range wasteful {
		if config.IsAdGroupPausedAll(slices.Collect(maps.Keys(item.data.AdGroups))) {
			if !showPaused {
				continue
			}
		}

		pZero := pZeroInstalls(item.data.Taps, stBase.CVR)
		conf := confidenceFromPZero(pZero).Format()
		confidence := 1 - pZero

		action := ""
		if confidence >= 0.90 {
			action = fmtx.RedS("✗ negate")
			highConf = append(highConf, TermInfo{term: item.term, data: item.data, pZero: pZero})
		} else if confidence >= 0.70 {
			action = fmtx.YellowS("likely negate")
			medConf = append(medConf, TermInfo{term: item.term, data: item.data, pZero: pZero})
		} else {
			action = fmtx.DimS("monitor")
			lowConf = append(lowConf, TermInfo{term: item.term, data: item.data, pZero: pZero})
		}

		campNames := make([]string, 0, len(item.data.Campaigns))
		for cid := range item.data.Campaigns {
			campNames = append(campNames, config.GetCampaign(cid).Name)
		}
		sort.Strings(campNames)
		agNames := make([]string, 0, len(item.data.AdGroups))
		for agid := range item.data.AdGroups {
			agNames = append(agNames, config.GetAdGroup(agid).Name)
		}
		sort.Strings(agNames)

		tw.WriteRow(
			item.term,
			fmt.Sprintf("$%.2f", item.data.Spend),
			strconv.Itoa(item.data.Imp),
			strconv.Itoa(item.data.Taps),
			fmt.Sprintf("%.2f%%", divSafe(item.data.Taps, item.data.Imp)*100),
			fmt.Sprintf("%.0f%%", pZero*100),
			conf,
			fmt.Sprintf("%dd", len(item.data.Days)),
			strings.Join(campNames, ", "),
			strings.Join(agNames, ", "),
			action,
			fmtx.VolumeBar(item.data.Imp, maxImp, 11),
		)
	}

	var totalSpentHighConf, totalSpentMedConf, totalSpentLowConf float64
	for _, h := range highConf {
		totalSpentHighConf += h.data.Spend
	}
	for _, m := range medConf {
		totalSpentMedConf += m.data.Spend
	}
	for _, l := range lowConf {
		totalSpentLowConf += l.data.Spend
	}

	w.WriteString("\n")
	w.WriteString("SUMMARY:\n")
	w.WriteString(fmt.Sprintf(" %s High-confidence negates (P(0)<10%%): %d terms ($%.2f wasted)\n", fmtx.RedS("✗"), len(highConf), totalSpentHighConf))
	w.WriteString(fmt.Sprintf(" %s Med-confidence likely negates (P(0) 10-30%%): %d terms ($%.2f)\n", fmtx.YellowS("~"), len(medConf), totalSpentMedConf))
	w.WriteString(fmt.Sprintf(" %s Low-confidence/monitor: %d terms ($%.2f)\n", fmtx.DimS("?"), len(lowConf), totalSpentLowConf))

	if len(highConf) > 0 {
		w.WriteString("\n Recommended negative keywords (HIGH confidence):\n")
		for i := 0; i < min(15, len(highConf)); i++ {
			h := highConf[i]
			w.WriteString(fmt.Sprintf(" %s [%s] (taps=%d, P(0)=%.0f%%, spend=$%.2f)\n", fmtx.RedS("✗"), h.term, h.data.Taps, h.pZero*100, h.data.Spend))
		}
		if len(highConf) > 15 {
			w.WriteString(fmt.Sprintf(" ... and %d more\n", len(highConf)-15))
		}
	}
	w.WriteString("\n")
}

func printSearchTermImpressionShare(w io.StringWriter, rows []goappleads.SearchTermInfo, n int, keywordsDB *goappleads.KeywordCSVDB, config goappleads.Config, baselines map[goappleads.CampaignID]goappleads.BaselineMetrics, showNegativeKeywords, showID, showPaused bool) {
	fmtx.HeaderTo(w, "SEARCH TERM IMPRESSION SHARE — TOP TERMS")
	if len(rows) == 0 {
		w.WriteString(" no data\n")
		return
	}

	// aggregate by (country, search term)
	byKey := make(map[searchTermInfoKey]*searchTermInfoAgg)
	for _, r := range rows {
		k := searchTermInfoKey{Country: r.Country, SearchTerm: r.SearchTerm}
		a := byKey[k]
		if a == nil {
			a = &searchTermInfoAgg{}
			byKey[k] = a
		}
		a.Impressions += r.Impressions
		a.Spend += r.Spend
		a.Taps += r.Taps
		a.Installs += r.Installs
		a.Days++
		// keep best (lowest) rank seen
		if a.Rank == 0 || (r.Rank > 0 && r.Rank < a.Rank) {
			a.Rank = r.Rank
		}
		if r.SearchPopularity > a.SearchPopularity {
			a.SearchPopularity = r.SearchPopularity
		}
		a.ImpShareFrom += r.ImpressionShare.From
		a.ImpShareTo += r.ImpressionShare.To
	}

	type entry struct {
		key       searchTermInfoKey
		agg       *searchTermInfoAgg
		avgShare  float64 // avg midpoint impression share [0,1]
		potential int     // estimated total market = our impressions / our share — higher means bigger untapped opportunity
	}

	// build country → campaign IDs index from config
	campaignsByCountry := make(map[geo.Country][]goappleads.CampaignID)
	for _, camp := range config.Campaigns {
		for _, country := range camp.Countries {
			campaignsByCountry[country] = append(campaignsByCountry[country], camp.ID)
		}
	}

	// build country → baseline CVR and CPI from campaign baselines
	countryCVR := make(map[geo.Country]float64)
	countryCPI := make(map[geo.Country]float64)
	for country, campIDs := range campaignsByCountry {
		var totalTaps, totalInst int
		var totalSpend float64
		for _, id := range campIDs {
			if b, ok := baselines[id]; ok {
				totalTaps += b.Taps
				totalInst += b.Inst
				totalSpend += b.Spend
			}
		}
		if totalTaps > 0 {
			countryCVR[country] = float64(totalInst) / float64(totalTaps)
		}
		if totalInst > 0 {
			countryCPI[country] = totalSpend / float64(totalInst)
		}
	}

	entries := make([]entry, 0, len(byKey))
	for k, a := range byKey {
		avgMid := 0.0
		if a.Days > 0 {
			avgMid = (a.ImpShareFrom/float64(a.Days) + a.ImpShareTo/float64(a.Days)) / 2
		}
		potential := 0
		if avgMid > 0 {
			potential = int(float64(a.Impressions) / avgMid)
		}
		entries = append(entries, entry{key: k, agg: a, avgShare: avgMid, potential: potential})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].potential > entries[j].potential })
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}

	maxImp := 0
	for _, e := range entries {
		if e.agg.Impressions > maxImp {
			maxImp = e.agg.Impressions
		}
	}

	tw := fmtx.TableWriter{
		Indent:     "  ",
		Out:        w,
		ColDefault: columnDefaults,
		Cols: []fmtx.TablCol{
			{Header: "", Width: 2},
			{Header: "Search Term", Width: 28},
			{Header: "Popularity", Width: 10, Alignment: fmtx.AlignRight},
			{Header: "Rank", Width: 4, Alignment: fmtx.AlignRight},
			{Header: "ImpShare", Width: 9, Alignment: fmtx.AlignRight},
			{Header: "", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Impression", Width: 11, Alignment: fmtx.AlignRight},
			{Header: "Potential.Imp", Width: 13, Alignment: fmtx.AlignRight},
			{Header: "Spend"},
			{Header: "CPI"},
			{Header: "CPI/Base"},
			{Header: "CVR"},
			{Header: "CVR/Base"},
			{Header: "Conf"},
			{Header: "Existing", Width: 12},
			{Header: "Action", Width: 16},
		},
	}

	if showID {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "Keyword IDs", Width: 15})
	}

	if showNegativeKeywords {
		tw.Cols = append(tw.Cols, fmtx.TablCol{Header: "Neg"})
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, e := range entries {
		a := e.agg
		avgFrom := a.ImpShareFrom / float64(a.Days) * 100
		avgTo := a.ImpShareTo / float64(a.Days) * 100

		// rank — green=1, yellow=2-3, red=>5
		rankStr := strconv.Itoa(a.Rank)
		if a.Rank == 6 {
			rankStr = ">5"
		}
		rankColored := rankStr
		switch {
		case a.Rank == 1:
			rankColored = fmtx.GreenS(rankStr)
		case a.Rank <= 3:
			rankColored = fmtx.YellowS(rankStr)
		default:
			rankColored = fmtx.RedS(rankStr)
		}

		// popularity — green=4-5, yellow=3, red=1-2
		popStr := strconv.Itoa(a.SearchPopularity)
		switch {
		case a.SearchPopularity >= 4:
			popStr = fmtx.GreenS(popStr)
		case a.SearchPopularity >= 3:
			popStr = fmtx.YellowS(popStr)
		default:
			popStr = fmtx.RedS(popStr)
		}

		impShareStr := fmt.Sprintf("%2.0f-%2.0f%%", avgFrom, avgTo)
		avgMidPct := (avgFrom + avgTo) / 2
		impShareColored := impShareStr
		switch {
		case avgMidPct >= 50:
			impShareColored = fmtx.GreenS(impShareStr)
		case avgMidPct >= 25:
			impShareColored = fmtx.YellowS(impShareStr)
		default:
			impShareColored = fmtx.RedS(impShareStr)
		}

		cvrStr := fmtx.DimS("-")
		cvrBaseStr := fmtx.DimS("-")
		cvrRatio := 0.0
		if a.Taps > 0 {
			cvr := float64(a.Installs) / float64(a.Taps)
			cvrStr = fmt.Sprintf("%.1f%%", cvr*100)
			if baseCVR, ok := countryCVR[e.key.Country]; ok && baseCVR > 0 {
				cvrRatio = cvr / baseCVR
				ratioStr := fmt.Sprintf("%.2fx", cvrRatio)
				switch {
				case cvrRatio >= 1.2:
					cvrBaseStr = fmtx.GreenS(ratioStr)
				case cvrRatio >= 0.8:
					cvrBaseStr = fmtx.YellowS(ratioStr)
				default:
					cvrBaseStr = fmtx.RedS(ratioStr)
				}
			}
		}

		cpiStr := fmtx.DimS("-")
		cpiBaseStr := fmtx.DimS("-")
		cpiRatio := 0.0
		if a.Installs > 0 {
			cpi := a.Spend / float64(a.Installs)
			cpiStr = fmt.Sprintf("$%.2f", cpi)
			if baseCPI, ok := countryCPI[e.key.Country]; ok && baseCPI > 0 {
				cpiRatio = cpi / baseCPI
				ratioStr := fmt.Sprintf("%.2fx", cpiRatio)
				switch {
				case cpiRatio <= 0.8:
					cpiBaseStr = fmtx.GreenS(ratioStr)
				case cpiRatio <= 1.2:
					cpiBaseStr = fmtx.YellowS(ratioStr)
				default:
					cpiBaseStr = fmtx.RedS(ratioStr)
				}
			}
		}

		// cross-reference with existing keywords for this country's campaign(s)
		var hasExact, hasBroad bool
		var isAllPaused, isAllNegative bool = true, true
		var keywordIDs []goappleads.KeywordID
		if campaignIDs, ok := campaignsByCountry[e.key.Country]; ok {
			campSet := make(map[goappleads.CampaignID]struct{}, len(campaignIDs))
			for _, id := range campaignIDs {
				campSet[id] = struct{}{}
			}
			for _, kw := range keywordsDB.GetKeywordsByText(e.key.SearchTerm) {
				if _, ok := campSet[kw.CampaignID]; !ok {
					continue
				}

				keywordIDs = append(keywordIDs, kw.ID)

				if !kw.IsNegative {
					isAllNegative = false
				}

				adgroup := config.GetAdGroup(kw.AdGroupID)
				campaign := config.GetCampaign(kw.CampaignID)

				paused := kw.Status == goappleads.Paused || kw.Status == goappleads.Deleted || adgroup.Status == goappleads.Paused || campaign.Status == goappleads.Paused
				if !paused {
					isAllPaused = false
				}

				switch kw.MatchType {
				case goappleads.Exact:
					hasExact = true
				case goappleads.Broad:
					hasBroad = true
				}
			}
		}

		if hasExact || hasBroad {
			if !showNegativeKeywords && isAllNegative {
				continue
			}
			if !showPaused && isAllPaused {
				continue
			}
		}

		existStr := "-"
		if hasExact || hasBroad {
			existStr = ""

			if hasExact {
				existStr += "exact"
			}
			if hasBroad {
				if hasExact {
					existStr += ", "
				}
				existStr += "broad"
			}
		}
		existStr = fmtx.DimS(existStr)

		goodPerf := cvrRatio >= 1.2 || (cpiRatio > 0 && cpiRatio <= 0.8)
		badPerf := (cvrRatio > 0 && cvrRatio < 0.8) && (cpiRatio == 0 || cpiRatio > 1.2)
		confidence := confidenceFromDaysImpressions(a.Days, a.Impressions)

		actionStr := fmtx.DimS("monitor")
		if confidence >= 0.75 {
			switch {
			case isAllPaused && goodPerf:
				actionStr = fmtx.YellowS("unpause")
			case (hasExact || hasBroad) && e.avgShare < 0.3 && a.Rank <= 3 && a.SearchPopularity >= 3:
				if badPerf {
					actionStr = fmtx.YellowS("add?") // opportunity but converting poorly
				} else {
					actionStr = fmtx.GreenS("add")
				}
			case (hasExact || hasBroad) && e.avgShare < 0.3 && a.Rank <= 3 && a.SearchPopularity >= 3:
				if badPerf {
					actionStr = fmtx.YellowS("raise bid") // be cautious — not converting well
				} else {
					actionStr = fmtx.GreenS("raise bid++")
				}
			case (hasExact || hasBroad) && e.avgShare >= 0.5:
				if badPerf {
					actionStr = fmtx.RedS("lower bid")
				} else {
					actionStr = fmtx.BlueS("hold bid")
				}
			case (hasExact || hasBroad) && a.Rank <= 3:
				if goodPerf {
					actionStr = fmtx.GreenS("raise bid")
				} else if badPerf {
					actionStr = fmtx.DimS("monitor")
				} else {
					actionStr = fmtx.YellowS("raise bid")
				}
			}
		}

		row := []string{
			e.key.Country.String(),
			e.key.SearchTerm,
			popStr,
			rankColored,
			impShareColored,
			strconv.Itoa(a.Impressions),
			fmtx.VolumeBar(a.Impressions, maxImp, 11),
			strconv.Itoa(e.potential),
			strconv.FormatFloat(a.Spend, 'f', 2, 64),
			cpiStr,
			cpiBaseStr,
			cvrStr,
			cvrBaseStr,
			confidence.Format(),
			existStr,
			actionStr,
		}

		if showID {
			if len(keywordIDs) == 0 {
				row = append(row, "-")
			} else {
				idStrs := make([]string, len(keywordIDs))
				for i, id := range keywordIDs {
					idStrs[i] = fmtx.DimS(id.String())
				}
				row = append(row, fmtx.DimS(strings.Join(idStrs, ", ")))
			}
		}

		if showNegativeKeywords {
			negS := ""
			if isAllNegative {
				negS = fmtx.DimS("neg")
			}
			row = append(row, negS)
		}

		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

const DocShort string = "search terms stats (CPI, CVR, CTR, Installs, Spend, Popularity, Rank, Impression Share, ...), new candidates"
const doc string = `
Apple Ads Search Terms Analysis — overview, top performers, new keyword candidates, underperformers, and impression share analysis.

Timezone: UTC
Currency: USD

`

func Run(args []string) {
	flag := flag.NewFlagSet("analyse searchterms", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		searchTermStatsCSV            string
		searchTermInfoCSV             string
		showID, showPaused            bool
		showNegativeKeywords          bool
		minSpend                      float64
		topN                          int
		campaignIDsStr, adGroupIDsStr string
		countries                     []geo.Country
		from, until                   time.Time
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV (used to compute campaign baselines for impression share)")
	flag.StringVar(&searchTermStatsCSV, "search-term-stats-csv", "data/apple_ads_search_terms_by_day.csv", "path to search term stats by day CSV")
	flag.StringVar(&searchTermInfoCSV, "search-term-info-csv", "data/apple_ads_search_term_impression_share_by_day.csv", "path to search term impression share by day CSV")
	flag.BoolVar(&showNegativeKeywords, "negative-keywords", false, "show negative keywords")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.BoolVar(&showPaused, "paused", false, "include paused keywords, adgroups, campaigns")
	flag.Float64Var(&minSpend, "min-spend", 0.40, "min spend threshold for wasteful search terms")
	flag.IntVar(&topN, "top-n", 300, "number of top search terms to show")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.Func("countries", "comma-separated list of countries (ISO 3166) to keep (default keep all)", slicex.Parser(&countries))
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep (for searchterm impression share selects country of campaign)")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of adgroup IDs to keep")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	flag.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	var keepCampaign map[goappleads.CampaignID]bool
	if len(campaignIDsStr) > 0 {
		keepCampaign = make(map[goappleads.CampaignID]bool)
		for id := range strings.SplitSeq(campaignIDsStr, ",") {
			keepCampaign[goappleads.CampaignID(id)] = true
		}
	}

	var keepAdGroup map[goappleads.AdGroupID]bool
	if len(adGroupIDsStr) > 0 {
		keepAdGroup = make(map[goappleads.AdGroupID]bool)
		for id := range strings.SplitSeq(adGroupIDsStr, ",") {
			keepAdGroup[goappleads.AdGroupID(id)] = true
		}
	}

	var keepCountries map[geo.Country]bool
	if len(countries) > 0 {
		keepCountries = make(map[geo.Country]bool)
		for _, country := range countries {
			keepCountries[country] = true
		}
	}

	var kwRows []goappleads.KeywordRow
	for r := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && r.Day.Before(from)) || (!until.IsZero() && r.Day.After(until)) {
			continue
		}
		if keepCampaign != nil && !keepCampaign[r.CampaignID] {
			continue
		}
		kwRows = append(kwRows, r)
	}

	var searchTerms []goappleads.SearchTermRow
	for e := range iterx.FromFile(searchTermStatsCSV, goappleads.ParseSearchTermsStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		if keepCampaign != nil && !keepCampaign[e.CampaignID] {
			continue
		}
		if keepAdGroup != nil && !keepAdGroup[e.AdGroupID] {
			continue
		}
		searchTerms = append(searchTerms, e)
	}

	var searchTermInfo []goappleads.SearchTermInfo
	for e := range iterx.FromFile(searchTermInfoCSV, goappleads.ParseSearchTermInfoFromCSV) {
		if !from.IsZero() || !until.IsZero() {
			ts, err := time.Parse(time.DateOnly, e.Day)
			if err == nil && ((!from.IsZero() && ts.Before(from)) || (!until.IsZero() && ts.After(until))) {
				continue
			}
		}

		if keepCountries != nil && !keepCountries[e.Country] {
			continue
		}

		searchTermInfo = append(searchTermInfo, e)
	}

	baselines, _ := goappleads.ComputeBaselines(kwRows)

	w := os.Stdout

	printSearchTermsOverview(w, searchTerms)
	printSearchTermsTopPerformers(w, searchTerms, topN, showNegativeKeywords, showPaused, keywordsDB, *config)
	printSearchTermsNewKeywordCandidates(w, searchTerms, topN, showPaused, keywordsDB, *config)
	printSearchTermsUnderperformers(w, searchTerms, minSpend, showNegativeKeywords, showPaused, keywordsDB, *config)
	printSearchTermImpressionShare(w, searchTermInfo, topN, keywordsDB, *config, baselines, showNegativeKeywords, showID, showPaused)
}
