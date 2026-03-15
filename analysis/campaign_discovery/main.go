package appleadsanalysiscampaigndiscovery

import (
	"errors"
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
	"github.com/ndx-technologies/timex"
)

const DocShort string = "detect Search Match enabled in non-discovery campaigns"
const doc string = `
Search Match works independently from Keywords.
It is possible even with all keywords disabled have Search Match generating traffic.
This particularly can drive wrong traffic into exact (non-discovery) campaigns.
On the other hand, Search Match is beneficial to Discovery campaigns.
`

func isDiscoveryCampaign(name string) bool { return strings.Contains(name, "Discovery") }

type IssueType string

const (
	SearchMatchEnabledInNonDiscovery IssueType = "search_match_enabled_in_non_discovery"
	SearchMatchDisabledInDiscovery   IssueType = "search_match_disabled_in_discovery"
)

type Issue struct {
	Type         IssueType
	CampaignID   goappleads.CampaignID
	CampaignName string
	AdGroupID    goappleads.AdGroupID
	AdGroupName  string
}

func Analyze(config *goappleads.Config) []Issue {
	var issues []Issue
	for _, camp := range config.Campaigns {
		discovery := isDiscoveryCampaign(camp.Name)
		for _, ag := range camp.AdGroups {
			if !discovery && ag.SearchMatch {
				issues = append(issues, Issue{
					Type:         SearchMatchEnabledInNonDiscovery,
					CampaignID:   camp.ID,
					CampaignName: camp.Name,
					AdGroupID:    ag.ID,
					AdGroupName:  ag.Name,
				})
			}
			if discovery && !ag.SearchMatch {
				issues = append(issues, Issue{
					Type:         SearchMatchDisabledInDiscovery,
					CampaignID:   camp.ID,
					CampaignName: camp.Name,
					AdGroupID:    ag.ID,
					AdGroupName:  ag.Name,
				})
			}
		}
	}
	return issues
}

func printAnalysis(w io.StringWriter, showID bool, issues []Issue) {
	fmtx.HeaderTo(w, "Campaign Discovery - Search Match Analysis")

	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.CampaignName == b.CampaignName {
			return a.AdGroupName < b.AdGroupName
		}
		return a.CampaignName < b.CampaignName
	})

	tw := fmtx.TableWriter{
		Indent: "  ",
		Out:    w,
		Cols: []fmtx.TablCol{
			{Header: "Campaign", Width: 36},
			{Header: "Ad Group", Width: 28},
			{Header: "Search Match", Width: 12, Alignment: fmtx.AlignRight},
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

	for _, e := range issues {
		var sm string
		switch e.Type {
		case SearchMatchEnabledInNonDiscovery:
			sm = fmtx.RedS("+")
		case SearchMatchDisabledInDiscovery:
			sm = fmtx.RedS("-")
		}
		row := []string{e.CampaignName, e.AdGroupName, sm}
		if showID {
			row = append([]string{fmtx.DimS(string(e.CampaignID)), fmtx.DimS(string(e.AdGroupID))}, row...)
		}
		tw.WriteRow(row...)
	}
	w.WriteString("\n")
}

func Run(args []string) (analysis.Info, error) {
	fs := flag.NewFlagSet("analyse campaign discovery", flag.ExitOnError)
	var (
		applePath                     string
		showID                        bool
		verbose                       bool
		campaignIDsStr, adGroupIDsStr string
		from, until                   time.Time
	)
	fs.Usage = func() {
		fs.Output().Write([]byte(doc))
		fs.PrintDefaults()
	}
	fs.StringVar(&applePath, "apple-path", "apple-ads", "path to dir with config.json and keywords CSVs")
	fs.BoolVar(&showID, "id", false, "show IDs")
	fs.BoolVar(&fmtx.EnableColor, "color", os.Getenv("NO_COLOR") == "", "colorize output")
	fs.BoolVar(&verbose, "v", false, "verbose: print full table; by default prints one-line summary")
	fs.StringVar(&campaignIDsStr, "campaign-ids", "", "comma-separated list of campaign IDs to keep")
	fs.StringVar(&adGroupIDsStr, "adgroup-ids", "", "comma-separated list of adgroup IDs to keep")
	fs.Func("from", "from UTC day start (e.g. 2025-01-01) (default keep all)", timex.TimeParserWithFormat(&from, time.DateOnly))
	fs.Func("until", "until UTC day start (e.g. 2025-12-31) (default keep all)", timex.TimeParserWithFormat(&until, time.DateOnly))
	fs.Parse(args)

	config, _, err := goappleads.Load(applePath)
	if err != nil {
		log.Fatal("failed to load data:", err)
	}

	issues := Analyze(config)

	if len(issues) == 0 {
		var numCampaigns, numDiscovery, numNonDiscovery, numNonDiscoveryAdGroups int
		for _, camp := range config.Campaigns {
			numCampaigns++
			if isDiscoveryCampaign(camp.Name) {
				numDiscovery++
			} else {
				numNonDiscovery++
				numNonDiscoveryAdGroups += len(camp.AdGroups)
			}
		}
		return analysis.Join(
			InfoSearchMatchDisabled{NumNonDiscovery: numNonDiscovery, NumCampaigns: numCampaigns, NumAdGroups: numNonDiscoveryAdGroups},
			InfoSearchMatchEnabled{NumDiscovery: numDiscovery, NumCampaigns: numCampaigns},
		), nil
	}

	if verbose {
		printAnalysis(os.Stdout, showID, issues)
	}

	var numEnabled, numDisabled int
	for _, iss := range issues {
		switch iss.Type {
		case SearchMatchEnabledInNonDiscovery:
			numEnabled++
		case SearchMatchDisabledInDiscovery:
			numDisabled++
		}
	}
	var errs []error
	if numEnabled > 0 {
		errs = append(errs, ErrSearchMatchEnabledInNonDiscovery{NumEnabled: numEnabled})
	}
	if numDisabled > 0 {
		errs = append(errs, ErrSearchMatchDisabledInDiscovery{NumDisabled: numDisabled})
	}
	return nil, errors.Join(errs...)
}

type InfoSearchMatchDisabled struct{ NumNonDiscovery, NumCampaigns, NumAdGroups int }

func (s InfoSearchMatchDisabled) String() string {
	return "search match disabled in all non-discovery campaigns(" + strconv.Itoa(s.NumNonDiscovery) + "/" + strconv.Itoa(s.NumCampaigns) + ") adgroups(" + strconv.Itoa(s.NumAdGroups) + ")"
}

type InfoSearchMatchEnabled struct{ NumDiscovery, NumCampaigns int }

func (s InfoSearchMatchEnabled) String() string {
	return "search match enabled in all discovery campaigns(" + strconv.Itoa(s.NumDiscovery) + "/" + strconv.Itoa(s.NumCampaigns) + ")"
}

type ErrSearchMatchEnabledInNonDiscovery struct{ NumEnabled int }

func (e ErrSearchMatchEnabledInNonDiscovery) Error() string {
	return strconv.Itoa(e.NumEnabled) + " adgroups in non-discovery campaigns have search match enabled (run with -v for details)"
}

type ErrSearchMatchDisabledInDiscovery struct{ NumDisabled int }

func (e ErrSearchMatchDisabledInDiscovery) Error() string {
	return strconv.Itoa(e.NumDisabled) + " adgroups in discovery campaigns have search match disabled (run with -v for details)"
}
