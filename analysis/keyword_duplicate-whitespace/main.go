package keywordduplicatewhitespace

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ndx-technologies/fmtx"
	goappleads "github.com/ndx-technologies/go-apple-ads"
	"github.com/ndx-technologies/go-apple-ads/analysis"
	"github.com/ndx-technologies/iterx"
	"github.com/ndx-technologies/timex"
)

const DocShort string = "detect duplicate keywords (whitespace)"
const doc string = `
Exact keywords without Search Match allow certain degree of variation.
This includes matching keywords without whitespace.

It is recommended to reduce such duplicates to increase apple learning engine efficiency and reduce cannibalisation.
https://ads.apple.com/app-store/help/keywords/0059-understand-keyword-match-types

`

type KeywordStats struct {
	Impressions int
	Taps        int
	Installs    int
	Spend       float64
}

type Issue struct {
	AdGroupID  goappleads.AdGroupID
	CampaignID goappleads.CampaignID
	Keywords   []goappleads.KeywordInfo
	Stats      map[goappleads.KeywordID]KeywordStats
}

func Analyze(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB, stats map[goappleads.KeywordID]KeywordStats) []Issue {
	type key struct {
		adGroupID   goappleads.AdGroupID
		fingerprint string
	}
	groups := make(map[key][]goappleads.KeywordInfo)
	for _, kw := range keywordsDB.Keywords {
		if kw.IsNegative || kw.Status != goappleads.Active {
			continue
		}
		if config.IsAdGroupPaused(kw.AdGroupID) {
			continue
		}
		fp := strings.ReplaceAll(strings.ToLower(kw.Keyword), " ", "")
		k := key{adGroupID: kw.AdGroupID, fingerprint: fp}
		groups[k] = append(groups[k], kw)
	}

	var issues []Issue
	for _, kwds := range groups {
		if len(kwds) < 2 {
			continue
		}
		issues = append(issues, Issue{
			AdGroupID:  kwds[0].AdGroupID,
			CampaignID: kwds[0].CampaignID,
			Keywords:   kwds,
			Stats:      stats,
		})
	}
	return issues
}

func printAnalysis(w io.StringWriter, showID bool, config *goappleads.Config, issues []Issue) {
	fmtx.HeaderTo(w, "Keyword Duplicate Whitespace Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		campA := config.GetCampaign(a.CampaignID).Name
		campB := config.GetCampaign(b.CampaignID).Name
		if campA != campB {
			return campA < campB
		}
		agA := config.GetAdGroup(a.AdGroupID).Name
		agB := config.GetAdGroup(b.AdGroupID).Name
		return agA < agB
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 36},
			{Header: "Ad Group", Width: 28},
			{Header: "Keyword", Width: 32},
			{Header: "Keyword ID", Width: 14},
			{Header: "Imp", Width: 7, Alignment: fmtx.AlignRight},
			{Header: "Taps", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Inst", Width: 6, Alignment: fmtx.AlignRight},
			{Header: "Spend", Width: 8, Alignment: fmtx.AlignRight},
		},
	}
	if showID {
		tw.Cols = append([]fmtx.TablCol{
			{Header: "CampaignID", Width: 12},
			{Header: "AdGroupID", Width: 12},
		}, tw.Cols...)
	}

	tw.WriteHeader()
	tw.WriteHeaderLine()

	for _, iss := range issues {
		for i, kw := range iss.Keywords {
			st := iss.Stats[kw.ID]
			campStr := ""
			agStr := ""
			if i == 0 {
				campStr = config.GetCampaign(iss.CampaignID).Name
				agStr = config.GetAdGroup(iss.AdGroupID).Name
			}
			kwStr := fmtx.RedS(kw.Keyword)
			if st.Impressions == 0 && st.Taps == 0 && st.Installs == 0 {
				kwStr = fmtx.DimS(kw.Keyword)
			}
			row := []string{
				campStr,
				agStr,
				kwStr,
				fmtx.DimS(string(kw.ID)),
				strconv.Itoa(st.Impressions),
				strconv.Itoa(st.Taps),
				strconv.Itoa(st.Installs),
				strconv.FormatFloat(st.Spend, 'f', 2, 64),
			}
			if showID {
				row = append([]string{fmtx.DimS(string(iss.CampaignID)), fmtx.DimS(string(iss.AdGroupID))}, row...)
			}
			tw.WriteRow(row...)
		}
		w.WriteString("\n")
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	fs := flag.NewFlagSet("analyse keywords duplicate-whitespace", flag.ExitOnError)
	var (
		applePath       string
		keywordStatsCSV string
		showID          bool
		verbose         bool
		from, until     time.Time
	)
	fs.Usage = func() {
		fs.Output().Write([]byte(doc))
		fs.PrintDefaults()
	}
	fs.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	fs.StringVar(&keywordStatsCSV, "keyword-stats-csv", "data/apple_ads_search_keywords_by_day.csv", "path to keyword stats by day CSV")
	fs.BoolVar(&showID, "id", false, "show IDs")
	fs.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	fs.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	fs.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	fs.Func("until", "until UTC day start (e.g. 2026-01-01) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	fs.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	stats := make(map[goappleads.KeywordID]KeywordStats)
	for e := range iterx.FromFile(keywordStatsCSV, goappleads.ParseKeywordStatsCSV) {
		if (!from.IsZero() && e.Day.Before(from)) || (!until.IsZero() && e.Day.After(until)) {
			continue
		}
		s := stats[e.KeywordID]
		s.Impressions += e.Impressions
		s.Taps += e.Taps
		s.Installs += e.Installs
		s.Spend += e.Spend
		stats[e.KeywordID] = s
	}

	issues := Analyze(config, keywordsDB, stats)

	if len(issues) == 0 {
		return Info{N: len(keywordsDB.Keywords)}, nil
	}

	if verbose {
		printAnalysis(os.Stdout, showID, config, issues)
	}

	return nil, ErrKeywordDuplicateWhitespace{Count: len(issues)}
}

type Info struct{ N int }

func (s Info) String() string {
	return "no whitespace-duplicate keywords (" + strconv.Itoa(s.N) + " active keywords checked)"
}

type ErrKeywordDuplicateWhitespace struct {
	Count int
}

func (e ErrKeywordDuplicateWhitespace) Error() string {
	return strconv.Itoa(e.Count) + " whitespace-duplicate keyword group(s) found (run with -v for details)"
}
