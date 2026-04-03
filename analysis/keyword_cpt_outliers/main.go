package appleadsanalysiskeywordcptoutliers

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
	"github.com/ndx-technologies/go-apple-ads/analysis"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/tdigest"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "detect keyword outliers with large CPT"
const doc string = "Keyword with anomalously high CPT can quickly drain budget."

type KeywordStats struct {
	Spend float64
	Taps  int
}

type Issue struct {
	Keyword goappleads.KeywordInfo
	Spend   float64
	Taps    int
	CPT     float64
	P50CPT  float64
	Ratio   float64
}

type Summary struct{ NumKeywords, NumAdGroups int }

func isEligibleKeyword(config *goappleads.Config, kw goappleads.KeywordInfo) bool {
	if kw.IsNegative || kw.Status != goappleads.Active || kw.MatchType == goappleads.Auto {
		return false
	}
	if config.IsAdGroupPaused(kw.AdGroupID) {
		return false
	}
	return true
}

func Analyze(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB, stats map[goappleads.KeywordID]KeywordStats, cptOutlierRatio float64) ([]Issue, Summary) {
	digestByAdGroup := make(map[goappleads.AdGroupID]tdigest.TDigest)
	checkedKeywords := 0

	for id, kw := range keywordsDB.Keywords {
		if !isEligibleKeyword(config, kw) {
			continue
		}
		st := stats[id]
		if st.Taps <= 0 || st.Spend <= 0 {
			continue
		}
		cpt := st.Spend / float64(st.Taps)
		if cpt <= 0 {
			continue
		}
		d := digestByAdGroup[kw.AdGroupID]
		d.Insert(float32(cpt), 1)
		digestByAdGroup[kw.AdGroupID] = d
		checkedKeywords++
	}

	for adGroupID := range digestByAdGroup {
		d := digestByAdGroup[adGroupID]
		d.Compress(100)
		digestByAdGroup[adGroupID] = d
	}

	var issues []Issue
	for id, kw := range keywordsDB.Keywords {
		if !isEligibleKeyword(config, kw) {
			continue
		}
		st := stats[id]
		if st.Taps <= 0 || st.Spend <= 0 {
			continue
		}
		d := digestByAdGroup[kw.AdGroupID]
		if d.IsZero() || d.Count == 0 {
			continue
		}
		p50 := float64(d.Quantile(0.5))
		if p50 <= 0 {
			continue
		}
		cpt := st.Spend / float64(st.Taps)
		ratio := cpt / p50
		if ratio < cptOutlierRatio {
			continue
		}
		issues = append(issues, Issue{
			Keyword: kw,
			Spend:   st.Spend,
			Taps:    st.Taps,
			CPT:     cpt,
			P50CPT:  p50,
			Ratio:   ratio,
		})
	}

	return issues, Summary{NumKeywords: checkedKeywords, NumAdGroups: len(digestByAdGroup)}
}

func printAnalysis(w io.StringWriter, config *goappleads.Config, showID bool, issues []Issue) {
	fmtx.HeaderTo(w, "Keyword CPT Outliers Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Ratio != b.Ratio {
			return a.Ratio > b.Ratio
		}
		campA := config.GetCampaign(a.Keyword.CampaignID).Name
		campB := config.GetCampaign(b.Keyword.CampaignID).Name
		if campA != campB {
			return campA < campB
		}
		agA := config.GetAdGroup(a.Keyword.AdGroupID).Name
		agB := config.GetAdGroup(b.Keyword.AdGroupID).Name
		if agA != agB {
			return agA < agB
		}
		return a.Keyword.Keyword < b.Keyword.Keyword
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 32},
			{Header: "Ad Group", Width: 24},
			{Header: "Keyword", Width: 28},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Spend", Width: 8, Alignment: fmtx.AlignRight},
			{Header: "CPT", Width: 8, Alignment: fmtx.AlignRight},
			{Header: "P50", Width: 8, Alignment: fmtx.AlignRight},
			{Header: "Ratio", Width: 7, Alignment: fmtx.AlignRight},
		},
	}

	if showID {
		tw.Cols = append([]fmtx.TablCol{
			{Header: "CampaignID", Width: 12},
			{Header: "AdGroupID", Width: 12},
			{Header: "KeywordID", Width: 12},
		}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, iss := range issues {
		camp := config.GetCampaign(iss.Keyword.CampaignID)
		ag := config.GetAdGroup(iss.Keyword.AdGroupID)
		row := []string{
			camp.Name,
			ag.Name,
			iss.Keyword.Label(),
			strconv.Itoa(iss.Taps),
			strconv.FormatFloat(iss.Spend, 'f', 2, 64),
			fmtx.RedS(strconv.FormatFloat(iss.CPT, 'f', 2, 64)),
			strconv.FormatFloat(iss.P50CPT, 'f', 2, 64),
			fmtx.RedS(strconv.FormatFloat(iss.Ratio, 'f', 1, 64) + "x"),
		}
		if showID {
			row = append([]string{
				fmtx.DimS(string(iss.Keyword.CampaignID)),
				fmtx.DimS(string(iss.Keyword.AdGroupID)),
				fmtx.DimS(string(iss.Keyword.ID)),
			}, row...)
		}
		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	flag := flag.NewFlagSet("analyse keywords cpt-outliers", flag.ExitOnError)
	var (
		applePath                     string
		keywordStatsCSV               string
		verbose                       bool
		campaignIDsStr, adGroupIDsStr string
		showID                        bool
		from, until                   time.Time
		cptOutlierRatio               float64
	)
	flag.Usage = func() {
		flag.Output().Write([]byte(doc))
		flag.PrintDefaults()
	}
	flag.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	flag.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	flag.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	flag.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	flag.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	flag.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of adgroup IDs to keep")
	flag.BoolVar(&showID, "id", false, "show IDs")
	flag.Float64Var(&cptOutlierRatio, "ratio", 3.0, "CPT outlier ratio threshold (e.g. 3.0 means flag keywords with CPT >= 3x ad group p50 CPT)")
	flag.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	flag.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
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

	stats := make(map[goappleads.KeywordID]KeywordStats)
	for r := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && r.Day.Before(from)) || (!until.IsZero() && r.Day.After(until)) {
			continue
		}
		if keepAdGroup != nil && !keepAdGroup[r.AdGroupID] {
			continue
		}
		if keepCampaign != nil && !keepCampaign[r.CampaignID] {
			continue
		}
		st := stats[r.KeywordID]
		st.Spend += r.Spend
		st.Taps += r.Taps
		stats[r.KeywordID] = st
	}

	issues, summary := Analyze(config, keywordsDB, stats, cptOutlierRatio)
	if len(issues) == 0 {
		return InfoKeywordCPTOutliersOK(summary), nil
	}

	if verbose {
		printAnalysis(os.Stdout, config, showID, issues)
	}

	return nil, &ErrKeywordCPTOutliers{NumIssues: len(issues)}
}

type InfoKeywordCPTOutliersOK Summary

func (s InfoKeywordCPTOutliersOK) String() string {
	return "no keyword CPT outliers found (" + strconv.Itoa(s.NumKeywords) + " keywords checked across " + strconv.Itoa(s.NumAdGroups) + " adgroups)"
}

type ErrKeywordCPTOutliers struct{ NumIssues int }

func (e *ErrKeywordCPTOutliers) Error() string {
	return strconv.Itoa(e.NumIssues) + " keyword CPT outlier(s) found (run with -v for details)"
}
