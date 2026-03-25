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

type Issue struct {
	AdGroupID  goappleads.AdGroupID
	CampaignID goappleads.CampaignID
	Keywords   []goappleads.KeywordInfo
}

func Analyze(config *goappleads.Config, keywordsDB *goappleads.KeywordCSVDB) []Issue {
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
			{Header: "Keywords", Width: 60},
			{Header: "Keyword IDs", Width: 40},
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
		var kwStrs []string
		var kwIDStrs []string
		for _, kw := range iss.Keywords {
			kwStrs = append(kwStrs, fmtx.RedS(kw.Keyword))
			kwIDStrs = append(kwIDStrs, fmtx.DimS(string(kw.ID)))
		}
		row := []string{
			config.GetCampaign(iss.CampaignID).Name,
			config.GetAdGroup(iss.AdGroupID).Name,
			strings.Join(kwStrs, ", "),
			strings.Join(kwIDStrs, ", "),
		}
		if showID {
			row = append([]string{fmtx.DimS(string(iss.CampaignID)), fmtx.DimS(string(iss.AdGroupID))}, row...)
		}
		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	fs := flag.NewFlagSet("analyse keywords duplicate-whitespace", flag.ExitOnError)
	var (
		applePath string
		showID    bool
		verbose   bool
	)
	fs.Usage = func() {
		fs.Output().Write([]byte(doc))
		fs.PrintDefaults()
	}
	fs.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	fs.BoolVar(&showID, "id", false, "show IDs")
	fs.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	fs.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	fs.Parse(args)

	config, keywordsDB, err := goappleads.Load(applePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	issues := Analyze(config, keywordsDB)

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
